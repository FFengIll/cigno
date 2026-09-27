package pkg

import (
	"archive/tar"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/docker/distribution/uuid"
	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/moby/buildkit/frontend/dockerfile/instructions"
	specsv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// copyHeaderOptions builds tar header options for COPY/ADD --chown/--chmod.
func (engine *Engine) copyHeaderOptions(chown, chmod string) ([]TarOption, error) {
	chown = engine.expandCurArg(chown)
	chmod = engine.expandCurArg(chmod)
	opts, err := parseCopyChown(chown)
	if err != nil {
		return nil, err
	}
	modOpt, err := parseCopyChmod(chmod)
	if err != nil {
		return nil, err
	}
	if modOpt != nil {
		opts = append(opts, modOpt)
	}
	return opts, nil
}

// doCopy
// COPY --from=image /path1 /path2 --chown=xx:xx --chmod=xxx
// COPY --from=image --base=image / /path2 --chown=xx:xx --chmod=xxx
// for local file, archive it into a tarball and modify own / mod / root path
func (engine *Engine) doCopy(cmd *instructions.CopyCommand, img v1.Image) (v1.Image, error) {
	if cmd.From != "" {
		return engine.doCopyFrom(cmd, img)
	}

	// expand args in sources/dest, e.g. `COPY app-${VERSION} /app`
	dest := engine.expandCurArg(cmd.Dest())
	sources := make([]string, len(cmd.Sources()))
	for i, src := range cmd.Sources() {
		sources[i] = engine.expandCurArg(src)
	}

	// --chown/--chmod apply to every archived entry (verbatim, like docker)
	extra, err := engine.copyHeaderOptions(cmd.Chown, cmd.Chmod)
	if err != nil {
		return nil, err
	}

	// expand wildcards in sources, e.g. `COPY dist/*.txt /data/`
	sources, err = expandWildcards(engine.BuildDir, sources)
	if err != nil {
		return nil, err
	}

	// copy via local file system

	// FIXME: for now, we do not extract special files, but copy each blob above base image (just like rebase)
	tarPath, err := createBlob(engine.BuildDir, dest, sources, extra...)
	if err != nil {
		return nil, err
	}
	logrus.Infof("cached blob to: %s", tarPath)

	// for image file, pull and extract them if possible (only process annotated layers / blobs if possible)

	// append
	layer, err := tarball.LayerFromFile(tarPath, tarball.WithMediaType(engine.LayerType))
	if err != nil {
		return nil, err
	}
	return mutate.AppendLayers(img, layer)
}

// copy --from=another_context
func (engine *Engine) doCopyFrom(cmd *instructions.CopyCommand, img v1.Image) (v1.Image, error) {
	// diff image with base
	// copy the top layers upon base, aka rebase
	// TODO: validate first
	orig := cmd.From
	ref := engine.getRef(orig)

	extra, err := engine.copyHeaderOptions(cmd.Chown, cmd.Chmod)
	if err != nil {
		return nil, err
	}

	// a built stage wins over any context mapping: docker semantics —
	// copy the given paths out of the stage's filesystem
	if stageImg, ok := engine.BuiltStages[orig]; ok {
		return engine.copyFromImageFS(cmd, stageImg, img, extra)
	}

	switch ref.Type {
	case ImageRef:
		orig = ref.Path

		// verbatim layer rebase cannot rewrite headers inside existing layers
		if len(extra) > 0 {
			logrus.Warn("--chown/--chmod are ignored with image-ref rebase (layers are copied verbatim)")
		}

		//
		origImg, err := crane.Pull(orig, craneOptions(orig)...)
		if err != nil {
			return nil, err
		}

		origMf, err := origImg.Manifest()
		if err != nil {
			return nil, err
		}

		//
		var base string
		var ok bool
		base, ok = origMf.Annotations[specsv1.AnnotationBaseImageName]
		if !ok {
			base, err = engine.getBase(orig)
			if err != nil {
				return nil, errors.Wrap(err, "no base image")
			}
		}

		// scratch has no layers, so all layers from origImg are the diff
		if base == "scratch" {
			origConfig, err := origImg.ConfigFile()
			if err != nil {
				return nil, err
			}
			origLayers, err := origImg.Layers()
			if err != nil {
				return nil, err
			}
			adds := createAddendums(0, 0, origConfig.History, origLayers)
			img, err = mutate.Append(img, adds...)
			if err != nil {
				return nil, err
			}
			break
		}

		//
		baseImg, err := crane.Pull(base, craneOptions(base)...)
		if err != nil {
			return nil, errors.Wrap(err, "failed to pull base image")
		}

		//
		adds, err := subBaseImage(origImg, baseImg)
		if err != nil {
			return nil, err
		}

		//
		img, err = mutate.Append(img, adds...)
		if err != nil {
			return nil, err
		}
		break
	case TarballRef:
		tarballPath := ref.Path

		var options []TarOption
		options = append(options, ReplacePrefixPath("./", ""))
		options = append(options, ReplacePrefixPath(cmd.Sources()[0], cmd.Dest()))
		options = append(options, ReplacePrefixPath("/", ""))
		options = append(options, extra...)
		blobPath, err := copyBlob("", tarballPath, options...)
		if err != nil {
			return nil, err
		}
		logrus.Infof("cached blob to: %s", blobPath)

		var layer v1.Layer
		layer, err = tarball.LayerFromFile(blobPath, tarball.WithMediaType(engine.LayerType))
		// layer ,err = tarball.LayerFromOpener(w, tarball.WithMediaType(layerType))
		if err != nil {
			return nil, err
		}
		img, err = mutate.AppendLayers(img, layer)
		if err != nil {
			return nil, err
		}
		break
	}
	return img, nil
}

// copyFromImageFS copies paths out of a built stage's filesystem
// (docker `COPY --from=<stage>` semantics): the stage image is flattened
// and the requested paths are archived into a new layer on dst.
func (engine *Engine) copyFromImageFS(cmd *instructions.CopyCommand, src v1.Image, dst v1.Image, extra []TarOption) (v1.Image, error) {
	// flatten the stage filesystem to a temp tar
	tmp := fmt.Sprintf("/tmp/cigno-stagefs-%s.tar", uuid.Generate())
	f, err := fs.Create(tmp)
	if err != nil {
		return nil, fmt.Errorf("creating stage fs blob: %w", err)
	}
	r := mutate.Extract(src)
	_, err = io.Copy(f, r)
	r.Close()
	f.Close()
	if err != nil {
		os.Remove(tmp)
		return nil, fmt.Errorf("flattening stage filesystem: %w", err)
	}
	defer os.Remove(tmp)

	// copy each source path with dest remap, like the tarball source path
	dest := engine.expandCurArg(cmd.Dest())
	for _, source := range cmd.Sources() {
		source = engine.expandCurArg(source)

		srcName := strings.TrimPrefix(filepath.ToSlash(source), "/")
		destName := strings.TrimPrefix(filepath.ToSlash(dest), "/")
		var options []TarOption
		// mutate.Extract emits absolute names ("/build/file"): normalize
		// to relative first, then remap source -> dest
		options = append(options, ReplacePrefixPath("/", ""))
		options = append(options, ReplacePrefixPath(srcName, destName))
		options = append(options, extra...)

		blobPath, err := copyBlob("", tmp, options...)
		if err != nil {
			return nil, fmt.Errorf("copying %q from stage: %w", source, err)
		}
		logrus.Infof("cached blob to: %s", blobPath)

		layer, err := tarball.LayerFromFile(blobPath, tarball.WithMediaType(engine.LayerType))
		if err != nil {
			return nil, err
		}
		dst, err = mutate.AppendLayers(dst, layer)
		if err != nil {
			return nil, err
		}
	}
	return dst, nil
}

func subBaseImage(orig v1.Image, base v1.Image) ([]mutate.Addendum, error) {
	origLayers, err := orig.Layers()
	if err != nil {
		return nil, fmt.Errorf("failed to get layers for original: %w", err)
	}

	baseLayers, err := base.Layers()
	if err != nil {
		return nil, fmt.Errorf("failed to get layers for base: %w", err)
	}

	// TODO: validate
	if len(origLayers) < len(baseLayers) {
		return nil, fmt.Errorf("original has fewer layers (%d) than base (%d), cannot rebase",
			len(origLayers), len(baseLayers))
	}
	for idx, rightLayer := range baseLayers {
		rightDigest, err := rightLayer.Digest()
		if err != nil {
			return nil, err
		}
		leftDigest, err := origLayers[idx].Digest()
		if err != nil {
			return nil, err
		}
		if leftDigest != rightDigest {
			return nil, fmt.Errorf("layer %d mismatch: original %s != base %s (is the image rebased on the given base?)",
				idx, leftDigest, rightDigest)
		}
	}

	origConfig, err := orig.ConfigFile()
	if err != nil {
		return nil, fmt.Errorf("failed to get config for original: %w", err)
	}

	baseConfig, err := base.ConfigFile()
	if err != nil {
		return nil, fmt.Errorf("could not get config for base: %w", err)
	}

	return createAddendums(
		len(baseConfig.History), len(baseLayers)+1, origConfig.History, origLayers), nil
}

// createAddendums makes a list of addendums from a history and layers starting from a specific history and layer
// indexes.
func createAddendums(startHistory, startLayer int, history []v1.History, layers []v1.Layer) []mutate.Addendum {
	var adds []mutate.Addendum
	// History should be a superset of layers; empty layers (e.g. ENV statements) only exist in history.
	// They cannot be iterated identically but must be walked independently, only advancing the iterator for layers
	// when a history entry for a non-empty layer is seen.
	layerIndex := 0
	for historyIndex := range history {
		var layer v1.Layer
		emptyLayer := history[historyIndex].EmptyLayer
		if !emptyLayer {
			layer = layers[layerIndex]
			layerIndex++
		}
		if historyIndex >= startHistory || layerIndex >= startLayer {
			adds = append(adds, mutate.Addendum{
				Layer:   layer,
				History: history[historyIndex],
			})
		}
	}
	// In the event history was malformed or non-existent, append the remaining layers.
	for i := layerIndex; i < len(layers); i++ {
		if i+1 >= startLayer {
			adds = append(adds, mutate.Addendum{Layer: layers[i]})
		}
	}

	return adds
}

// expandWildcards resolves glob patterns in COPY sources against the build
// context. Non-pattern sources pass through unchanged; patterns with no
// matches are an error (like docker). Matches are returned relative to the
// build dir so downstream path remapping keeps working.
func expandWildcards(buildDir string, sources []string) ([]string, error) {
	var out []string
	for _, src := range sources {
		if !containsWildcard(src) {
			out = append(out, src)
			continue
		}
		pattern := src
		if !filepath.IsAbs(pattern) {
			pattern = filepath.Join(buildDir, pattern)
		}
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("bad COPY pattern %q: %w", src, err)
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("COPY source %q matched no files", src)
		}
		sort.Strings(matches)
		for _, m := range matches {
			if rel, err := filepath.Rel(buildDir, m); err == nil && !strings.HasPrefix(rel, "..") {
				m = rel
			}
			out = append(out, m)
		}
	}
	return out, nil
}

