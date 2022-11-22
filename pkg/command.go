package pkg

import (
	"errors"
	"fmt"
	"github.com/docker/distribution/uuid"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/spf13/afero"
)

type Engine struct {
	BuildDir     string
	LayerType    types.MediaType
	BuildContext map[string]BuildContext
	Bases        map[string]string
}

// BuildContext describe the context for `--from` and `FROM`
type BuildContext struct {
	Type refType
	Path string
}

type refType string

const (
	ImageRef   refType = "image-ref"
	TarballRef refType = "tarball-ref"
	PathRef    refType = "path-ref"
)

func (engine *Engine) getBase(orig string) (string, error) {

	base, ok := engine.Bases[orig]
	if ok {
		return base, nil
	}
	return "", errors.New(fmt.Sprintf("no such base for origal: %s", orig))
}

func (engine *Engine) getRef(orig string) BuildContext {
	ref, ok := engine.BuildContext[orig]
	if ok {
		return ref
	}
	return BuildContext{Type: ImageRef, Path: orig}
}

func createBlob(absCtx string, dest string, sources []string) string {
	tb := NewTarball(absCtx)

	var blob afero.File
	var err error
	id := uuid.Generate()
	path := fmt.Sprintf("/tmp/%s.tar", id)
	blob, err = fs.Create(path)
	defer blob.Close()

	var options []TarOption
	err = tb.tar(blob, sources, options...)
	if err != nil {
		panic("")
	}
	blob.Close()

	return path
}

func NewTarball(ctx string) *Tarball {
	var options []TarOption
	return &Tarball{
		Root:    ctx,
		Options: options,
	}
}
