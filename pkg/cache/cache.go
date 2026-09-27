package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/sirupsen/logrus"
)

// Cache provides local disk-based image caching
// Stores images as tarball files on disk
type Cache struct {
	path string
}

// Default cache location
const DefaultCachePath = ".cigno/cache"

// New creates a new cache instance
func New(path string) (*Cache, error) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("getting home dir: %w", err)
		}
		path = filepath.Join(home, DefaultCachePath)
	}

	// Ensure cache directory exists
	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, fmt.Errorf("creating cache dir: %w", err)
	}

	return &Cache{path: path}, nil
}

// Get retrieves an image from cache by reference
// Returns (image, found, error)
func (c *Cache) Get(ref string) (v1.Image, bool, error) {
	tarPath := c.imagePath(ref)

	// Check if cached tar file exists
	if _, err := os.Stat(tarPath); os.IsNotExist(err) {
		return nil, false, nil
	}

	// Load image from tar file
	img, err := tarball.ImageFromPath(tarPath, nil)
	if err != nil {
		return nil, false, fmt.Errorf("loading image from cache: %w", err)
	}

	logrus.WithField("ref", ref).WithField("path", tarPath).Debug("cache hit")
	return img, true, nil
}

// Put stores an image in cache as a tar file
func (c *Cache) Put(img v1.Image, ref string) error {
	tarPath := c.imagePath(ref)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(tarPath), 0755); err != nil {
		return fmt.Errorf("creating cache dir: %w", err)
	}

	// Parse reference for name
	refParsed, err := name.ParseReference(ref)
	if err != nil {
		return fmt.Errorf("parsing reference: %w", err)
	}

	// Write image to tar file
	if err := tarball.WriteToFile(tarPath, refParsed, img); err != nil {
		return fmt.Errorf("writing image to tar: %w", err)
	}

	logrus.WithField("ref", ref).WithField("path", tarPath).Debug("cache write")
	return nil
}

// imagePath returns the tar file path for a reference
func (c *Cache) imagePath(ref string) string {
	// Sanitize reference for filesystem
	sanitized := sanitizeRef(ref)
	return filepath.Join(c.path, sanitized+".tar")
}

// sanitizeRef converts a reference to a filesystem-safe name
func sanitizeRef(ref string) string {
	// Replace filesystem-unsafe characters with underscores
	sanitized := ref
	sanitized = strings.ReplaceAll(sanitized, "/", "_")
	sanitized = strings.ReplaceAll(sanitized, ":", "_")
	sanitized = strings.ReplaceAll(sanitized, "\\", "_")
	return sanitized
}

// Clear removes all cached images
func (c *Cache) Clear() error {
	logrus.WithField("path", c.path).Info("clearing cache")
	return os.RemoveAll(c.path)
}

// List returns all cached image references
func (c *Cache) List() ([]string, error) {
	var refs []string

	_ = filepath.Walk(c.path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Ext(path) == ".tar" && !strings.HasPrefix(filepath.Base(path), ".") {
			refs = append(refs, strings.TrimSuffix(filepath.Base(path), ".tar"))
		}
		return nil
	})

	return refs, nil
}
