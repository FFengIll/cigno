package pkg

import (
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/moby/buildkit/frontend/dockerfile/instructions"
)

func (engine *Engine) AllocLocalArgs() {
	engine.LocalArgs = append(engine.LocalArgs, map[string]string{})
}

func (engine *Engine) doArg(argCmd *instructions.ArgCommand, img v1.Image) (v1.Image, error) {
	// TODO: add a layer for arg command into image
	for _, arg := range argCmd.Args {
		engine.withArg(arg.Key, arg.ValueString())
	}
	return img, nil
}

func (engine *Engine) withGlobalArg(key string, valueString string) {
	engine.GlobalArg[key] = valueString
}

func (engine *Engine) withArg(key string, valueString string) {
	var args = engine.LocalArgs[len(engine.LocalArgs)]
	args[key] = valueString
}
