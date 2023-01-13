package pkg

import (
	"encoding/json"
	"fmt"

	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

type BuildOption func(img v1.Image) error

func prettyPrint(data any) {
	bs, _ := json.MarshalIndent(data, "", "  ")
	fmt.Println(string(bs))
}

func PrintHistoryOption() BuildOption {
	return func(img v1.Image) error {
		manifest, _ := img.Manifest()
		for _, layer := range manifest.Layers {
			prettyPrint(layer)
		}
		return nil
	}
}

func OutFileOption(outFile string, tag string) BuildOption {
	return func(img v1.Image) error {
		if outFile != "" {
			if err := crane.Save(img, tag, outFile); err != nil {
				return fmt.Errorf("writing output %q: %w", outFile, err)
			}
		}
		return nil
	}
}

func PushOption(tags []string) BuildOption {
	return func(img v1.Image) error {
		for _, tag := range tags {
			err := push(img, tag)
			if err != nil {
				return err
			}
		}
		return nil
	}
}
