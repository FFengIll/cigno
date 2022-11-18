package pkg

import (
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/moby/buildkit/frontend/dockerfile/instructions"
	"github.com/sirupsen/logrus"
)

type Engine struct {
	BuildCtx  string
	LayerType types.MediaType
}

// doCopy
// COPY --from=image /path1 /path2 --chown=xx:xx --chmod=xxx
// COPY --from=image --base=image / /path2 --chown=xx:xx --chmod=xxx
// for local file, archive it into a tarball and modify own / mod / root path
func (engine *Engine) doCopy(cmd *instructions.CopyCommand, img v1.Image) error {
	var err error
	if cmd.From != "" {
		// copy --from=another_image
		panic("not implemented `COPY --from=xxx`")

	} else {
		// copy via local file system

		// FIXME: for now, we do not extract special files, but copy each blob above base image (just like rebase)
		tarPath := buildBlob(engine.BuildCtx, cmd.Dest(), cmd.Sources())
		logrus.Infof("cached blob to: %s", tarPath)

		// for image file, pull and extract them if possible (only process annotated layers / blobs if possible)

		// append
		var layer v1.Layer
		layer, err = tarball.LayerFromFile(tarPath, tarball.WithMediaType(engine.LayerType))
		// layer ,err = tarball.LayerFromOpener(w, tarball.WithMediaType(layerType))
		img, err = mutate.AppendLayers(img, layer)
	}
	return err
}