// doAdd handles ADD command
// ADD is like COPY but with additional features:
// 1. Supports URLs (http, https) - downloads and adds as file
// 2. Automatically extracts local tar files
func (engine *Engine) doAdd(cmd *instructions.AddCommand, img v1.Image) (v1.Image, error) {
	// --chown/--chmod, same semantics as COPY
	extra, err := engine.copyHeaderOptions(cmd.Chown, cmd.Chmod)
	if err != nil {
		return nil, err
	}

	dest := engine.expandCurArg(cmd.Dest())

	blobs, err := engine.addSources(cmd.Sources(), dest, extra)
	if err != nil {
		return nil, err
	}

	var blobPath string
	if len(blobs) == 0 {
		return img, nil
	} else if len(blobs) == 1 {
		blobPath = blobs[0]
	} else {
		if blobPath, err = combineBlobs(blobs); err != nil {
			return nil, err
		}
	}

	layer, err := tarball.LayerFromFile(blobPath, tarball.WithMediaType(engine.LayerType))
	if err != nil {
		return nil, err
	}

	return mutate.AppendLayers(img, layer)
}

// addSources processes ADD sources into per-source tar blobs, applying the
// docker semantics: local tar archives (compression detected by content)
// are extracted; remote URLs are downloaded and placed verbatim, never
// decompressed. The caller merges the blobs into a single layer.
func (engine *Engine) addSources(sources []string, dest string, extra []TarOption) ([]string, error) {
	// docker: "If you specify multiple source files, either directly or
	// using a wildcard, then the destination must be a directory" — the
	// single-file rename semantics only applies with exactly one source
	if len(sources) > 1 && dest != "" && dest != "." && !strings.HasSuffix(dest, "/") {
		dest += "/"
	}

	var blobs []string
	for _, src := range sources {
		src = engine.expandCurArg(src)
		var blob string
		var err error
		if isURL(src) {
			blob, err = engine.addURL(src, dest, extra)
			if err != nil {
				return nil, fmt.Errorf("downloading URL %s: %w", src, err)
			}
		} else {
			blob, err = engine.extractTar(src, dest, extra)
			if err != nil {
				return nil, fmt.Errorf("adding %s: %w", src, err)
			}
			logrus.Infof("cached blob to: %s", blob)
		}
		blobs = append(blobs, blob)
	}
	return blobs, nil
}

