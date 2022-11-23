package pkg

import (
	"fmt"
	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/moby/buildkit/frontend/dockerfile/instructions"
	specsv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
)

// doCopy
// COPY --from=image /path1 /path2 --chown=xx:xx --chmod=xxx
// COPY --from=image --base=image / /path2 --chown=xx:xx --chmod=xxx
// for local file, archive it into a tarball and modify own / mod / root path
func (engine *Engine) doCopy(cmd *instructions.CopyCommand, img v1.Image) (v1.Image, error) {
	var err error
	if cmd.From != "" {
		// copy --from=another_image
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
					return nil, err
				}
			}

			//
			baseImg, err := crane.Pull(base, options...)
			if err != nil {
				return nil, err
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
			options = append(options, ReplacePrefix("./", ""))
			options = append(options, ReplacePrefix(cmd.Sources()[0], cmd.Dest()))
			options = append(options, ReplacePrefix("/", ""))
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
