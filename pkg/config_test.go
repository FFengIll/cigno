package pkg

import (
	"path/filepath"
	"strings"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

// buildImageFromDF is the harness for config-semantics assertions: it builds
// the given Dockerfile and returns the resulting image config.
func buildImageFromDF(t *testing.T, df string) *v1.ConfigFile {
	t.Helper()
	engine := NewEngine()
	engine.BuildDir = "./testdata"
	out := filepath.Join(t.TempDir(), "out.tar")
	if err := engine.Build(strings.NewReader(df), OutFileOption(out, "harness:test")); err != nil {
		t.Fatalf("build failed: %v", err)
	}
	img, err := tarball.ImageFromPath(out, nil)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := img.ConfigFile()
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestConfig_CmdShellForm(t *testing.T) {
	cfg := buildImageFromDF(t, "FROM scratch\nCMD echo hi\n")
	want := []string{"/bin/sh", "-c", "echo hi"}
	if strings.Join(cfg.Config.Cmd, " ") != strings.Join(want, " ") {
		t.Errorf("shell-form CMD = %v, want %v", cfg.Config.Cmd, want)
	}
}

func TestConfig_CmdExecForm(t *testing.T) {
	cfg := buildImageFromDF(t, `FROM scratch
CMD ["/bin/app", "--flag"]
`)
	want := []string{"/bin/app", "--flag"}
	if strings.Join(cfg.Config.Cmd, " ") != strings.Join(want, " ") {
		t.Errorf("exec-form CMD = %v, want %v", cfg.Config.Cmd, want)
	}
}

func TestConfig_EntrypointShellAndExec(t *testing.T) {
	cfg := buildImageFromDF(t, "FROM scratch\nENTRYPOINT serve --port 80\n")
	want := []string{"/bin/sh", "-c", "serve --port 80"}
	if strings.Join(cfg.Config.Entrypoint, " ") != strings.Join(want, " ") {
		t.Errorf("shell-form ENTRYPOINT = %v, want %v", cfg.Config.Entrypoint, want)
	}

	cfg = buildImageFromDF(t, `FROM scratch
ENTRYPOINT ["serve", "--port", "80"]
`)
	want = []string{"serve", "--port", "80"}
	if strings.Join(cfg.Config.Entrypoint, " ") != strings.Join(want, " ") {
		t.Errorf("exec-form ENTRYPOINT = %v, want %v", cfg.Config.Entrypoint, want)
	}
}

func TestConfig_WorkdirAbsoluteRelativeAndEnv(t *testing.T) {
	cfg := buildImageFromDF(t, `
FROM scratch
ENV APP=/opt
WORKDIR /base
WORKDIR sub
WORKDIR $APP
`)
	if cfg.Config.WorkingDir != "/opt" {
		t.Errorf("final WORKDIR = %q, want /opt", cfg.Config.WorkingDir)
	}

	// relative WORKDIR chains: /base -> /base/sub (assert via intermediate build)
	cfg = buildImageFromDF(t, `
FROM scratch
WORKDIR /base
WORKDIR sub
`)
	if cfg.Config.WorkingDir != "/base/sub" {
		t.Errorf("relative WORKDIR = %q, want /base/sub", cfg.Config.WorkingDir)
	}

	// relative WORKDIR with no prior one resolves against /
	cfg = buildImageFromDF(t, "FROM scratch\nWORKDIR rel\n")
	if cfg.Config.WorkingDir != "/rel" {
		t.Errorf("first relative WORKDIR = %q, want /rel", cfg.Config.WorkingDir)
	}
}

func TestConfig_UserExposeVolumeLabel(t *testing.T) {
	cfg := buildImageFromDF(t, `
FROM scratch
ARG PORT=8080
USER 1000:1000
EXPOSE $PORT/tcp
VOLUME ["/data"]
LABEL a=1 b=two
`)
	if cfg.Config.User != "1000:1000" {
		t.Errorf("USER = %q, want 1000:1000", cfg.Config.User)
	}
	if _, ok := cfg.Config.ExposedPorts["8080/tcp"]; !ok {
		t.Errorf("EXPOSE $PORT/tcp missing (have %v)", cfg.Config.ExposedPorts)
	}
	if _, ok := cfg.Config.Volumes["/data"]; !ok {
		t.Errorf("VOLUME /data missing (have %v)", cfg.Config.Volumes)
	}
	if cfg.Config.Labels["a"] != "1" || cfg.Config.Labels["b"] != "two" {
		t.Errorf("LABELs = %v", cfg.Config.Labels)
	}
}

func TestConfig_EnvChainAndArgPrecedence(t *testing.T) {
	cfg := buildImageFromDF(t, `
FROM scratch
ARG V=from-arg
ENV V=from-env
ENV NEXT=$V
`)
	// ENV overrides same-named ARG, and later ENV sees the overridden value
	if cfg.Config.Env == nil {
		t.Fatal("no env in config")
	}
	got := map[string]string{}
	for _, e := range cfg.Config.Env {
		kv := strings.SplitN(e, "=", 2)
		got[kv[0]] = kv[1]
	}
	if got["V"] != "from-env" {
		t.Errorf("ENV should override ARG: V=%q", got["V"])
	}
	if got["NEXT"] != "from-env" {
		t.Errorf("ENV chain from overridden value: NEXT=%q", got["NEXT"])
	}
}

func TestConfig_UndefinedVarExpandsEmpty(t *testing.T) {
	cfg := buildImageFromDF(t, "FROM scratch\nENV OUT=$NOPE-x\n")
	got := map[string]string{}
	for _, e := range cfg.Config.Env {
		kv := strings.SplitN(e, "=", 2)
		got[kv[0]] = kv[1]
	}
	if got["OUT"] != "-x" {
		t.Errorf("undefined var must expand to empty: OUT=%q", got["OUT"])
	}
}

// TestConfig_CopyPathUsesEnv: ENV is expanded in COPY paths (docker env
// replacement covers COPY/ADD).
func TestConfig_CopyPathUsesEnv(t *testing.T) {
	cfg := buildImageFromDF(t, `
FROM scratch
ENV DIR=/data
COPY folder $DIR/
`)
	_ = cfg
	out := filepath.Join(t.TempDir(), "out.tar")
	engine := NewEngine()
	engine.BuildDir = "./testdata"
	if err := engine.Build(strings.NewReader(`
FROM scratch
ENV DIR=/data
COPY folder $DIR/
`), OutFileOption(out, "env:test")); err != nil {
		t.Fatal(err)
	}
	img, err := tarball.ImageFromPath(out, nil)
	if err != nil {
		t.Fatal(err)
	}
	// directory contents land under $DIR
	assertImageHasFileContent(t, img, "/data/folder/file", "file content")
}

// TestConfig_CopyStripsParentNavigation: `COPY ../outside /x` strips the
// parent navigation and copies from the context root, per docker.
func TestConfig_CopyStripsParentNavigation(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out.tar")
	engine := NewEngine()
	engine.BuildDir = "./testdata"
	if err := engine.Build(strings.NewReader(`
FROM scratch
COPY ../folder/file /x
`), OutFileOption(out, "escape:test")); err != nil {
		t.Fatal(err)
	}
	img, err := tarball.ImageFromPath(out, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertImageHasFileContent(t, img, "/x", "file content")
}
