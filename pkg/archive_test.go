package pkg

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ulikunitz/xz"
)

// makeTarBytes builds an uncompressed tar with the given files.
func makeTarBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func compressBytes(t *testing.T, data []byte, algo string) []byte {
	t.Helper()
	var buf bytes.Buffer
	var w io.WriteCloser
	switch algo {
	case "gzip":
		w = gzip.NewWriter(&buf)
	case "xz":
		xw, err := xz.NewWriter(&buf)
		if err != nil {
			t.Fatal(err)
		}
		w = xw
	case "none":
		return data
	default:
		t.Fatalf("unsupported test compression %q", algo)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestExtractTar_Compressed walks the matrix: tar/tar.gz/tgz-like/bz2/xz
// must all extract; identification is by content, not extension.
func TestExtractTar_Compressed(t *testing.T) {
	inner := makeTarBytes(t, map[string]string{
		"bin/app":  "binary",
		"etc/conf": "config",
	})

	cases := []struct {
		algo        string
		ext         string
		noExtension bool
	}{
		{"none", ".tar", false},
		{"gzip", ".tar.gz", false},
		{"gzip", ".tgz", false},
		{"gzip", "", true},      // misnamed file — magic bytes decide
		{"gzip", ".tar", false}, // lying extension — still extracts
		{"xz", ".tar.xz", false},
		// bzip2 omitted: compress/bzip2 provides no writer for fixtures;
		// the reader path is exercised by the production code via stdlib
	}
	for _, tc := range cases {
		t.Run(tc.algo+tc.ext, func(t *testing.T) {
			dir := t.TempDir()
			name := "archive" + tc.ext
			if tc.noExtension {
				name = "artifact"
			}
			src := filepath.Join(dir, name)
			if err := os.WriteFile(src, compressBytes(t, inner, tc.algo), 0o644); err != nil {
				t.Fatal(err)
			}

			engine := NewEngine()
			engine.BuildDir = dir
			blob, err := engine.extractTar(name, "/opt", nil)
			if err != nil {
				t.Fatal(err)
			}
			files := tarFileSet(t, blob)
			if files["opt/bin/app"] != "binary" || files["opt/etc/conf"] != "config" {
				t.Errorf("unexpected extraction: %v", files)
			}
		})
	}
}

// TestExtractTar_NotTar keeps compressed non-tar files as plain files.
func TestExtractTar_NotTar(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "notes.txt.gz")
	if err := os.WriteFile(src, compressBytes(t, []byte("hello notes"), "gzip"), 0o644); err != nil {
		t.Fatal(err)
	}

	engine := NewEngine()
	engine.BuildDir = dir
	blob, err := engine.extractTar("notes.txt.gz", "/docs", nil)
	if err != nil {
		t.Fatal(err)
	}
	files := tarFileSet(t, blob)
	// single-file ADD with a non-existent dest renames the file to dest;
	// a compressed non-tar file is added verbatim (still compressed)
	want := compressBytes(t, []byte("hello notes"), "gzip")
	got, ok := files["docs"]
	if !ok || string(want) != got {
		t.Errorf("compressed non-tar must be added verbatim, got: %q", got)
	}
}

// TestExtractTar_TraversalRejected refuses archive entries escaping dest.
func TestExtractTar_TraversalRejected(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{Name: "../evil", Mode: 0o644, Size: 2}); err != nil {
		t.Fatal(err)
	}
	tw.Write([]byte("no"))
	tw.Close()
	evilBytes := buf.Bytes()

	src := filepath.Join(dir, "evil.tar.gz")
	if err := os.WriteFile(src, compressBytes(t, evilBytes, "gzip"), 0o644); err != nil {
		t.Fatal(err)
	}

	engine := NewEngine()
	engine.BuildDir = dir
	if _, err := engine.extractTar("evil.tar.gz", "/opt", nil); err == nil {
		t.Fatal("path traversal must be rejected")
	}
}

// TestExtractTar_WhiteoutSkipped drops whiteout markers.
func TestExtractTar_WhiteoutSkipped(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	tw.WriteHeader(&tar.Header{Name: "keep.txt", Mode: 0o644, Size: 3})
	tw.Write([]byte("yes"))
	tw.WriteHeader(&tar.Header{Name: ".wh.drop.txt", Typeflag: tar.TypeReg, Mode: 0o644, Size: 0})
	tw.Close()

	src := filepath.Join(dir, "arch.tar")
	if err := os.WriteFile(src, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	engine := NewEngine()
	engine.BuildDir = dir
	blob, err := engine.extractTar("arch.tar", "/opt", nil)
	if err != nil {
		t.Fatal(err)
	}
	files := tarFileSet(t, blob)
	if files["opt/keep.txt"] != "yes" {
		t.Errorf("missing keep.txt: %v", files)
	}
	for name := range files {
		if strings.HasPrefix(filepath.Base(name), ".wh.") {
			t.Errorf("whiteout marker leaked: %s", name)
		}
	}
}

// tarFileSet reads a blob tar produced by extractTar into name->content.
func tarFileSet(t *testing.T, blob string) map[string]string {
	t.Helper()
	f, err := os.Open(blob)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	files := map[string]string{}
	tr := tar.NewReader(f)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if hdr.Typeflag == tar.TypeReg {
			bs, _ := io.ReadAll(tr)
			files[strings.TrimPrefix(hdr.Name, "/")] = string(bs)
		} else {
			files[strings.TrimPrefix(hdr.Name, "/")] = ""
		}
	}
	return files
}
