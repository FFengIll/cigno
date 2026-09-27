package cache

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

func TestSanitizeRef(t *testing.T) {
	cases := map[string]string{
		"localhost:5000/foo/bar:latest": "localhost_5000_foo_bar_latest",
		"alpine:3.17":                   "alpine_3.17",
	}
	for in, want := range cases {
		if got := sanitizeRef(in); got != want {
			t.Errorf("sanitizeRef(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCache_PutGet(t *testing.T) {
	dir := t.TempDir()
	c, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	// write a real (empty) image tar at the expected cache path
	ref := "localhost:5000/foo:latest"
	tarPath := c.imagePath(ref)
	if err := tarball.WriteToFile(tarPath, mustRef(t, ref), empty.Image); err != nil {
		t.Fatal(err)
	}

	_, found, err := c.Get(ref)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Error("expected cache hit")
	}

	_, found, err = c.Get("missing:ref")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("expected cache miss")
	}
}

func mustRef(t *testing.T, ref string) name.Reference {
	t.Helper()
	r, err := name.ParseReference(ref)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestCache_ListClear(t *testing.T) {
	c, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ref := "alpine:3.17"
	tarPath := c.imagePath(ref)
	if err := os.MkdirAll(filepath.Dir(tarPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tarPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	refs, err := c.List()
	if err != nil {
		t.Fatal(err)
	}
	// List returns sanitized on-disk names
	if len(refs) != 1 || refs[0] != sanitizeRef(ref) {
		t.Errorf("List() = %v, want [%s]", refs, sanitizeRef(ref))
	}

	if err := c.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(c.path); !os.IsNotExist(err) {
		t.Error("cache dir still exists after Clear")
	}
}
