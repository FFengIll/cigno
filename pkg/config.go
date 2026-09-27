package pkg

import (
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/moby/buildkit/frontend/dockerfile/instructions"
	"github.com/sirupsen/logrus"
)

// doWorkdir handles WORKDIR command
func (engine *Engine) doWorkdir(stage *instructions.Stage, cmd *instructions.WorkdirCommand, cfg *v1.ConfigFile) error {
	// docker resolves WORKDIR against ENV vars, then against the current
	// working directory when relative (which starts from the base image's
	// WORKDIR and defaults to "/")
	path := engine.expandCurArg(cmd.Path)
	path = engine.expandEnvIn(path, engine.Env[stage])

	if !strings.HasPrefix(path, "/") {
		// chain onto the previous WORKDIR ("/" when none), avoiding "//"
		path = strings.TrimSuffix(cfg.Config.WorkingDir, "/") + "/" + path
	}
	cfg.Config.WorkingDir = path
	logrus.WithField("workdir", path).Debug("WORKDIR")
	return nil
}

// doUser handles USER command
func (engine *Engine) doUser(cmd *instructions.UserCommand, cfg *v1.ConfigFile) error {
	user := engine.expandCurArg(cmd.User)
	cfg.Config.User = user
	logrus.WithField("user", user).Debug("USER")
	return nil
}

// doCmd handles CMD command
func (engine *Engine) doCmd(cmd *instructions.CmdCommand, cfg *v1.ConfigFile) error {
	cfg.Config.Cmd = engine.cmdLine(cmd.ShellDependantCmdLine)
	logrus.WithField("cmd", cfg.Config.Cmd).Debug("CMD")
	return nil
}

// doEntrypoint handles ENTRYPOINT command
func (engine *Engine) doEntrypoint(cmd *instructions.EntrypointCommand, cfg *v1.ConfigFile) error {
	cfg.Config.Entrypoint = engine.cmdLine(cmd.ShellDependantCmdLine)
	logrus.WithField("entrypoint", cfg.Config.Entrypoint).Debug("ENTRYPOINT")
	return nil
}

// cmdLine resolves a parsed CMD/ENTRYPOINT line into image-config form:
// exec form is used as-is; shell form (PrependShell) is wrapped in the
// default shell, matching docker: CMD echo hi -> ["/bin/sh","-c","echo hi"]
func (engine *Engine) cmdLine(line instructions.ShellDependantCmdLine) []string {
	if line.PrependShell {
		return []string{"/bin/sh", "-c", strings.Join(line.CmdLine, " ")}
	}
	return []string(line.CmdLine)
}

// doLabel handles LABEL command
func (engine *Engine) doLabel(cmd *instructions.LabelCommand, cfg *v1.ConfigFile) error {
	if cfg.Config.Labels == nil {
		cfg.Config.Labels = make(map[string]string)
	}
	for _, kv := range cmd.Labels {
		// expand args/env in label value, e.g. `LABEL version=${VERSION}`
		value := engine.expandCurArg(kv.Value)
		cfg.Config.Labels[kv.Key] = value
		logrus.WithField("key", kv.Key).WithField("value", value).Debug("LABEL")
	}
	return nil
}

// doExpose handles EXPOSE command
func (engine *Engine) doExpose(cmd *instructions.ExposeCommand, cfg *v1.ConfigFile) error {
	if cfg.Config.ExposedPorts == nil {
		cfg.Config.ExposedPorts = make(map[string]struct{})
	}
	for _, port := range cmd.Ports {
		port = engine.expandCurArg(port)
		cfg.Config.ExposedPorts[port] = struct{}{}
		logrus.WithField("port", port).Debug("EXPOSE")
	}
	return nil
}

// doVolume handles VOLUME command
func (engine *Engine) doVolume(cmd *instructions.VolumeCommand, cfg *v1.ConfigFile) error {
	if cfg.Config.Volumes == nil {
		cfg.Config.Volumes = make(map[string]struct{})
	}
	for _, vol := range cmd.Volumes {
		vol = engine.expandCurArg(vol)
		cfg.Config.Volumes[vol] = struct{}{}
		logrus.WithField("volume", vol).Debug("VOLUME")
	}
	return nil
}