// combineBlobs merges several tar blobs into a single tar file, removing the
// inputs. Duplicate directory entries are harmless (later entries win).
func combineBlobs(blobs []string) (string, error) {
	out := fmt.Sprintf("/tmp/cigno-combined-%s.tar", uuid.Generate())
	dst, err := os.Create(out)
	if err != nil {
		return "", err
	}
	defer dst.Close()
	tw := tar.NewWriter(dst)
	defer tw.Close()

	for _, blob := range blobs {
		if err := appendTarFile(tw, blob); err != nil {
			return "", err
		}
		os.Remove(blob)
	}
	return out, nil
}

func appendTarFile(tw *tar.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	tr := tar.NewReader(f)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if hdr.Size > 0 {
			if _, err := io.Copy(tw, tr); err != nil {
				return err
			}
		}
	}
}

// isURL checks if the source is a URL
func isURL(source string) bool {
	u, err := url.Parse(source)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https")
}

// addURL downloads a remote file and places it verbatim at dest — remote
// resources are NEVER decompressed, even for tar archives ("If remote file
// is a tar archive, the archive is not extracted by default", docker docs).
//
// Naming: a dest ending in "/" (or empty) takes the filename from the URL
// path; otherwise dest itself becomes the filename.
func (engine *Engine) addURL(urlStr, dest string, extra []TarOption) (string, error) {
	tempFile, err := os.CreateTemp("", "cigno-download-*")
	if err != nil {
		return "", err
	}
	downloadedPath := tempFile.Name()

	resp, err := http.Get(urlStr)
	if err != nil {
		os.Remove(downloadedPath)
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		os.Remove(downloadedPath)
		return "", fmt.Errorf("download failed with status: %s", resp.Status)
	}

	if _, err := io.Copy(tempFile, resp.Body); err != nil {
		tempFile.Close()
		os.Remove(downloadedPath)
		return "", err
	}
	tempFile.Close()

	filename := filepath.Base(urlStr)
	if filename == "" || filename == "." || filename == "/" {
		filename = "download"
	}

	// single-file blob: the downloaded file lives outside the build
	// context, so root the tarball at its parent directory. The tar source
	// is the actual temp filename; the URL basename is only the name the
	// entry gets remapped to.
	tb := NewTarball(filepath.Dir(downloadedPath))
	tempBase := filepath.Base(downloadedPath)

	destTrimmed := strings.Trim(dest, "/")
	var destName string
	if dest == "" || dest == "." || strings.HasSuffix(dest, "/") {
		destName = filepath.Join(destTrimmed, filename)
	} else {
		destName = destTrimmed
	}

	options := []TarOption{ReplacePrefixPath(tempBase, destName)}
	options = append(options, extra...)

	blobPath := fmt.Sprintf("/tmp/cigno-url-%s.tar", uuid.Generate())
	blob, err := fs.Create(blobPath)
	if err != nil {
		os.Remove(downloadedPath)
		return "", err
	}
	err = tb.tar(blob, []string{tempBase}, options...)
	blob.Close()
	os.Remove(downloadedPath)
	if err != nil {
		os.Remove(blobPath)
		return "", err
	}
	return blobPath, nil
}

