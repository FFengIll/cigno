package registry

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

func newFileLayer(t *testing.T, files map[string]string) v1.Layer {
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
	bs := buf.Bytes()
	layer, err := tarball.LayerFromOpener(func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(bs)), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return layer
}

func testImage(t *testing.T) v1.Image {
	t.Helper()
	img, err := mutate.Append(empty.Image, mutate.Addendum{
		Layer: newFileLayer(t, map[string]string{"hello.txt": "world"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func TestServer_PushPull(t *testing.T) {
	srv, err := NewSimpleServer("127.0.0.1:0", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Stop() })
	addr := srv.GetAddr()

	ref := fmt.Sprintf("%s/test/app:v1", addr)
	img := testImage(t)

	if err := crane.Push(img, ref, crane.Insecure); err != nil {
		t.Fatalf("push: %v", err)
	}

	got, err := crane.Pull(ref, crane.Insecure)
	if err != nil {
		t.Fatalf("pull: %v", err)
	}

	wantDigest, err := img.Digest()
	if err != nil {
		t.Fatal(err)
	}
	gotDigest, err := got.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if wantDigest.String() != gotDigest.String() {
		t.Errorf("digest mismatch: want %s got %s", wantDigest, gotDigest)
	}

	// blob content must be on disk
	mf, err := img.Manifest()
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range mf.Layers {
		blobPath := filepath.Join(srv.GetStoragePath(), "blobs", "sha256", l.Digest.Hex)
		if _, err := os.Stat(blobPath); err != nil {
			t.Errorf("blob %s missing on disk: %v", l.Digest.Hex, err)
		}
	}
}

func TestServer_TagsList(t *testing.T) {
	srv, err := NewSimpleServer("127.0.0.1:0", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Stop() })
	addr := srv.GetAddr()

	img := testImage(t)
	for _, tag := range []string{"v1", "v2"} {
		if err := crane.Push(img, fmt.Sprintf("%s/test/app:%s", addr, tag), crane.Insecure); err != nil {
			t.Fatal(err)
		}
	}

	resp, err := http.Get(fmt.Sprintf("http://%s/v2/test/app/tags/list", addr))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tags/list status = %d", resp.StatusCode)
	}
}

func TestServer_DigestVerification(t *testing.T) {
	srv, err := NewSimpleServer("127.0.0.1:0", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Stop() })
	addr := srv.GetAddr()

	// monolithic upload with a wrong digest must fail
	req, _ := http.NewRequest(http.MethodPost,
		fmt.Sprintf("http://%s/v2/test/app/blobs/uploads/?digest=sha256:%064d", addr, 0),
		nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("wrong digest upload status = %d, want 400", resp.StatusCode)
	}
}

func TestSplitNameRef(t *testing.T) {
	cases := []struct {
		path, kind, name, ref string
		ok                    bool
	}{
		{"/v2/foo/bar/manifests/latest", "manifests", "foo/bar", "latest", true},
		{"/v2/foo/blobs/sha256:abcd", "blobs", "foo", "sha256:abcd", true}, // digest validity is checked separately
		{"/v2/", "manifests", "", "", false},
	}
	for _, c := range cases {
		name, ref, ok := splitNameRef(c.path, c.kind)
		if ok != c.ok {
			t.Errorf("splitNameRef(%q) ok = %v", c.path, ok)
			continue
		}
		if ok && (name != c.name || ref != c.ref) {
			t.Errorf("splitNameRef(%q) = %q, %q, want %q, %q", c.path, name, ref, c.name, c.ref)
		}
	}
}
