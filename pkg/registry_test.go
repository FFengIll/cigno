package pkg

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

func saveTestImage(t *testing.T) v1.Image {
	t.Helper()
	img, err := mutate.Append(empty.Image, mutate.Addendum{
		Layer: newTarLayer(t, map[string]string{"hello.txt": "world"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func digestOf(t *testing.T, img v1.Image) string {
	t.Helper()
	d, err := img.Digest()
	if err != nil {
		t.Fatal(err)
	}
	return d.String()
}

func TestSaveLoadImage_RoundTrip(t *testing.T) {
	addr := startTestRegistry(t)
	src := fmt.Sprintf("%s/test/app:v1", addr)
	img := saveTestImage(t)
	if err := crane.Push(img, src, crane.Insecure); err != nil {
		t.Fatalf("push: %v", err)
	}

	// save: registry -> tar
	archive := filepath.Join(t.TempDir(), "app.tar")
	if err := SaveImage(src, archive); err != nil {
		t.Fatalf("SaveImage: %v", err)
	}

	// the archive must load back as the same image
	stored, err := tarball.ImageFromPath(archive, nil)
	if err != nil {
		t.Fatalf("loading archive: %v", err)
	}
	if d := digestOf(t, stored); d != digestOf(t, img) {
		t.Errorf("digest mismatch: want %s got %s", digestOf(t, img), d)
	}

	// load: tar -> registry under a new tag
	dst := fmt.Sprintf("%s/test/app:v2", addr)
	if err := LoadImage(archive, dst); err != nil {
		t.Fatalf("LoadImage: %v", err)
	}
	got, err := crane.Pull(dst, crane.Insecure)
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if d := digestOf(t, got); d != digestOf(t, img) {
		t.Errorf("digest mismatch: want %s got %s", digestOf(t, img), d)
	}
}

func TestLoadImage_DockerSaveArchive(t *testing.T) {
	addr := startTestRegistry(t)
	img := saveTestImage(t)

	// an archive produced by `docker save` (legacy format, tarball.Write)
	archive := filepath.Join(t.TempDir(), "app.tar")
	ref, err := name.ParseReference("origin/app:v1")
	if err != nil {
		t.Fatal(err)
	}
	if err := tarball.WriteToFile(archive, ref, img); err != nil {
		t.Fatalf("writing archive: %v", err)
	}

	dst := fmt.Sprintf("%s/test/app:v1", addr)
	if err := LoadImage(archive, dst); err != nil {
		t.Fatalf("LoadImage: %v", err)
	}
	got, err := crane.Pull(dst, crane.Insecure)
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if d := digestOf(t, got); d != digestOf(t, img) {
		t.Errorf("digest mismatch: want %s got %s", digestOf(t, img), d)
	}
}

func TestLoadImage_MissingFile(t *testing.T) {
	addr := startTestRegistry(t)
	err := LoadImage(filepath.Join(t.TempDir(), "nope.tar"), fmt.Sprintf("%s/test/app:v1", addr))
	if err == nil {
		t.Fatal("LoadImage with missing file should fail")
	}
}
