package pkg

import (
	"fmt"
	"strings"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

// craneOptions returns crane options for a ref, enabling plain HTTP for
// well-known local registries (localhost / 127.0.0.1) so the embedded
// registry can be used without TLS setup.
func craneOptions(ref string) []crane.Option {
	parsed, err := name.ParseReference(ref, name.WeakValidation)
	if err != nil {
		return nil
	}
	reg := parsed.Context().RegistryStr()
	if reg == "localhost" || strings.HasPrefix(reg, "127.0.0.1") || strings.HasPrefix(reg, "::1") {
		return []crane.Option{crane.Insecure}
	}
	return nil
}

func push(img v1.Image, dst string) error {
	if err := crane.Push(img, dst, craneOptions(dst)...); err != nil {
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
	return nil
}
