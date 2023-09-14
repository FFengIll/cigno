package cmd

import (
	"bytes"
	"fmt"
	"io"

	"devpod/cigno/pkg"

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
	Use: "build",
	// Args: cobra.ExactArgs(1),
	PreRun: doVerbose,
	Run: func(cmd *cobra.Command, args []string) {

		tags, _ := cmd.Flags().GetStringArray("tag")
		dockerfile, _ := cmd.Flags().GetString("file")
		buildArgs, _ := cmd.Flags().GetStringArray("build-arg")
		doPush, _ := cmd.Flags().GetBool("push")
		logrus.Info(tags)
		logrus.Info(dockerfile)
		logrus.Info(buildArgs)

		es, _ := cmd.Flags().GetString("env-strategy")
		var found = false
		for _, item := range envStrategy {
			if es == item {
				found = true
			}
		}
		if !found {
			cmd.Usage()
			return
		}

		// load dockerfile
		fs := afero.NewOsFs()
		var df afero.File
		df, err := fs.Open(dockerfile)
		if err != nil {
			panic(err)
		}
		bs, _ := io.ReadAll(df)

		// build with dockerfile
		engine := pkg.NewEngine()

		var options []pkg.BuildOption
		options = append(options, pkg.PrintHistoryOption())
		if doPush {
			options = append(options, pkg.PushOption(tags))
		}
		err = engine.Build(
			bytes.NewReader(bs),
			options...,
		)
		if err != nil {
			panic(err)
		}
	},
}

func init() {
	flags := buildCmd.Flags()
	flags.StringP("file", "f", "Dockerfile", "dockerfile")
	flags.StringVarP(&buildDir, "context", "c", buildDir, "")
	flags.StringArrayP("tag", "t", []string{}, "tag used in the `name:tag` format")
	flags.BoolVar(&dryRun, "dry-run", false, "")
	flags.BoolVarP(&validate, "validate", "v", false, "")
	flags.StringArrayVar(&imageBaseArray, "image-base", []string{}, "")
	flags.StringArrayVar(&tarballArray, "tarball", []string{}, "")
	flags.StringArrayVar(&folderArray, "folder", []string{}, "")
	flags.StringVarP(&outFile, "output-file", "o", "", "")
	flags.StringArray("build-arg", []string{}, "build arg")
	flags.Bool("push", false, "do push")
	flags.Bool("rebase-as-copy", false, "use rebase to replace `COPY --from`")
	flags.String("env-strategy", "keep", fmt.Sprintf("strategy to process env. (%s)", envStrategy))
}
