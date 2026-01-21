package pkg

import (
	"fmt"
	"io"
	"strings"

	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/moby/buildkit/frontend/dockerfile/command"
	"github.com/moby/buildkit/frontend/dockerfile/instructions"
	specsv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
)

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
		engine.AllocLocalEnv(&stage, cfg.Config.Env)

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
			case command.Add:
				addCmd := ins.(*instructions.AddCommand)
				img, err = engine.doAdd(addCmd, img)
				if err != nil {
					return err
				}
				break
			case command.Run:
				// do not run in a daemon, overlay fs or any other isolation
				// we do only support some `scope in control` cmd and files, e.g. wget, tar, tee
				// furthermore, use a temporary path to hold root fs structure if possible
				// then we archive the results into a tar file as blob to append
				logrus.Warn("RUN command is not yet implemented")
			case command.Env:
				// here is an easy way to append ENV,
				// and to support `+=`, we should be careful to merge original value and plus value.
				// furthermore, we should record any ENV we meet to do the eval
				envCmd := ins.(*instructions.EnvCommand)
				if err := engine.doEnv(&stage, envCmd); err != nil {
					return err
				}
				img, err = addHistory(img, envCmd.String())
				if err != nil {
					return err
				}
				break
			case command.Arg:
				argCmd := ins.(*instructions.ArgCommand)
				if _, err := engine.doArg(&stage, argCmd, img); err != nil {
					return err
				}
				img, err = addHistory(img, argCmd.String())
				if err != nil {
					return err
				}
				break
			case command.Workdir:
				workdirCmd := ins.(*instructions.WorkdirCommand)
				if err := engine.doWorkdir(&stage, workdirCmd, cfg); err != nil {
					return err
				}
				img, err = addHistory(img, workdirCmd.String())
				if err != nil {
					return err
				}
				break
			case command.User:
				userCmd := ins.(*instructions.UserCommand)
				if err := engine.doUser(userCmd, cfg); err != nil {
					return err
				}
				img, err = addHistory(img, userCmd.String())
				if err != nil {
					return err
				}
				break
			case command.Cmd:
				cmdCmd := ins.(*instructions.CmdCommand)
				if err := engine.doCmd(cmdCmd, cfg); err != nil {
					return err
				}
				img, err = addHistory(img, cmdCmd.String())
				if err != nil {
					return err
				}
				break
			case command.Entrypoint:
				entrypointCmd := ins.(*instructions.EntrypointCommand)
				if err := engine.doEntrypoint(entrypointCmd, cfg); err != nil {
					return err
				}
				img, err = addHistory(img, entrypointCmd.String())
				if err != nil {
					return err
				}
				break
			case command.Label:
				labelCmd := ins.(*instructions.LabelCommand)
				if err := engine.doLabel(labelCmd, cfg); err != nil {
					return err
				}
				img, err = addHistory(img, labelCmd.String())
				if err != nil {
					return err
				}
				break
			case command.Expose:
				exposeCmd := ins.(*instructions.ExposeCommand)
				if err := engine.doExpose(exposeCmd, cfg); err != nil {
					return err
				}
				img, err = addHistory(img, exposeCmd.String())
				if err != nil {
					return err
				}
				break
			case command.Volume:
				volumeCmd := ins.(*instructions.VolumeCommand)
				if err := engine.doVolume(volumeCmd, cfg); err != nil {
					return err
				}
				img, err = addHistory(img, volumeCmd.String())
				if err != nil {
					return err
				}
				break
			default:
				break
			}
		}

		cfg = cfg.DeepCopy()

		// Set labels.
		if cfg.Config.Labels == nil {
			cfg.Config.Labels = map[string]string{}
		}

		// FIXME: we can not add history for current API
		// for _, ins := range stage.Commands {
		// 	add := mutate.Addendum{
		// 		History: v1.History{
		// 			CreatedBy: ins.Name(),
		// 		},
		// 	}
		// 	img, err = mutate.Append(img, add)
		// 	if err != nil {
		// 		return err
		// 	}
		// }

		// Update annotations.
		annotations := map[string]string{}
		baseDigest, _ := img.Digest()
		baseName := basePath
		annotations[specsv1.AnnotationBaseImageName] = baseName
		annotations[specsv1.AnnotationBaseImageDigest] = baseDigest.String()
		img = mutate.Annotations(img, annotations).(v1.Image)

		// Update env vars.
		env := engine.Env[&stage]
		if err := setEnvVars(cfg, env); err != nil {
			return err
		}
		img, err = mutate.Config(img, cfg.Config)
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

// setEnvVars override envvars in a config
func setEnvVars(cfg *v1.ConfigFile, envVars map[string]string) error {
	newEnv := make([]string, 0, len(cfg.Config.Env))
	for _, old := range cfg.Config.Env {
		split := strings.SplitN(old, "=", 2)
		if len(split) != 2 {
			return fmt.Errorf("invalid key value pair in config: %s", old)
		}
		// keep order so override if specified again
		oldKey := split[0]
		if v, ok := envVars[oldKey]; ok {
			newEnv = append(newEnv, fmt.Sprintf("%s=%s", oldKey, v))
			delete(envVars, oldKey)
		} else {
			newEnv = append(newEnv, old)
		}
	}
	isWindows := cfg.OS == "windows"
	for k, v := range envVars {
		if isWindows {
			k = strings.ToUpper(k)
		}
		newEnv = append(newEnv, fmt.Sprintf("%s=%s", k, v))
	}
	cfg.Config.Env = newEnv
	return nil
}

// addHistory adds a history entry for config-only commands (empty layer)
func addHistory(img v1.Image, createdBy string) (v1.Image, error) {
	add := mutate.Addendum{
		History: v1.History{
			CreatedBy:  createdBy,
			EmptyLayer: true,
		},
	}
	return mutate.Append(img, add)
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
