package pkg

import (
	"github.com/moby/buildkit/frontend/dockerfile/instructions"
	"github.com/moby/buildkit/frontend/dockerfile/parser"
	"io"
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
