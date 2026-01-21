package pkg

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/distribution/uuid"
	"cigno/pkg/cache"
	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/moby/buildkit/frontend/dockerfile/instructions"
	"github.com/sirupsen/logrus"
	"github.com/spf13/afero"
)

type Engine struct {
	BuildDir     string
	LayerType    types.MediaType
	BuildContext map[string]*BuildContext
	GlobalArg    map[string]string
	LocalArgs    map[*instructions.Stage]map[string]string
	Env          map[*instructions.Stage]map[string]string
	Cache        *cache.Cache

	Log *logrus.Logger
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

func NewEngine() *Engine {
	return &Engine{
		BuildDir:     "",
		BuildContext: map[string]*BuildContext{},
		GlobalArg:    map[string]string{},
		LocalArgs:    map[*instructions.Stage]map[string]string{},
		Env:          map[*instructions.Stage]map[string]string{},
		Log:          logrus.New(),
	}
}

// InitCache initializes the local cache
func (engine *Engine) InitCache(cachePath string) error {
	c, err := cache.New(cachePath)
	if err != nil {
		return err
	}
	engine.Cache = c
	return nil
}

// PullImageWithCache pulls an image with cache support
// Checks local cache first, then falls back to remote pull
func (engine *Engine) PullImageWithCache(ref string) (v1.Image, error) {
	// Try cache first if enabled
	if engine.Cache != nil {
		if img, found, err := engine.Cache.Get(ref); err == nil && found {
			logrus.WithField("ref", ref).Info("cache hit")
			return img, nil
		}
		logrus.WithField("ref", ref).Debug("cache miss")
	}

	// Pull from remote
	logrus.WithField("ref", ref).Info("pulling image")
	img, err := crane.Pull(ref)
	if err != nil {
		return nil, fmt.Errorf("pulling %s: %w", ref, err)
	}

	// Store in cache for future use
	if engine.Cache != nil {
		if err := engine.Cache.Put(img, ref); err != nil {
			logrus.WithError(err).Warn("failed to cache image")
		}
	}

	return img, nil
}

func (engine *Engine) WithLog(log *logrus.Logger) {
	engine.Log = log
}

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
		realPath := engine.expandGlobalArg(ref.Path)
		return &BuildContext{Type: ref.Type, Path: realPath}
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

func (engine *Engine) AddContext(name string, image string) {
	engine.BuildContext[name] = &BuildContext{
		Type: ImageRef,
		Path: image,
	}
}

func (engine *Engine) expandGlobalArg(expr string) string {
	expr = os.Expand(expr, func(s string) string {
		if value, ok := engine.GlobalArg[s]; ok {
			return value
		}
		return ""
	})
	return expr
}

func (engine *Engine) expandArg(stage *instructions.Stage, expr string) (string, bool) {
	ok := false
	expr = os.Expand(expr, func(s string) string {
		var value string
		if value, ok = engine.LocalArgs[stage][s]; ok {
			return value
		}

		if value, ok = engine.GlobalArg[s]; ok {
			return value
		}

		return fmt.Sprintf("$%s", s)
	})
	return expr, ok
}

func (engine *Engine) AllocLocalEnv(stage *instructions.Stage, env []string) {
	kv := parseEnv(env)
	engine.Env[stage] = kv
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
	options = append(options, ReplacePrefixPath("./", ""))
	for _, src := range sources {
		options = append(options, ReplacePrefixPath(src, dest))
	}
	options = append(options, ReplacePrefixPath("/", ""))

	switch len(sources) {
	case 1:
		// ref: https://docs.docker.com/engine/reference/builder/#copy
		// for source
		source := sources[0]
		fs := afero.NewOsFs()
		if ok, _ := afero.IsDir(fs, source); ok {
			if !strings.HasSuffix(dest, "/") {
				dest += "/"
			}
			if !strings.HasSuffix(source, "/") {
				source += "/"
			}
			options = append(options, ReplacePrefixPath(source, dest))
			sources[0] = source
		} else {
			if strings.HasSuffix(dest, "/") {
				ReplacePrefixPath(filepath.Dir(source)+"/", dest)
			} else {
				ReplacePrefixPath(source, dest)
			}
		}
		break
	default:
		if !strings.HasSuffix(dest, "/") {
			dest += "/"
		}
		for _, src := range sources {
			fs := afero.NewOsFs()
			if ok, _ := afero.IsDir(fs, src); ok {
				if !strings.HasSuffix(src, "/") {
					src += "/"
				}
				ReplacePrefixPath(src, dest)
			} else {
				ReplacePrefixPath(filepath.Dir(src)+"/", dest)
			}
		}
	}

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
