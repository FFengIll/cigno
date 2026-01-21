package pkg

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/moby/buildkit/frontend/dockerfile/instructions"
	specsv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	archive "github.com/vbatts/tar-split/archive/tar"
)

// doCopy
// COPY --from=image /path1 /path2 --chown=xx:xx --chmod=xxx
// COPY --from=image --base=image / /path2 --chown=xx:xx --chmod=xxx
// for local file, archive it into a tarball and modify own / mod / root path
func (engine *Engine) doCopy(cmd *instructions.CopyCommand, img v1.Image) (v1.Image, error) {
	var err error
	if cmd.From != "" {
		return engine.doCopyFrom(cmd, img)
	} else {
		// copy via local file system

		// FIXME: for now, we do not extract special files, but copy each blob above base image (just like rebase)
		tarPath := createBlob(engine.BuildDir, cmd.Dest(), cmd.Sources())
		logrus.Infof("cached blob to: %s", tarPath)

		// for image file, pull and extract them if possible (only process annotated layers / blobs if possible)

		// append
		var layer v1.Layer
		layer, err = tarball.LayerFromFile(tarPath, tarball.WithMediaType(engine.LayerType))
		// layer ,err = tarball.LayerFromOpener(w, tarball.WithMediaType(layerType))
		img, err = mutate.AppendLayers(img, layer)
	}
	return img, err
}

