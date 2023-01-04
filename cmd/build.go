package cmd

import (
	"bytes"
	"github.com/sirupsen/logrus"
	"io"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"

	"devpod/cigno/pkg"
)

var buildCmd = &cobra.Command{
	Use: "build",
	Run: func(cmd *cobra.Command, args []string) {
		tag, _ := cmd.Flags().GetString("tag")
		dockerfile, _ := cmd.Flags().GetString("dockerfile")
		logrus.Info(tag)
		logrus.Info(dockerfile)

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
		err = engine.Build(
			bytes.NewReader(bs),
			options...,
		)
		if err != nil {
			panic(err)
		}
	},
}
