// Package registry provides an embedded OCI distribution (registry v2) server
// with filesystem-based storage. It is used as a local cache / push target so
// that builds never require an external registry.
package registry

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// Server is a minimal registry v2 server backed by the local filesystem.
//
// Storage layout (relative to the storage root):
//
//	blobs/<algo>/<hex>                 blob content
//	repos/<name>/manifests/<ref>       manifest content (tag or digest)
//	repos/<name>/uploads/<uuid>        in-progress blob uploads
type Server struct {
	addr     string
	storage  string
	listener net.Listener
	server   *http.Server
	mu       sync.Mutex
}

// NewSimpleServer creates a registry server bound to addr with storage at path.
// The storage directory is created if missing.
func NewSimpleServer(addr, storage string) (*Server, error) {
	if addr == "" {
		addr = "localhost:5000"
	}
	if storage == "" {
		return nil, fmt.Errorf("storage path cannot be empty")
	}
	if err := os.MkdirAll(storage, 0o755); err != nil {
		return nil, fmt.Errorf("creating storage dir %s: %w", storage, err)
	}
	return &Server{addr: addr, storage: storage}, nil
}

// Start starts serving in the background. It returns an error only if the
// listener cannot be bound; requests are then served asynchronously.
func (s *Server) Start() error {
	l, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", s.addr, err)
	}
	s.mu.Lock()
	s.listener = l
	s.addr = l.Addr().String()
	s.mu.Unlock()

	mux := http.NewServeMux()
	mux.HandleFunc("/v2/", s.handleV2)
	srv := &http.Server{Handler: mux}
	s.mu.Lock()
	s.server = srv
	s.mu.Unlock()

	go func() {
		_ = srv.Serve(l)
	}()
	return nil
}

// Stop gracefully shuts the server down.
func (s *Server) Stop() error {
	s.mu.Lock()
	srv := s.server
	s.mu.Unlock()
	if srv == nil {
		return nil
	}
	return srv.Close()
}

// GetAddr returns the address the server listens on.
func (s *Server) GetAddr() string {
	return s.addr
}

// GetStoragePath returns the filesystem storage root.
func (s *Server) GetStoragePath() string {
	return s.storage
}

// newUUID returns a random hex id for upload sessions.
func newUUID() string {
	bs := make([]byte, 16)
	_, _ = rand.Read(bs)
	return hex.EncodeToString(bs)
}