// copy --from=another_context
func (engine *Engine) doCopyFrom(cmd *instructions.CopyCommand, img v1.Image) (v1.Image, error) {
	var options []crane.Option

	// diff image with base
	// copy the top layers upon base, aka rebase
	// TODO: validate first
	orig := cmd.From
	ref := engine.getRef(orig)

	switch ref.Type {
	case ImageRef:
		orig = ref.Path

		//
		origImg, err := crane.Pull(orig, options...)
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

		//
		baseImg, err := crane.Pull(base, options...)
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
		blobPath, err := copyBlob("", tarballPath, options...)
		if err != nil {
			panic(err)
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

func subBaseImage(orig v1.Image, base v1.Image) ([]mutate.Addendum, error) {
	origLayers, err := orig.Layers()

	baseLayers, err := base.Layers()

	// TODO: validate
	for idx, rightLayer := range baseLayers {
		rightDigest, err := rightLayer.Digest()
		leftDigest, err := origLayers[idx].Digest()
		if leftDigest != rightDigest {
			return nil, err
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
		if i >= startLayer {
			adds = append(adds, mutate.Addendum{Layer: layers[layerIndex]})
		}
	}

	return adds
}

// doAdd handles ADD command
// ADD is like COPY but with additional features:
// 1. Supports URLs (http, https) - downloads and adds as file
// 2. Automatically extracts local tar files
func (engine *Engine) doAdd(cmd *instructions.AddCommand, img v1.Image) (v1.Image, error) {
	sources := cmd.Sources()
	dest := cmd.Dest()

	// If there's a URL source, we can only handle one source at a time
	var tarPath string
	var err error

	// Check if source is a URL
	if len(sources) == 1 && isURL(sources[0]) {
		tarPath, err = engine.downloadURL(sources[0], dest)
		if err != nil {
			return nil, fmt.Errorf("downloading URL %s: %w", sources[0], err)
		}
	} else {
		// Check if source is a tar file that should be extracted
		if len(sources) == 1 && isTarFile(sources[0]) {
			tarPath, err = engine.extractTar(sources[0], dest)
			if err != nil {
				return nil, fmt.Errorf("extracting tar file %s: %w", sources[0], err)
			}
		} else {
			// Same as COPY for local files
			tarPath = createBlob(engine.BuildDir, dest, sources)
			logrus.Infof("cached blob to: %s", tarPath)
		}
	}

	var layer v1.Layer
	layer, err = tarball.LayerFromFile(tarPath, tarball.WithMediaType(engine.LayerType))
	if err != nil {
		return nil, err
	}

	img, err = mutate.AppendLayers(img, layer)
	if err != nil {
		return nil, err
	}

	// Clean up temp file if it's a downloaded URL
	if len(sources) == 1 && isURL(sources[0]) {
		os.Remove(tarPath)
	}

	return img, nil
}

// isURL checks if the source is a URL
func isURL(source string) bool {
	u, err := url.Parse(source)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https")
}

// isTarFile checks if the source is a tar file
func isTarFile(source string) bool {
	return strings.HasSuffix(source, ".tar") ||
		strings.HasSuffix(source, ".tar.gz") ||
		strings.HasSuffix(source, ".tgz") ||
		strings.HasSuffix(source, ".tar.bz2") ||
		strings.HasSuffix(source, ".tar.xz")
}

// downloadURL downloads a file from URL and returns the path to the downloaded file
func (engine *Engine) downloadURL(urlStr, dest string) (string, error) {
	// Download to temp file
	tempFile, err := os.CreateTemp("", "cigno-download-*")
	if err != nil {
		return "", err
	}
	defer tempFile.Close()

	resp, err := http.Get(urlStr)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with status: %s", resp.Status)
	}

	_, err = io.Copy(tempFile, resp.Body)
	if err != nil {
		return "", err
	}

	// Now create a tarball with the downloaded file
	tb := NewTarball(engine.BuildDir)
	blobPath := fmt.Sprintf("/tmp/cigno-add-%d.tar", os.Getpid())
	blob, err := os.Create(blobPath)
	if err != nil {
		return "", err
	}
	defer blob.Close()

	// Get filename from URL
	filename := filepath.Base(urlStr)
	if dest != "" && !strings.HasSuffix(dest, "/") {
		// If dest is a file (not directory), use that as filename
		filename = filepath.Base(dest)
	}

	// Create tar with the downloaded file inside
	downloadedPath := tempFile.Name()
	defer os.Remove(downloadedPath)

	// Extract basename for tar header
	tarDest := dest
	if tarDest == "" || tarDest == "." {
		tarDest = "/"
	}

	options := []TarOption{
		ReplacePrefixPath("./", ""),
		ReplacePrefixPath(filename, strings.TrimPrefix(tarDest, "/")),
	}

	err = tb.tar(blob, []string{downloadedPath}, options...)
	if err != nil {
		return "", err
	}

	return blobPath, nil
}

// extractTar extracts a local tar file and returns the path to the extracted tarball
func (engine *Engine) extractTar(source, dest string) (string, error) {
	// Resolve source path relative to build dir
	srcPath := source
	if !filepath.IsAbs(source) {
		srcPath = filepath.Join(engine.BuildDir, source)
	}

	// Check if it's a directory (if so, we don't extract)
	info, err := os.Stat(srcPath)
	if err != nil {
		return "", err
	}

	if info.IsDir() {
		// Directory, just copy it like COPY does
		return createBlob(engine.BuildDir, dest, []string{source}), nil
	}

	// It's a file, check if it's a tar file by opening it
	f, err := os.Open(srcPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// Try to read as tar to verify
	tr := archive.NewReader(f)
	_, err = tr.Next()
	if err != nil {
		// Not a valid tar file, just copy it like COPY does
		return createBlob(engine.BuildDir, dest, []string{source}), nil
	}

	// Valid tar file - create a new tarball with extracted contents
	blobPath := fmt.Sprintf("/tmp/cigno-add-%d.tar", os.Getpid())

	tb := NewTarball(engine.BuildDir)
	destDir := dest
	if destDir == "" {
		destDir = "/"
	}

	// Use tarball Copy to extract with path transformations
	options := []TarOption{
		ReplacePrefixPath("./", strings.TrimPrefix(destDir, "/")),
	}

	err = tb.Copy(blobPath, srcPath, options...)
	if err != nil {
		return "", err
	}

	return blobPath, nil
}
