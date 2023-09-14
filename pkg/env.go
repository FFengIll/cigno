package pkg

import (
	"fmt"
	"os"
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/moby/buildkit/frontend/dockerfile/instructions"
	"github.com/sirupsen/logrus"
)

// doEnv will update env with expr support
// e.g.
// ENV OPT=/opt
// ENV PATH=/opt:$PATH
func (engine *Engine) doEnv(stage *instructions.Stage, cmd *instructions.EnvCommand, img v1.Image) (v1.Image, error) {
	var env = engine.Env[stage]

	for _, kv := range cmd.Env {
		key := kv.Key
		expr := kv.Value

		// expand from arg at first, then from env
		value, _ := engine.expandArg(stage, expr)
		value, _ = expandEnv(env, value)

		env[key] = value
		logrus.WithField("key", key).WithField("value", value).Debug("ENV")
	}

	// FIXME: do env mutate into image config later
	return img, nil
}

func expandEnv(env map[string]string, expr string) (string, bool) {
	var ok bool
	res := os.Expand(expr, func(s string) string {
		var v string
		if v, ok = env[s]; ok {
			return v
		}
		return fmt.Sprintf("$%s", s)
	})
	return res, ok
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
