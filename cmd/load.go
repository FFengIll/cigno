package cmd

import (
	"cigno/pkg"

	"github.com/spf13/cobra"
)

var loadImageTag string

var loadCmd = &cobra.Command{
	Use:   "load FILE -t IMAGE",
	Short: "Import an image archive (docker-save tar) and push it to a registry",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return pkg.LoadImage(args[0], loadImageTag)
	},
}

func init() {
	loadCmd.Flags().StringVarP(&loadImageTag, "tag", "t", "", "tag used in the `name:tag` format")
	_ = loadCmd.MarkFlagRequired("tag")
	rootCmd.AddCommand(loadCmd)
}
