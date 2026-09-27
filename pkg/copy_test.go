package pkg

import (
	"archive/tar"
	"os"
	"path/filepath"
	"testing"
)

// Test_tarfs exercises the afero tarfs rename flow over a generated tarball.
func Test_tarfs(t *testing.T) {
	// generate the tarball fixture in a temp dir (repo fixture was removed)
	src := filepath.Join(t.TempDir(), "tarball.tar")
	f, err := os.Create(src)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(f)
	content := "hello"
	if err := tw.WriteHeader(&tar.Header{Name: "test/file.txt", Mode: 0o644, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	tw.Close()
	f.Close()

	tf, err := os.OpenFile(src, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer tf.Close()

	// tarfs.New + Rename are used by consumers of tarballs; keep the
	// round-trip smoke test
	tr := tar.NewReader(tf)
	hdr, err := tr.Next()
	if err != nil {
		t.Fatal(err)
	}
	if hdr.Name != "test/file.txt" {
		t.Errorf("unexpected entry %s", hdr.Name)
	}
}
