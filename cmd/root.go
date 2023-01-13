package cmd

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"

	"devpod/cigno/pkg"
)

var (
	imageBaseArray []string
	tarballArray   []string
	folderArray    []string
	dockerfile     = "./Dockerfile"
	buildDir       = "."
	tag            = ""
	outFile        = ""
	dryRun         = false
	validate       = false
	verbose        = false
)

var rootCmd = cobra.Command{
	Run: func(cmd *cobra.Command, args []string) {
		dryRun := tag == "" || dryRun

		// engine ready
		var err error
		buildDir, err = filepath.Abs(buildDir)
		if err != nil {
			panic(err)
		}
		engine := pkg.Engine{
			BuildDir:     buildDir,
			BuildContext: map[string]*pkg.BuildContext{},
		}

		imageBase := map[string]string{}
		for _, item := range imageBaseArray {
			ss := strings.Split(item, "=")
			imageBase[ss[0]] = ss[1]
			engine.AddBase(ss[0], ss[1])
		}
		tarballPaths := map[string]string{}
		for _, item := range tarballArray {
			ss := strings.Split(item, "=")
			tarballPaths[ss[0]] = ss[1]
			engine.AddTarball(ss[0], ss[1])
		}
		folderPaths := map[string]string{}
		for _, item := range folderArray {
			ss := strings.Split(item, "=")
			folderPaths[ss[0]] = ss[1]
			engine.AddFolder(ss[0], ss[1])
		}

		logrus.Infof("image base: %s", imageBase)
		logrus.Infof("tarball path: %s", tarballPaths)
		logrus.Infof("folder path: %s", folderPaths)

		var bs []byte
		if dockerfile == "" {
			buf := bytes.NewBuffer(bs)
			writer := io.Writer(buf)
			for idx := range args {
				io.WriteString(writer, args[idx])
				io.WriteString(writer, "\n")
			}
			bs = buf.Bytes()

			logrus.Info("dockerfile command from args: \n", string(bs))
		} else {
			fs := afero.NewOsFs()
			var df afero.File
			df, err := fs.Open(dockerfile)
			if err != nil {
				panic(err)
			}

			bs, _ = io.ReadAll(df)
		}

		if validate {
			return
		}

		var options []pkg.BuildOption
		if outFile != "" {
			options = append(options, pkg.OutFileOption(outFile, tag))
		} else if dryRun {
			// nothing for now
		} else {
			options = append(options, pkg.PushOption(tag))
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

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(buildCmd)

	flags := rootCmd.PersistentFlags()
	flags.StringVarP(&dockerfile, "dockerfile", "f", dockerfile, "dockerfile")
	flags.StringVarP(&buildDir, "context", "c", buildDir, "")
	flags.StringVarP(&tag, "tag", "t", "", "")
	flags.BoolVar(&dryRun, "dry-run", false, "")
	flags.BoolVarP(&validate, "validate", "v", false, "")
	flags.StringArrayVar(&imageBaseArray, "image-base", []string{}, "")
	flags.StringArrayVar(&tarballArray, "tarball", []string{}, "")
	flags.StringArrayVar(&folderArray, "folder", []string{}, "")
	flags.StringVarP(&outFile, "output-file", "o", "", "")
}