var (
	nameRe   = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)*$`)
	digestRe = regexp.MustCompile(`^[a-z0-9]+(?:[.+_-][a-z0-9]+)*:[a-f0-9]{64}$`)
)

func errJSON(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"errors": []map[string]string{{"code": code, "message": message}},
	})
}

// splitNameRef splits "/v2/<name>/blobs|manifests/<ref>" into name and ref.
func splitNameRef(path, kind string) (name, ref string, ok bool) {
	rest := strings.TrimPrefix(path, "/v2/")
	idx := strings.LastIndex(rest, "/"+kind+"/")
	if idx <= 0 {
		return "", "", false
	}
	name = rest[:idx]
	ref = rest[idx+len(kind)+2:]
	if name == "" || ref == "" || !nameRe.MatchString(name) {
		return "", "", false
	}
	return name, ref, true
}

func (s *Server) handleV2(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/v2/" || r.URL.Path == "/v2":
		w.WriteHeader(http.StatusOK)
		return
	case strings.Contains(r.URL.Path, "/blobs/uploads"):
		s.handleBlobUpload(w, r)
		return
	case strings.Contains(r.URL.Path, "/blobs/"):
		s.handleBlob(w, r)
		return
	case strings.Contains(r.URL.Path, "/manifests/"):
		s.handleManifest(w, r)
		return
	case strings.HasSuffix(r.URL.Path, "/tags/list"):
		s.handleTags(w, r)
		return
	}
	errJSON(w, http.StatusNotFound, "NAME_UNKNOWN", "unknown route")
}

func (s *Server) handleBlobUpload(w http.ResponseWriter, r *http.Request) {
	name, _, ok := splitNameRef(r.URL.Path, "blobs")
	if !ok {
		errJSON(w, http.StatusNotFound, "NAME_UNKNOWN", "invalid blob upload path")
		return
	}

	switch r.Method {
	case http.MethodPost:
		// Monolithic upload: POST /v2/<name>/blobs/uploads/?digest=<digest>
		if d := r.URL.Query().Get("digest"); d != "" {
			s.finishBlob(w, r, name, d)
			return
		}
		// Init a chunked upload session.
		id := newUUID()
		path := s.uploadPath(name, id)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			errJSON(w, http.StatusInternalServerError, "UNKNOWN", err.Error())
			return
		}
		f, err := os.Create(path)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, "UNKNOWN", err.Error())
			return
		}
		f.Close()
		w.Header().Set("Location", fmt.Sprintf("/v2/%s/blobs/uploads/%s", name, id))
		w.Header().Set("Docker-Upload-UUID", id)
		w.WriteHeader(http.StatusAccepted)
	case http.MethodPatch:
		// Append a chunk: PATCH /v2/<name>/blobs/uploads/<uuid>
		_, id, ok := splitUpload(r.URL.Path, name)
		if !ok {
			errJSON(w, http.StatusNotFound, "BLOB_UPLOAD_UNKNOWN", "unknown upload session")
			return
		}
		path := s.uploadPath(name, id)
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			errJSON(w, http.StatusNotFound, "BLOB_UPLOAD_UNKNOWN", "unknown upload session")
			return
		}
		defer f.Close()
		if _, err := io.Copy(f, r.Body); err != nil {
			errJSON(w, http.StatusInternalServerError, "UNKNOWN", err.Error())
			return
		}
		fi, _ := f.Stat()
		w.Header().Set("Location", fmt.Sprintf("/v2/%s/blobs/uploads/%s", name, id))
		w.Header().Set("Range", fmt.Sprintf("0-%d", fi.Size()-1))
		w.Header().Set("Docker-Upload-UUID", id)
		w.WriteHeader(http.StatusAccepted)
	case http.MethodPut:
		// Finalize: PUT /v2/<name>/blobs/uploads/<uuid>?digest=<digest>
		s.finishBlob(w, r, name, r.URL.Query().Get("digest"))
	case http.MethodDelete:
		_, id, ok := splitUpload(r.URL.Path, name)
		if !ok {
			errJSON(w, http.StatusNotFound, "BLOB_UPLOAD_UNKNOWN", "unknown upload session")
			return
		}
		if err := os.Remove(s.uploadPath(name, id)); err != nil {
			errJSON(w, http.StatusNotFound, "BLOB_UPLOAD_UNKNOWN", "unknown upload session")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		errJSON(w, http.StatusMethodNotAllowed, "UNSUPPORTED", "method not allowed")
	}
}

// finishBlob moves the uploaded data to its final blob location after
// verifying the sha256 digest.
func (s *Server) finishBlob(w http.ResponseWriter, r *http.Request, name, digest string) {
	if !digestRe.MatchString(digest) {
		errJSON(w, http.StatusBadRequest, "DIGEST_INVALID", "invalid digest "+digest)
		return
	}
	_, id, hasSession := splitUpload(r.URL.Path, name)

	// Assemble content: existing session data plus any body on the PUT.
	var content []byte
	if hasSession {
		prev, err := os.ReadFile(s.uploadPath(name, id))
		if err != nil {
			errJSON(w, http.StatusNotFound, "BLOB_UPLOAD_UNKNOWN", "unknown upload session")
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, "UNKNOWN", err.Error())
			return
		}
		content = append(prev, body...)
		_ = os.Remove(s.uploadPath(name, id))
	} else {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, "UNKNOWN", err.Error())
			return
		}
		content = body
	}

	sum := sha256.Sum256(content)
	got := fmt.Sprintf("sha256:%x", sum)
	if got != digest {
		errJSON(w, http.StatusBadRequest, "DIGEST_INVALID", fmt.Sprintf("digest mismatch: got %s want %s", got, digest))
		return
	}

	blobPath := s.blobPath(digest)
	if err := os.MkdirAll(filepath.Dir(blobPath), 0o755); err != nil {
		errJSON(w, http.StatusInternalServerError, "UNKNOWN", err.Error())
		return
	}
	if err := os.WriteFile(blobPath, content, 0o644); err != nil {
		errJSON(w, http.StatusInternalServerError, "UNKNOWN", err.Error())
		return
	}
	w.Header().Set("Docker-Content-Digest", digest)
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) handleBlob(w http.ResponseWriter, r *http.Request) {
	_, ref, ok := splitNameRef(r.URL.Path, "blobs")
	if !ok || !digestRe.MatchString(ref) {
		errJSON(w, http.StatusNotFound, "DIGEST_INVALID", "invalid digest")
		return
	}
	blobPath := s.blobPath(ref)
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		f, err := os.Open(blobPath)
		if err != nil {
			errJSON(w, http.StatusNotFound, "BLOB_UNKNOWN", "blob not found")
			return
		}
		defer f.Close()
		fi, _ := f.Stat()
		w.Header().Set("Content-Length", fmt.Sprintf("%d", fi.Size()))
		w.Header().Set("Docker-Content-Digest", ref)
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodGet {
			_, _ = io.Copy(w, f)
		}
	case http.MethodDelete:
		if err := os.Remove(blobPath); err != nil {
			errJSON(w, http.StatusNotFound, "BLOB_UNKNOWN", "blob not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		errJSON(w, http.StatusMethodNotAllowed, "UNSUPPORTED", "method not allowed")
	}
}

func (s *Server) handleManifest(w http.ResponseWriter, r *http.Request) {
	name, ref, ok := splitNameRef(r.URL.Path, "manifests")
	if !ok {
		errJSON(w, http.StatusNotFound, "NAME_UNKNOWN", "invalid manifest path")
		return
	}
	manifestPath := s.manifestPath(name, ref)
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		bs, err := os.ReadFile(manifestPath)
		if err != nil {
			errJSON(w, http.StatusNotFound, "MANIFEST_UNKNOWN", "manifest not found")
			return
		}
		digest := fmt.Sprintf("sha256:%x", sha256.Sum256(bs))
		w.Header().Set("Docker-Content-Digest", digest)
		w.Header().Set("Content-Type", r.Header.Get("Accept"))
		if ct := detectContentType(bs); ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(bs)))
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodGet {
			_, _ = w.Write(bs)
		}
	case http.MethodPut:
		bs, err := io.ReadAll(r.Body)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, "UNKNOWN", err.Error())
			return
		}
		if len(bs) == 0 {
			errJSON(w, http.StatusBadRequest, "MANIFEST_INVALID", "empty manifest")
			return
		}
		if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
			errJSON(w, http.StatusInternalServerError, "UNKNOWN", err.Error())
			return
		}
		if err := os.WriteFile(manifestPath, bs, 0o644); err != nil {
			errJSON(w, http.StatusInternalServerError, "UNKNOWN", err.Error())
			return
		}
		// Also store by digest so digest-based pulls work.
		digest := fmt.Sprintf("sha256:%x", sha256.Sum256(bs))
		_ = os.MkdirAll(filepath.Dir(s.manifestPath(name, digest)), 0o755)
		_ = os.WriteFile(s.manifestPath(name, digest), bs, 0o644)
		w.Header().Set("Docker-Content-Digest", digest)
		w.WriteHeader(http.StatusCreated)
	case http.MethodDelete:
		if err := os.Remove(manifestPath); err != nil {
			errJSON(w, http.StatusNotFound, "MANIFEST_UNKNOWN", "manifest not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		errJSON(w, http.StatusMethodNotAllowed, "UNSUPPORTED", "method not allowed")
	}
}

func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v2/"), "/tags/list")
	if name == "" || !nameRe.MatchString(name) {
		errJSON(w, http.StatusNotFound, "NAME_UNKNOWN", "invalid repository name")
		return
	}
	entries, err := os.ReadDir(s.manifestDir(name))
	tags := []string{}
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				tags = append(tags, e.Name())
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"name": name, "tags": tags})
}

// detectContentType returns the media type of a manifest/image-index payload.
func detectContentType(bs []byte) string {
	var probe struct {
		MediaType string `json:"mediaType"`
	}
	if err := json.Unmarshal(bs, &probe); err == nil && probe.MediaType != "" {
		return probe.MediaType
	}
	return "application/vnd.docker.distribution.manifest.v2+json"
}

func (s *Server) blobPath(digest string) string {
	// digest is "sha256:<hex>"
	return filepath.Join(s.storage, "blobs", strings.ReplaceAll(digest, ":", "/"))
}

func (s *Server) manifestDir(name string) string {
	return filepath.Join(s.storage, "repos", filepath.FromSlash(name), "manifests")
}

func (s *Server) manifestPath(name, ref string) string {
	// refs are either tags (safe chars) or digests ("sha256:<hex>")
	return filepath.Join(s.manifestDir(name), strings.ReplaceAll(ref, ":", "_"))
}

func (s *Server) uploadPath(name, id string) string {
	return filepath.Join(s.storage, "repos", filepath.FromSlash(name), "uploads", id)
}

// splitUpload extracts the upload uuid from "/v2/<name>/blobs/uploads/<uuid>".
func splitUpload(path, name string) (string, string, bool) {
	prefix := "/v2/" + name + "/blobs/uploads/"
	if !strings.HasPrefix(path, prefix) {
		return "", "", false
	}
	id := strings.TrimPrefix(path, prefix)
	if id == "" || strings.Contains(id, "/") {
		return "", "", false
	}
	return name, id, true
}