// extractTar handles a local ADD source that may be a (possibly compressed)
// tar archive. Compression is detected by magic bytes (gzip/bzip2/xz); only
// tar-family archives are extracted — anything else is added as a plain
// file, matching docker ADD semantics.
func (engine *Engine) extractTar(source, dest string, extra []TarOption) (string, error) {
	srcPath := source
	if !filepath.IsAbs(source) {
		srcPath = filepath.Join(engine.BuildDir, source)
	}
	return engine.extractTarFile(srcPath, source, dest, extra)
}

// extractTarFile is the path-based core shared by local ADD and downloaded
// URLs. srcDisplay is the context-relative name used when falling back to a
// plain file blob.
func (engine *Engine) extractTarFile(srcPath, srcDisplay, dest string, extra []TarOption) (string, error) {
	info, err := os.Stat(srcPath)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return createBlob(engine.BuildDir, dest, []string{srcDisplay}, extra...)
	}

	arc, isTar, err := openArchiveTar(srcPath)
	if err != nil {
		return "", err
	}
	if !isTar {
		// not a tar archive: ADD semantics keep it as a plain file
		return createBlob(engine.BuildDir, dest, []string{srcDisplay}, extra...)
	}
	defer arc.Close()

	// rewrite the archive into a new blob, placing entries under dest
	blobPath := fmt.Sprintf("/tmp/cigno-extract-%s.tar", uuid.Generate())
	tb := NewTarball(engine.BuildDir)
	tb.Skip = skipWhiteouts

	options := []TarOption{
		ReplacePrefixPath("./", ""),
		ReplacePrefixPath("/", ""),
		PathPrefixOption(strings.Trim(dest, "/")),
	}
	options = append(options, extra...)

	if err := tb.CopyFromReader(blobPath, srcPath, arc, options...); err != nil {
		return "", err
	}
	logrus.Infof("cached extracted blob to: %s", blobPath)
	return blobPath, nil
}

// skipWhiteouts drops layer whiteout markers: they carry layer semantics
// that do not apply to ADD extraction.
func skipWhiteouts(name string) bool {
	base := name
	if idx := strings.LastIndexByte(name, '/'); idx >= 0 {
		base = name[idx+1:]
	}
	return strings.HasPrefix(base, ".wh.")
}
