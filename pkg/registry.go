package pkg

import (
	"fmt"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

func push(img v1.Image, dst string) error {
	var options []crane.Option
	if err := crane.Push(img, dst, options...); err != nil {
		return fmt.Errorf("pushing image %s: %w", dst, err)
	}
	ref, err := name.ParseReference(dst)
	if err != nil {
		return fmt.Errorf("parsing reference %s: %w", dst, err)
	}
	d, err := img.Digest()
	if err != nil {
		return fmt.Errorf("digest: %w", err)
	}
	fmt.Println(ref.Context().Digest(d.String()))
	// }
	return nil
}
