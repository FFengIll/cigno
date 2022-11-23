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
	BuildContext map[string]*BuildContext
}

// BuildContext describe the context for `--from` and `FROM`
type BuildContext struct {
	Type refType
	Path string
	Base string
}

type refType string

const (
	ImageRef   refType = "image-ref"
	TarballRef refType = "tarball-ref"
	PathRef    refType = "path-ref"
)

func (engine *Engine) getBase(orig string) (string, error) {
	item, ok := engine.BuildContext[orig]
	if ok {
		return item.Base, nil
	}
	return "", errors.New(fmt.Sprintf("no such base for origal: %s", orig))
}

func (engine *Engine) getRef(orig string) *BuildContext {
	ref, ok := engine.BuildContext[orig]
	if ok {
		return ref
	}
	return &BuildContext{Type: ImageRef, Path: orig}
}

func (engine *Engine) AddBase(image string, base string) {
	engine.BuildContext[image] = &BuildContext{
		Type: ImageRef,
		Path: image,
		Base: base,
	}
}

func (engine *Engine) AddTarball(name string, path string) {
	engine.BuildContext[name] = &BuildContext{
		Type: TarballRef,
		Path: path,
		Base: "",
	}
}

func (engine *Engine) AddFolder(name string, path string) {
	engine.BuildContext[name] = &BuildContext{
		Type: PathRef,
		Path: path,
		Base: "",
	}
}

func (engine *Engine) AddImage(stage string, image string) {
	engine.BuildContext[stage] = &BuildContext{
		Type: ImageRef,
		Path: image,
	}
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

func copyBlob(absCtx string, origin string, options ...TarOption) (string, error) {
	tb := NewTarball(absCtx)

	var err error
	var blob afero.File
	id := uuid.Generate()
	path := fmt.Sprintf("/tmp/%s.tar", id)
	blob, err = fs.Create(path)
	defer blob.Close()

	err = tb.Copy(path, origin, options...)
	if err != nil {
		return "", err
	}

	return path, nil
}

func NewTarball(ctx string) *Tarball {
	var options []TarOption
	return &Tarball{
		Root:       ctx,
		PreOptions: options,
	}
}
