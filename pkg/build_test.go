package pkg

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"

	"cigno/pkg/registry"
)

// startTestRegistry starts the embedded registry on an ephemeral port and
// returns its address (e.g. 127.0.0.1:PORT).
func startTestRegistry(t *testing.T) string {
	t.Helper()
	srv, err := registry.NewSimpleServer("127.0.0.1:0", t.TempDir())
	if err != nil {
		t.Fatalf("creating registry: %v", err)
	}
	if err := srv.Start(); err != nil {
		t.Fatalf("starting registry: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })
	return srv.GetAddr()
}

// pushBase pushes a simple image (one marker layer) to the test registry.
func pushBase(t *testing.T, addr, repo, tag string) string {
	t.Helper()
	img, err := mutate.Append(empty.Image, mutate.Addendum{
		Layer: newTarLayer(t, map[string]string{"base-file": "base"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	ref := fmt.Sprintf("%s/%s:%s", addr, repo, tag)
	if err := crane.Push(img, ref, crane.Insecure); err != nil {
		t.Fatalf("pushing base: %v", err)
	}
	return ref
}

func TestEngine_Build_LocalCopy(t *testing.T) {
	addr := startTestRegistry(t)
	baseRef := pushBase(t, addr, "test/base", "latest")

	buildDir, err := filepath.Abs("./testdata")
	if err != nil {
		t.Fatal(err)
	}

	df := fmt.Sprintf(`
FROM %s
COPY folder /data
ENV PATH=/data:$PATH
`, baseRef)

	engine := NewEngine()
	engine.BuildDir = buildDir

	out := filepath.Join(t.TempDir(), "out.tar")
	if err := engine.Build(strings.NewReader(df), OutFileOption(out, "local-copy:test")); err != nil {
		t.Fatal(err)
	}

	img, err := tarball.ImageFromPath(out, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertImageHasFileContent(t, img, "/data/file", "file content")
	assertImageEnv(t, img, "PATH", "/data")
}

func TestEngine_Build_FromScratch(t *testing.T) {
	buildDir, err := filepath.Abs("./testdata")
	if err != nil {
		t.Fatal(err)
	}

	df := `
FROM scratch
COPY folder /data
`

	engine := NewEngine()
	engine.BuildDir = buildDir

	out := filepath.Join(t.TempDir(), "out.tar")
	if err := engine.Build(strings.NewReader(df), OutFileOption(out, "scratch:test")); err != nil {
		t.Fatal(err)
	}

	img, err := tarball.ImageFromPath(out, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertImageHasFileContent(t, img, "/data/file", "file content")
}

func TestEngine_Build_CopySingleFile(t *testing.T) {
	buildDir, err := filepath.Abs("./testdata")
	if err != nil {
		t.Fatal(err)
	}

	// single file source must land at the exact dest path
	df := `
FROM scratch
COPY folder/file /marker
`

	engine := NewEngine()
	engine.BuildDir = buildDir

	out := filepath.Join(t.TempDir(), "out.tar")
	if err := engine.Build(strings.NewReader(df), OutFileOption(out, "file:test")); err != nil {
		t.Fatal(err)
	}

	img, err := tarball.ImageFromPath(out, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertImageHasFileContent(t, img, "/marker", "file content")
}

func TestEngine_Build_MultiStageCopyFromStage(t *testing.T) {
	addr := startTestRegistry(t)
	xRef := pushBase(t, addr, "test/x", "latest")
	yRef := pushBase(t, addr, "test/y", "latest")

	buildDir, err := filepath.Abs("./testdata")
	if err != nil {
		t.Fatal(err)
	}

	// multi-stage: build in stage `builder`, copy paths out into the
	// final stage (docker semantics — path-level, not layer rebase)
	df := fmt.Sprintf(`
FROM %s AS builder
COPY folder /build
FROM %s
COPY --from=builder /build /app
`, xRef, yRef)

	engine := NewEngine()
	engine.BuildDir = buildDir

	out := filepath.Join(t.TempDir(), "out.tar")
	if err := engine.Build(strings.NewReader(df), OutFileOption(out, "multi:test")); err != nil {
		t.Fatal(err)
	}

	img, err := tarball.ImageFromPath(out, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertImageHasFileContent(t, img, "/app/file", "file content")
	// the final stage's own base must be present, the builder's base must not
	assertImageHasFileContent(t, img, "/base-file", "base")
	files := extractFileSet(t, img)
	if _, ok := files["build/file"]; ok {
		t.Error("builder stage filesystem leaked into final image (/build)")
	}

	yImg, err := crane.Pull(yRef, crane.Insecure)
	if err != nil {
		t.Fatal(err)
	}
	yLayers, _ := yImg.Layers()
	appLayers, _ := img.Layers()
	if len(appLayers) != len(yLayers)+1 {
		t.Errorf("expected %d layers (final base + 1 copied layer), got %d", len(yLayers)+1, len(appLayers))
	}
}

func TestEngine_Build_CopyFromRebase(t *testing.T) {
	addr := startTestRegistry(t)
	baseRef := pushBase(t, addr, "test/base", "latest")

	buildDir, err := filepath.Abs("./testdata")
	if err != nil {
		t.Fatal(err)
	}

	// build a component image on top of base, then push it for rebase use
	compEngine := NewEngine()
	compEngine.BuildDir = buildDir
	compDF := fmt.Sprintf(`
FROM %s
COPY folder /comp
`, baseRef)
	compRef := fmt.Sprintf("%s/test/comp:latest", addr)
	if err := compEngine.Build(strings.NewReader(compDF), PushOption([]string{compRef})); err != nil {
		t.Fatalf("building component: %v", err)
	}

	// assemble: COPY --from the component brings its diff layers over base.
	// NOTE: rebase copies layers verbatim (component assembly semantics),
	// so the component's paths are preserved as-is.
	appEngine := NewEngine()
	appEngine.BuildDir = buildDir
	appDF := fmt.Sprintf(`
FROM %s
COPY --from=%s / /
`, baseRef, compRef)
	out := filepath.Join(t.TempDir(), "out.tar")
	if err := appEngine.Build(strings.NewReader(appDF), OutFileOption(out, "app:test")); err != nil {
		t.Fatal(err)
	}

	img, err := tarball.ImageFromPath(out, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertImageHasFileContent(t, img, "/comp/file", "file content")
	assertImageHasFileContent(t, img, "/base-file", "base")

	// the app must not duplicate base layers
	baseImg, err := crane.Pull(baseRef, crane.Insecure)
	if err != nil {
		t.Fatal(err)
	}
	baseLayers, _ := baseImg.Layers()
	appLayers, _ := img.Layers()
	if len(appLayers) != len(baseLayers)+1 {
		t.Errorf("expected %d layers (base + 1 diff), got %d", len(baseLayers)+1, len(appLayers))
	}
}

func TestEngine_Build_CopyWildcard(t *testing.T) {
	buildDir, err := filepath.Abs("./testdata")
	if err != nil {
		t.Fatal(err)
	}

	df := `
FROM scratch
COPY wild/*.txt /data/
`

	engine := NewEngine()
	engine.BuildDir = buildDir

	out := filepath.Join(t.TempDir(), "out.tar")
	if err := engine.Build(strings.NewReader(df), OutFileOption(out, "wild:test")); err != nil {
		t.Fatal(err)
	}

	img, err := tarball.ImageFromPath(out, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertImageHasFileContent(t, img, "/data/a.txt", "a\n")
	assertImageHasFileContent(t, img, "/data/b.txt", "b\n")
	files := extractFileSet(t, img)
	if _, ok := files["data/c.log"]; ok {
		t.Error("c.log should not match *.txt")
	}

	// a wildcard matching nothing must fail the build
	engine2 := NewEngine()
	engine2.BuildDir = buildDir
	err = engine2.Build(strings.NewReader("FROM scratch\nCOPY wild/*.nothing /data/\n"))
	if err == nil {
		t.Error("expected no-match wildcard to fail")
	}
}

func TestEngine_Build_InvalidBaseRef(t *testing.T) {
	engine := NewEngine()
	err := engine.Build(strings.NewReader("FROM this is not a ref\n"))
	if err == nil {
		t.Error("expected invalid base ref to fail")
	}
}

func TestEngine_Build_ArgExpansion(t *testing.T) {
	buildDir, err := filepath.Abs("./testdata")
	if err != nil {
		t.Fatal(err)
	}

	df := `
ARG VERSION=default-version
FROM scratch
ARG VERSION
COPY folder/file /marker-${VERSION}
`

	engine := NewEngine()
	engine.BuildDir = buildDir

	out := filepath.Join(t.TempDir(), "out.tar")
	if err := engine.Build(strings.NewReader(df), OutFileOption(out, "arg:test")); err != nil {
		t.Fatal(err)
	}

	img, err := tarball.ImageFromPath(out, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertImageHasFileContent(t, img, "/marker-default-version", "file content")
}

func TestParseDockerFile(t *testing.T) {
	buf := bytes.NewBufferString(`
FROM alpine:3.16.2 as base
COPY / /test/
ENV A=1
`)
	stages, _, err := ParseDockerFile(buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(stages) != 1 {
		t.Fatalf("expected 1 stage, got %d", len(stages))
	}
	if len(stages[0].Commands) != 2 {
		t.Errorf("expected 2 commands, got %d", len(stages[0].Commands))
	}
}

// assertImageEnv checks that the image config contains `key` with `substring`.
func assertImageEnv(t *testing.T, img v1.Image, key, substring string) {
	t.Helper()
	cfg, err := img.ConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	for _, env := range cfg.Config.Env {
		if strings.HasPrefix(env, key+"=") {
			if strings.Contains(env, substring) {
				return
			}
			t.Errorf("env %s does not contain %q: %s", key, substring, env)
			return
		}
	}
	t.Errorf("env %s not found in config: %v", key, cfg.Config.Env)
}
