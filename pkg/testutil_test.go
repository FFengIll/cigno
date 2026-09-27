package pkg

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

// newTarLayer builds an uncompressed tar layer containing the given files.
func newTarLayer(t *testing.T, files map[string]string) v1.Layer {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	bs := buf.Bytes()
	layer, err := tarball.LayerFromOpener(func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(bs)), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return layer
}

// extractFileSet flattens the image filesystem into a set of file paths.
func extractFileSet(t *testing.T, img v1.Image) map[string]string {
	t.Helper()
	r := mutateExtract(img)
	defer r.Close()
	files := map[string]string{}
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		name := strings.TrimPrefix(hdr.Name, "./")
		name = strings.TrimPrefix(name, "/")
		if hdr.Typeflag == tar.TypeReg {
			var buf bytes.Buffer
			if _, err := io.Copy(&buf, tr); err != nil {
				t.Fatal(err)
			}
			files[name] = buf.String()
		} else {
			files[name] = ""
		}
	}
	return files
}

func mutateExtract(img v1.Image) io.ReadCloser {
	return mutate.Extract(img)
}

// assertImageHasFile checks a file exists in the image with the given content.
func assertImageHasFileContent(t *testing.T, img v1.Image, path, want string) {
	t.Helper()
	files := extractFileSet(t, img)
	got, ok := files[strings.TrimPrefix(path, "/")]
	if !ok {
		t.Fatalf("file %s not found in image; have: %v", path, keys(files))
	}
	if want != "" && got != want {
		t.Errorf("file %s content = %q, want %q", path, got, want)
	}
}

func keys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

var _ = fmt.Sprintf
