package pkg

import (
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/moby/buildkit/frontend/dockerfile/instructions"
	"github.com/sirupsen/logrus"
)

// doWorkdir handles WORKDIR command
func (engine *Engine) doWorkdir(stage *instructions.Stage, cmd *instructions.WorkdirCommand, cfg *v1.ConfigFile) error {
	path := cmd.Path
	// Expand ARG variables in path
	expandedPath, _ := engine.expandArg(stage, path)
	cfg.Config.WorkingDir = expandedPath
	logrus.WithField("workdir", expandedPath).Debug("WORKDIR")
	return nil
}

// doUser handles USER command
func (engine *Engine) doUser(cmd *instructions.UserCommand, cfg *v1.ConfigFile) error {
	user := cmd.User
	cfg.Config.User = user
	logrus.WithField("user", user).Debug("USER")
	return nil
}

// doCmd handles CMD command
func (engine *Engine) doCmd(cmd *instructions.CmdCommand, cfg *v1.ConfigFile) error {
	// CmdLine is strslice.StrSlice which converts to []string
	cfg.Config.Cmd = []string(cmd.CmdLine)
	logrus.WithField("cmd", cfg.Config.Cmd).Debug("CMD")
	return nil
}

// doEntrypoint handles ENTRYPOINT command
func (engine *Engine) doEntrypoint(cmd *instructions.EntrypointCommand, cfg *v1.ConfigFile) error {
	// CmdLine is strslice.StrSlice which converts to []string
	cfg.Config.Entrypoint = []string(cmd.CmdLine)
	logrus.WithField("entrypoint", cfg.Config.Entrypoint).Debug("ENTRYPOINT")
	return nil
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
		cfg.Config.Volumes[vol] = struct{}{}
		logrus.WithField("volume", vol).Debug("VOLUME")
	}
	return nil
}
