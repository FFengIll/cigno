package pkg

import (
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/moby/buildkit/frontend/dockerfile/instructions"
	"github.com/sirupsen/logrus"
)

func (engine *Engine) AllocLocalArgs(stage *instructions.Stage) {
	engine.LocalArgs[stage] = map[string]string{}
}

func (engine *Engine) doArg(stage *instructions.Stage, argCmd *instructions.ArgCommand, img v1.Image) (v1.Image, error) {
	// TODO: add a layer for arg command into image
	for _, arg := range argCmd.Args {
		logrus.WithField("key", arg.Key).WithField("value", arg.ValueString()).Debug("ARG")
		engine.withArg(stage, arg.Key, arg.ValueString())
	}
	return img, nil
}

func (engine *Engine) withGlobalArg(key string, valueString string) {
	engine.GlobalArg[key] = valueString
}

func (engine *Engine) withArg(stage *instructions.Stage, key string, valueString string) {
	var args = engine.LocalArgs[stage]
	args[key] = valueString
}
