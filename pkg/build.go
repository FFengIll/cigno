package pkg

import (
	"fmt"
	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/moby/buildkit/frontend/dockerfile/command"
	"github.com/moby/buildkit/frontend/dockerfile/instructions"
	"github.com/sirupsen/logrus"
	"io"
)

type BuildOption func(img v1.Image, tag string) error

func OutFileOption(outFile string) BuildOption {
	return func(img v1.Image, tag string) error {
		if outFile != "" {
			if err := crane.Save(img, tag, outFile); err != nil {
				return fmt.Errorf("writing output %q: %w", outFile, err)
			}
		}
		return nil
	}
}

func PushOption() BuildOption {
	return func(img v1.Image, tag string) error {
		err := push(img, tag)
		if err != nil {
			return err
		}
		return nil
	}
}

func (engine *Engine) Build(cmdReader io.Reader, tag string, options ...BuildOption) error {
	/* load crane shell
	- rebase --from= --base=
	- copy
	- annotation
	*/
	/* load crane with dockerfile (only support ENV / COPY / RUN / ARG)
	 */

	// start from an empty image
	var img v1.Image
	img = empty.Image
	logrus.Debug(img)

	var env []string
	var envHistory []string
	logrus.Debug(env)
	logrus.Debug(envHistory)

	// var dockerfile string
	stages, _, err := ParseDockerFile(cmdReader)
	if err != nil {
		panic(err)
	}

	for _, stage := range stages {
		// var base v1.Image
		// var err error

		baseRef := stage.BaseName
		stageRef := stage.Name

		engine.AddImage(stageRef, baseRef)
		logrus.Infof("base: %s, stage: %s", baseRef, stageRef)

		// TODO: validate base and tag here

		// doFrom
		var options []crane.Option
		base, err := crane.Pull(baseRef, options...)
		if err != nil {
			return fmt.Errorf("pulling %s: %s", baseRef, err)
		}

		// check media type
		baseMediaType, err := base.MediaType()
		if err != nil {
			logrus.Fatalf("getting base image media type: %s", err)
		}
		layerType := types.DockerLayer
		if baseMediaType == types.OCIManifestSchema1 {
			layerType = types.OCILayer
		}

		// init the layer type
		if engine.LayerType == "" {
			engine.LayerType = layerType
		}

		img = base
		for _, ins := range stage.Commands {
			// process instruction
			name := ins.Name()
			switch name {
			case "REBASE":
				// must support annotation in original image

				// now we have a new image with new base
				// aka. `app / new base`
				// furthermore, we can build use rebase for image like `comp1 / comp2 / comp3 / new base`

			case command.Copy:
				copyCmd := ins.(*instructions.CopyCommand)
				img, err = engine.doCopy(copyCmd, img)
				if err != nil {
					return err
				}
				break
			case command.Run:
				// do not run in a daemon, overlay fs or any other isolation
				// we do only support some `scope in control` cmd and files, e.g. wget, tar, tee
				// furthermore, use a temporary path to hold root fs structure if possible
				// then we archive the results into a tar file as blob to append
			case command.Env:
				// here is an easy way to append ENV,
				// and to support `+=`, we should be careful to merge original value and plus value.
				// furthermore, we should record any ENV we meet to do the eval
				// FIXME: no we use a `bash -c` command to help eval env
				envCmd := ins.(*instructions.EnvCommand)
				img, err = engine.doEnv(envCmd, img)
				if err != nil {
					return err
				}
				break
			case command.Arg:
				break
			default:
				break
			}
		}
	}
	// verify the image if possible

	// push image
	for _, opt := range options {
		err := opt(img, tag)
		if err != nil {
			return err
		}
	}

	return nil
}

// validation will validate build arguments to confirm it works well
func validation() {

}

// FIXME: merge the image config field to confirm the output work, e.g. env, user, entrypoint
// maybe no use
func mergeConfig() {

}

func mergeEnv() {

}
