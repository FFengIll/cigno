package pkg

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/moby/buildkit/frontend/dockerfile/command"
	"github.com/moby/buildkit/frontend/dockerfile/instructions"
	"github.com/sirupsen/logrus"
)

type BuildOption func(img v1.Image) error

func prettyPrint(data any) {
	bs, _ := json.MarshalIndent(data, "", "  ")
	fmt.Println(string(bs))
}

func PrintHistoryOption() BuildOption {
	return func(img v1.Image) error {
		manifest, _ := img.Manifest()
		for _, layer := range manifest.Layers {
			prettyPrint(layer)
		}
		return nil
	}
}

func OutFileOption(outFile string, tag string) BuildOption {
	return func(img v1.Image) error {
		if outFile != "" {
			if err := crane.Save(img, tag, outFile); err != nil {
				return fmt.Errorf("writing output %q: %w", outFile, err)
			}
		}
		return nil
	}
}

func PushOption(tags []string) BuildOption {
	return func(img v1.Image) error {
		for _, tag := range tags {
			err := push(img, tag)
			if err != nil {
				return err
			}
		}
		return nil
	}
}

func (engine *Engine) Build(cmdReader io.Reader, options ...BuildOption) error {
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
	stages, globalArgCmds, err := ParseDockerFile(cmdReader)
	if err != nil {
		panic(err)
	}

	for _, cmd := range globalArgCmds {
		for _, arg := range cmd.Args {
			engine.withGlobalArg(arg.Key, arg.ValueString())
		}
	}

	for _, stage := range stages {
		// var base v1.Image
		// var err error
		engine.AllocLocalArgs(&stage)

		stageImage := stage.BaseName
		stageName := stage.Name

		engine.AddContext(stageName, stageImage)
		logrus.WithField("stage image", stageImage).WithField("stage name", stageName).Infof("STAGE")

		// TODO: validate base and tag here
		basePath, _ := engine.expandArg(&stage, stageImage)

		logrus.WithField("stage image", basePath).Info("STAGE")

		// doFrom
		var options []crane.Option
		base, err := crane.Pull(basePath, options...)
		if err != nil {
			return fmt.Errorf("pulling %s: %s", stageImage, err)
		}

		cfg, err := base.ConfigFile()
		if err != nil {
			return err
		}
		env := cfg.Config.Env
		engine.AllocLocalEnv(&stage, env)

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
				img, err = engine.doEnv(&stage, envCmd, img)
				if err != nil {
					return err
				}
				break
			case command.Arg:
				argCmd := ins.(*instructions.ArgCommand)
				img, err = engine.doArg(&stage, argCmd, img)
				break
			default:
				break
			}
		}
	}
	// verify the image if possible

	// push image
	for _, opt := range options {
		err := opt(img)
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
