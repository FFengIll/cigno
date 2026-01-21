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
