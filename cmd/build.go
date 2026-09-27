package cmd

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"cigno/pkg"

	"github.com/sirupsen/logrus"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

const (
	keepEnv    = "keep"
	refreshEnv = "refresh"
)

var envStrategy = []string{
	keepEnv,
	refreshEnv,
}

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build an image from a Dockerfile without a daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		tags, _ := cmd.Flags().GetStringArray("tag")
		dockerfile, _ := cmd.Flags().GetString("file")
		buildArgs, _ := cmd.Flags().GetStringArray("build-arg")
		doPush, _ := cmd.Flags().GetBool("push")
		outFile, _ := cmd.Flags().GetString("output-file")
		noCache, _ := cmd.Flags().GetBool("no-cache")
		logrus.WithField("dockerfile", dockerfile).
			WithField("tag", tags).
			WithField("args", buildArgs).
			Info("Basic Info:")

		es, _ := cmd.Flags().GetString("env-strategy")
		var found = false
		for _, item := range envStrategy {
			if es == item {
				found = true
			}
		}
		if !found {
			cmd.Usage()
			return fmt.Errorf("invalid env-strategy %q", es)
		}

		// validate only: parse the dockerfile and exit
		if validateOnly, _ := cmd.Flags().GetBool("validate"); validateOnly {
			return pkg.ValidateDockerfile(dockerfile)
		}

		// load dockerfile
		fs := afero.NewOsFs()
		var df afero.File
		df, err := fs.Open(dockerfile)
		if err != nil {
			return fmt.Errorf("opening dockerfile %s: %w", dockerfile, err)
		}
		bs, _ := io.ReadAll(df)

		logrus.Infof("Dockerfile Content: \n%s", string(bs))

		// build with dockerfile
		engine := pkg.NewEngine()

		// context (build dir)
		ctxDir, _ := cmd.Flags().GetString("context")
		if ctxDir != "" {
			engine.BuildDir = ctxDir
		}

		// build args override the ARG defaults declared in the dockerfile
		for _, item := range buildArgs {
			ss := strings.SplitN(item, "=", 2)
			if len(ss) != 2 {
				return fmt.Errorf("invalid build-arg %q, expected `key=value`", item)
			}
			engine.WithGlobalArg(ss[0], ss[1])
		}

		// base image mapping / tarball / folder contexts
		for _, item := range imageBaseArray {
			ss := strings.SplitN(item, "=", 2)
			if len(ss) != 2 {
				return fmt.Errorf("invalid image-base %q, expected `image=base`", item)
			}
			engine.AddBase(ss[0], ss[1])
		}
		for _, item := range tarballArray {
			ss := strings.SplitN(item, "=", 2)
			if len(ss) != 2 {
				return fmt.Errorf("invalid tarball %q, expected `name=path`", item)
			}
			engine.AddTarball(ss[0], ss[1])
		}
		for _, item := range folderArray {
			ss := strings.SplitN(item, "=", 2)
			if len(ss) != 2 {
				return fmt.Errorf("invalid folder %q, expected `name=path`", item)
			}
			engine.AddFolder(ss[0], ss[1])
		}

		// local disk cache for base images
		if !noCache {
			if err := engine.InitCache(""); err != nil {
				logrus.WithError(err).Warn("failed to init cache, continuing without")
			}
		}

		var options []pkg.BuildOption
		options = append(options, pkg.PrintHistoryOption())
		if outFile != "" {
			if len(tags) == 0 {
				tags = []string{"cigno:latest"}
			}
			options = append(options, pkg.OutFileOption(outFile, tags[0]))
		} else if doPush {
			if len(tags) == 0 {
				return fmt.Errorf("push requires at least one -t tag")
			}
			options = append(options, pkg.PushOption(tags))
		} else {
			logrus.Warn("no -o output or --push given, running as dry-run (no output)")
		}

		return engine.Build(
			bytes.NewReader(bs),
			options...,
		)
	},
}

func init() {
	flags := buildCmd.Flags()
	flags.StringP("file", "f", "", "dockerfile")
	buildCmd.MarkFlagRequired("file")
	flags.StringVarP(&buildDir, "context", "c", buildDir, "build context dir")
	flags.StringArrayP("tag", "t", []string{}, "tag used in the `name:tag` format")
	flags.BoolVar(&dryRun, "dry-run", false, "")
	flags.BoolVarP(&validate, "validate", "v", false, "")
	flags.StringArrayVar(&imageBaseArray, "image-base", []string{}, "map an image name to a base ref (`image=base`)")
	flags.StringArrayVar(&tarballArray, "tarball", []string{}, "add a tarball source (`name=path`)")
	flags.StringArrayVar(&folderArray, "folder", []string{}, "add a folder source (`name=path`)")
	flags.StringVarP(&outFile, "output-file", "o", "", "save the image to a docker-load-able tar file instead of pushing")
	flags.StringArray("build-arg", []string{}, "set a build arg (`key=value`)")
	flags.Bool("push", false, "push the image to the registry")
	flags.Bool("rebase-as-copy", false, "use rebase to replace `COPY --from`")
	flags.Bool("no-cache", false, "disable the local base image cache")
	flags.String("env-strategy", "keep", fmt.Sprintf("strategy to process env. (%s)", envStrategy))
}
