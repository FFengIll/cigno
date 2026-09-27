package pkg

import (
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/moby/buildkit/frontend/dockerfile/instructions"
	"github.com/sirupsen/logrus"
)

func (engine *Engine) AllocLocalArgs(stage *instructions.Stage) {
	if engine.LocalArgs == nil {
		engine.LocalArgs = map[*instructions.Stage]map[string]string{}
	}
	engine.LocalArgs[stage] = map[string]string{}
}

func (engine *Engine) doArg(stage *instructions.Stage, argCmd *instructions.ArgCommand, img v1.Image) (v1.Image, error) {
	// TODO: add a layer for arg command into image
	for _, arg := range argCmd.Args {
		value := arg.ValueString()
		// an in-stage `ARG KEY` without a value inherits the global default,
		// matching docker behavior
		if arg.Value == nil {
			if g, ok := engine.GlobalArg[arg.Key]; ok {
				value = g
			}
		}
		logrus.WithField("key", arg.Key).WithField("value", value).Debug("ARG")
		engine.withArg(stage, arg.Key, value)
	}
	return img, nil
}

// WithGlobalArg sets a global (build-wide) arg value from the CLI.
func (engine *Engine) WithGlobalArg(key string, valueString string) {
	engine.withGlobalArg(key, valueString)
}

func (engine *Engine) withGlobalArg(key string, valueString string) {
	if engine.GlobalArg == nil {
		engine.GlobalArg = map[string]string{}
	}
	engine.GlobalArg[key] = valueString
}

func (engine *Engine) withArg(stage *instructions.Stage, key string, valueString string) {
	if engine.LocalArgs == nil {
		engine.LocalArgs = map[*instructions.Stage]map[string]string{}
	}
	var args = engine.LocalArgs[stage]
	if args == nil {
		args = map[string]string{}
		engine.LocalArgs[stage] = args
	}
	args[key] = valueString
}
