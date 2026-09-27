package cmd

import (
	"fmt"

	"cigno/pkg/cache"

	"github.com/spf13/cobra"
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
	Use:   "cigno",
	Short: "Fast container image builder without a Docker daemon, using OCI operations",
	Long: `Cigno builds container images without a Docker daemon.

It assembles images from Dockerfiles using pure OCI operations (via crane),
so base images are pulled layer-wise with local caching, and COPY --from
supports image rebase and tarball sources. Designed for artifact image
assembly and CI pipelines.

Use "cigno build -f Dockerfile" to get started.`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(buildCmd)
	rootCmd.AddCommand(cacheCmd)

	flags := rootCmd.PersistentFlags()
	flags.BoolVar(&verbose, "verbose", false, "run with verbose information")
}

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Manage the local base image cache",
}

var cacheListCmd = &cobra.Command{
	Use:   "list",
	Short: "List cached base image refs",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := cache.New("")
		if err != nil {
			return err
		}
		refs, err := c.List()
		if err != nil {
			return err
		}
		if len(refs) == 0 {
			fmt.Println("(cache is empty)")
			return nil
		}
		for _, ref := range refs {
			fmt.Println(ref)
		}
		return nil
	},
}

var cacheClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Remove all cached images",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := cache.New("")
		if err != nil {
			return err
		}
		return c.Clear()
	},
}

func init() {
	cacheCmd.AddCommand(cacheListCmd)
	cacheCmd.AddCommand(cacheClearCmd)
}
