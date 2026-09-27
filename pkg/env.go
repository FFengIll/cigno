package pkg

import (
	"strings"

	"github.com/moby/buildkit/frontend/dockerfile/instructions"
	"github.com/sirupsen/logrus"
)

// doEnv will update env with expr support
// e.g.
// ENV OPT=/opt
// ENV PATH=/opt:$PATH
// The env is stored in engine.Env[stage] and applied later via setEnvVars
func (engine *Engine) doEnv(stage *instructions.Stage, cmd *instructions.EnvCommand) error {
	if engine.Env == nil {
		engine.Env = map[*instructions.Stage]map[string]string{}
	}
	var env = engine.Env[stage]
	if env == nil {
		env = map[string]string{}
		engine.Env[stage] = env
	}

	for _, kv := range cmd.Env {
		key := kv.Key
		expr := kv.Value

		// expand against the in-progress stage environment first (so
		// previously declared ENV wins over same-named ARG), then args;
		// undefined names expand to "" per docker semantics
		value := engine.expandEnvIn(expr, env)

		env[key] = value
		logrus.WithField("key", key).WithField("value", value).Debug("ENV")
	}

	return nil
}

func parseEnv(env []string) map[string]string {
	parsed := map[string]string{}
	for _, line := range env {
		items := strings.SplitN(line, "=", 2)
		if len(items) < 2 {
			logrus.WithField("expr", line).Error("ENV expr invalid")
			continue
		}
		parsed[items[0]] = items[1]
	}

	return parsed
}
