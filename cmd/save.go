package cmd

import (
	"cigno/pkg"

	"github.com/spf13/cobra"
)

var saveOutFile string

var saveCmd = &cobra.Command{
	Use:   "save IMAGE -o FILE",
	Short: "Export an image from a registry to a docker-loadable tar",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return pkg.SaveImage(args[0], saveOutFile)
	},
}

func init() {
	saveCmd.Flags().StringVarP(&saveOutFile, "output", "o", "", "output tar file")
	_ = saveCmd.MarkFlagRequired("output")
	rootCmd.AddCommand(saveCmd)
}
