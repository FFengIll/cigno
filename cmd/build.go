package main

import (
	"bytes"
	"devpod/cranit/pkg"
	"fmt"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"io"
)

var buildCmd = &cobra.Command{
	Use: "build",
	Run: func(cmd *cobra.Command, args []string) {
		tag, _ := cmd.Flags().GetString("tag")
		dockerfile, _ := cmd.Flags().GetString("dockerfile")
		fmt.Println(tag)
		fmt.Println(dockerfile)

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
