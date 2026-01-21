package pkg

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/moby/buildkit/frontend/dockerfile/instructions"
	"github.com/sirupsen/logrus"
)

// Validator defines validation interface
type Validator interface {
	Validate() error
}

// BaseImageValidator validates base image references
type BaseImageValidator struct {
	BasePath string
}

func (v *BaseImageValidator) Validate() error {
	if v.BasePath == "" {
		return fmt.Errorf("base image cannot be empty")
	}

	// Check if it's a valid image reference
	// Try to parse as a reference
	_, err := name.ParseReference(v.BasePath, name.WeakValidation)
	if err != nil {
		return fmt.Errorf("invalid base image reference %q: %w", v.BasePath, err)
	}

	return nil
}

// CopySourceValidator validates COPY sources
type CopySourceValidator struct {
	Sources    []string
	From       string
	BuildContext string
}

func (v *CopySourceValidator) Validate() error {
	// If --from is used, validate it exists
	if v.From != "" {
		// Validate the reference exists in build context
		if v.From != "" {
			// This will be validated when used
			logrus.WithField("from", v.From).Debug("COPY --from will be validated during execution")
		}
	}

	// For local sources, check they exist in build context
	for _, src := range v.Sources {
		// Skip wildcards for now
		if containsWildcard(src) {
			continue
		}

		fullPath := src
		if v.BuildContext != "" && !filepath.IsAbs(src) {
			fullPath = fmt.Sprintf("%s/%s", v.BuildContext, src)
		}

		if _, err := os.Stat(fullPath); err != nil {
			return fmt.Errorf("COPY source %q not found: %w", src, err)
		}
	}

	return nil
}

// validateConfig validates image config
func validateConfig(cfg *v1.ConfigFile) error {
	// Basic validation
	if cfg == nil {
		return fmt.Errorf("config cannot be nil")
	}

	// Config is a struct, not a pointer, so we just validate the ConfigFile exists
	return nil
}

// ValidateStage validates a stage configuration
func ValidateStage(stage *instructions.Stage, basePath string) error {
	if stage == nil {
		return fmt.Errorf("stage cannot be nil")
	}

	// Validate base image
	baseValidator := &BaseImageValidator{BasePath: basePath}
	if err := baseValidator.Validate(); err != nil {
		return fmt.Errorf("base image validation failed: %w", err)
	}

	return nil
}

// containsWildcard checks if a path contains wildcard characters
func containsWildcard(path string) bool {
	for _, c := range path {
		if c == '*' || c == '?' || c == '[' {
			return true
		}
	}
	return false
}
