package pkg

import (
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/moby/buildkit/frontend/dockerfile/instructions"
	"os"
	"strings"
)

// doEnv
// ENV OPT=/opt
// ENV PATH=/opt:$PATH
func (engine *Engine) doEnv(cmd *instructions.EnvCommand, img v1.Image) (v1.Image, error) {
	cfg, err := img.ConfigFile()
	if err != nil {
		return nil, err
	}
	env := cfg.Config.Env

	parsedEnv := parseEnv(env)

	for _, kv := range cmd.Env {
		key := kv.Key
		value := kv.Value
		if _, ok := parsedEnv[key]; ok {
			value = os.Expand(value, func(s string) string {
				if v, ok := parsedEnv[s]; ok {
					return v
				} else {
					return ""
				}
			})

		}

		pair := instructions.KeyValuePair{Key: key, Value: value}
		cfg.Config.Env = append(cfg.Config.Env, pair.String())
	}

	return img, nil
}

func parseEnv(env []string) map[string]string {
	parsed := map[string]string{}
	for idx := range env {
		items := strings.SplitN(env[idx], "=", 1)
		parsed[items[0]] = items[1]
	}

	return parsed
}
