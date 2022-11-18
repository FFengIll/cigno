package main

import (
	"bytes"
	"github.com/sirupsen/logrus"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"io"
	"self/use-crane/pkg"
)

var (
	filepath  = "test/sample/Dockerfile"
	buildpath = "./test"
	tag       = "mirrors.tencent.com/devpod/nouse:use-crane"
	dryRun    = false
	rootCmd   = cobra.Command{
		Run: func(cmd *cobra.Command, args []string) {
			dryRun := tag == "" || dryRun

			var bs []byte
			if filepath == "" {
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
				df, err := fs.Open(filepath)
				if err != nil {
					panic(err)
				}

				bs, _ = io.ReadAll(df)
			}

			err := pkg.Run(
				bytes.NewReader(bs),
				buildpath,
				tag,
				dryRun,
			)
			if err != nil {
				panic(err)
			}
		},
	}
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		panic(err)
	}
}

func init() {
	flags := rootCmd.PersistentFlags()
	flags.StringVarP(&filepath, "file", "f", "", "")
	flags.StringVarP(&buildpath, "context", "c", "./", "")
	flags.StringVarP(&tag, "tag", "t", "", "")
	flags.BoolVar(&dryRun, "dry-run", false, "")
}
