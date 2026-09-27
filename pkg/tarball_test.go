package pkg

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"
)

func TestTarball_tartar(t *testing.T) {
	fs := afero.NewOsFs()

	// build context in a temp dir
	ctx := t.TempDir()
	sub := filepath.Join(ctx, "test", "data", "folder")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	var options []TarOption
	options = append(options, ChownOption(0, 0))
	options = append(options, ChownNameOption("root", "root"))
	options = append(options, PathPrefixOption("usr/local/tmp/"))

	path := filepath.Join(t.TempDir(), "test.tar")
	file, err := fs.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	tb := NewTarball(ctx)
	if err := tb.tar(file, []string{filepath.Join("test", "data", "folder")}, options...); err != nil {
		t.Fatal(err)
	}
	file.Close()

	bs, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) == 0 {
		t.Fatal("tar is empty")
	}

	// verify entries carry the prefix
	tr := tar.NewReader(bytes.NewReader(bs))
	found := false
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(hdr.Name, "f.txt") && strings.HasPrefix(hdr.Name, "usr/local/tmp/") {
			found = true
		}
	}
	if !found {
		t.Error("expected prefixed entry not found in tar")
	}
}

// TestTarball_singleFile ensures a plain file source lands in the tar.
func TestTarball_singleFile(t *testing.T) {
	ctx := t.TempDir()
	if err := os.WriteFile(filepath.Join(ctx, "marker"), []byte("m"), 0o644); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "single.tar")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	tb := NewTarball(ctx)
	if err := tb.tar(f, []string{"marker"}, ReplacePrefixPath("marker", "dest/marker")); err != nil {
		t.Fatal(err)
	}
	f.Close()

	tr := tar.NewReader(open(t, path))
	hdr, err := tr.Next()
	if err != nil {
		t.Fatal(err)
	}
	if hdr.Name != "dest/marker" {
		t.Errorf("entry = %s, want dest/marker", hdr.Name)
	}
	bs, _ := io.ReadAll(tr)
	if string(bs) != "m" {
		t.Errorf("content = %q, want m", bs)
	}
}

func TestTarball_Copy(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "in.tar")

	// build source tar with two entries
	f, err := os.Create(src)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(f)
	for _, name := range []string{"a/b.txt", "c.txt"} {
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: 1})
		tw.Write([]byte("x"))
	}
	tw.Close()
	f.Close()

	dst := filepath.Join(dir, "out.tar")
	tb := NewTarball(dir)
	if err := tb.Copy(dst, src, ReplacePrefixPath("a/", "renamed/")); err != nil {
		t.Fatal(err)
	}

	tr := tar.NewReader(open(t, dst))
	names := map[string]bool{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names[hdr.Name] = true
	}
	if !names["renamed/b.txt"] || !names["c.txt"] {
		t.Errorf("unexpected entries: %v", names)
	}
}

func open(t *testing.T, path string) *os.File {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return f
}
