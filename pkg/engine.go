package pkg

import (
	"fmt"
	"os"
	gopath "path"
	"path/filepath"
	"strings"

	"cigno/pkg/cache"
	"github.com/docker/distribution/uuid"
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

	// BuiltStages holds the finished image of each named stage, so that
	// `COPY --from=<stage>` copies from the stage's built state.
	BuiltStages map[string]v1.Image

	// curStage is the stage currently being built, used to expand
	// stage-scoped args in instruction handlers that have no stage param.
	curStage *instructions.Stage

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
	ImageRef refType = "image-ref"
	PathRef  refType = "path-ref"
)

func NewEngine() *Engine {
	return &Engine{
		BuildDir:     "",
		BuildContext: map[string]*BuildContext{},
		GlobalArg:    map[string]string{},
		LocalArgs:    map[*instructions.Stage]map[string]string{},
		Env:          map[*instructions.Stage]map[string]string{},
		BuiltStages:  map[string]v1.Image{},
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
	img, err := crane.Pull(ref, craneOptions(ref)...)
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
	return "", fmt.Errorf("no such base for original: %s", orig)
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

		// docker: prior to its definition (or when undefined), a variable
		// reference expands to the empty string
		ok = true
		return ""
	})
	return expr, ok
}

// expandCurArg expands variables against the stage currently being built.
// Per docker semantics: ENV values override same-named ARG values, and
// references to undefined variables expand to the empty string.
func (engine *Engine) expandCurArg(expr string) string {
	if engine.curStage == nil {
		return engine.expandGlobalArg(expr)
	}
	return engine.expandEnvIn(expr, engine.Env[engine.curStage])
}

// expandEnvIn expands $VAR/${VAR} against env first (ENV overrides ARG),
// then stage-local and global args; undefined names become "".
func (engine *Engine) expandEnvIn(expr string, env map[string]string) string {
	lookup := func(name string) (string, bool) {
		if v, ok := env[name]; ok {
			return v, true
		}
		if engine.curStage != nil {
			if v, ok := engine.LocalArgs[engine.curStage][name]; ok {
				return v, true
			}
		}
		v, ok := engine.GlobalArg[name]
		return v, ok
	}
	return os.Expand(expr, func(s string) string {
		v, _ := lookup(s)
		return v
	})
}

func (engine *Engine) AllocLocalEnv(stage *instructions.Stage, env []string) {
	kv := parseEnv(env)
	if engine.Env == nil {
		engine.Env = map[*instructions.Stage]map[string]string{}
	}
	engine.Env[stage] = kv
}

// createBlob archives the given sources (relative to absCtx) into a temp tar
// file, applying Docker COPY path semantics for dest:
//   - single dir source: contents of the dir go under dest
//   - single file source: file lands at dest (or dest/basename if dest is a dir)
//   - multiple sources: dest must act as a directory
func createBlob(absCtx string, dest string, sources []string, extra ...TarOption) (string, error) {
	tb := NewTarball(absCtx)

	// honor .dockerignore from the build context root
	skip, err := newDockerIgnoreMatcher(absCtx)
	if err != nil {
		return "", err
	}
	tb.Skip = skip

	id := uuid.Generate()
	path := fmt.Sprintf("/tmp/%s.tar", id)
	blob, err := fs.Create(path)
	if err != nil {
		return "", fmt.Errorf("creating blob %s: %w", path, err)
	}
	defer blob.Close()

	fs := afero.NewOsFs()
	isDir := func(p string) bool {
		ok, _ := afero.IsDir(fs, p)
		return ok
	}

	var options []TarOption
	options = append(options, ReplacePrefixPath("./", ""))
	// dirMapping builds the header-name remap for a directory source:
	// regular dirs remap their prefix; the context root (".") has no
	// prefix, so entries just get the dest path prepended.
	dirMapping := func(src, dest string) TarOption {
		if src == "." || src == "./" {
			return PathPrefixOption(strings.Trim(dest, "/"))
		}
		return ReplacePrefixPath(src, dest)
	}
	// ref: https://docs.docker.com/engine/reference/builder/#copy
	switch len(sources) {
	case 1:
		source := sources[0]
		if isDir(source) {
			if !strings.HasSuffix(dest, "/") {
				dest += "/"
			}
			if !strings.HasSuffix(source, "/") {
				source += "/"
			}
			options = append(options, dirMapping(source, dest))
			sources[0] = source
		} else if strings.HasSuffix(dest, "/") {
			options = append(options, ReplacePrefixPath(source, filepath.Join(dest, filepath.Base(source))))
		} else {
			options = append(options, ReplacePrefixPath(source, dest))
		}
	default:
		if !strings.HasSuffix(dest, "/") {
			dest += "/"
		}
		for i, src := range sources {
			if isDir(src) {
				if !strings.HasSuffix(src, "/") {
					src += "/"
				}
				options = append(options, dirMapping(src, dest))
				sources[i] = src
			} else {
				options = append(options, ReplacePrefixPath(src, filepath.Join(dest, filepath.Base(src))))
			}
		}
	}

	if err := tb.tar(blob, sources, append(options, extra...)...); err != nil {
		return "", fmt.Errorf("archiving sources %v: %w", sources, err)
	}

	return path, nil
}

func copyBlob(absCtx string, origin string, options ...TarOption) (string, error) {
	tb := NewTarball(absCtx)

	id := uuid.Generate()
	path := fmt.Sprintf("/tmp/%s.tar", id)
	blob, err := fs.Create(path)
	if err != nil {
		return "", fmt.Errorf("creating blob %s: %w", path, err)
	}
	defer blob.Close()

	if err := tb.Copy(path, origin, options...); err != nil {
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

// cleanContextSource normalizes a context-relative source path per docker:
// "Specifying a source path with a leading slash or one that navigates
// outside the build context, such as COPY ../something, automatically
// removes any parent directory navigation (../)."
func cleanContextSource(src string) string {
	cleaned := gopath.Clean("/" + filepath.ToSlash(src))
	return strings.TrimPrefix(cleaned, "/")
}
