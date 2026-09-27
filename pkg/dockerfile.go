package pkg

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/moby/buildkit/frontend/dockerfile/instructions"
	"github.com/moby/buildkit/frontend/dockerfile/parser"
)

func ParseDockerFile(r io.Reader) ([]instructions.Stage, []instructions.ArgCommand, error) {
	p, err := parser.Parse(r)
	if err != nil {
		return nil, nil, err
	}
	stages, metaArgs, err := instructions.Parse(p.AST)
	if err != nil {
		return nil, nil, err
	}

	// metaArgs, err = stripEnclosingQuotes(metaArgs)
	// if err != nil {
	// 	return nil, nil, err
	// }

	return stages, metaArgs, nil
}

// ValidateDockerfile parses the dockerfile at path without building.
func ValidateDockerfile(path string) error {
	bs, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("opening dockerfile %s: %w", path, err)
	}
	stages, _, err := ParseDockerFile(bytes.NewReader(bs))
	if err != nil {
		return fmt.Errorf("invalid dockerfile %s: %w", path, err)
	}
	fmt.Printf("validated %d stage(s) in %s\n", len(stages), path)
	return nil
}
