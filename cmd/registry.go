package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"cigno/pkg/registry"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	registryAddr    string
	registryStorage string
)

var registryCmd = &cobra.Command{
	Use:   "registry",
	Short: "Manage embedded registry server",
}

var registryStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the embedded registry server",
	Run: func(cmd *cobra.Command, args []string) {
		// Set default storage path if not specified
		if registryStorage == "" {
			homeDir, _ := os.UserHomeDir()
			registryStorage = fmt.Sprintf("%s/.cigno/registry", homeDir)
		}

		// Create and start server
		server, err := registry.NewSimpleServer(registryAddr, registryStorage)
		if err != nil {
			logrus.Fatalf("creating registry server: %v", err)
		}

		if err := server.Start(); err != nil {
			logrus.Fatalf("starting registry server: %v", err)
		}

		logrus.WithField("addr", server.GetAddr()).Info("registry server started")
		logrus.WithField("storage", server.GetStoragePath()).Info("using storage")

		// Wait for interrupt signal
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan

		logrus.Info("shutting down registry server...")
		if err := server.Stop(); err != nil {
			logrus.Errorf("stopping registry server: %v", err)
		}
	},
}

var registryStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the embedded registry server (if managed by this process)",
	Run: func(cmd *cobra.Command, args []string) {
		logrus.Info("registry server is managed per-process; use Ctrl+C to stop")
	},
}

var registryStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show registry server status",
	Run: func(cmd *cobra.Command, args []string) {
		logrus.Info("registry server is managed per-process; check if port is in use")
		logrus.Infof("configured address: %s", registryAddr)
		if registryStorage != "" {
			logrus.Infof("configured storage: %s", registryStorage)
		}
	},
}

func init() {
	rootCmd.AddCommand(registryCmd)
	registryCmd.AddCommand(registryStartCmd)
	registryCmd.AddCommand(registryStopCmd)
	registryCmd.AddCommand(registryStatusCmd)

	// Flags for start command
	registryStartCmd.Flags().StringVarP(&registryAddr, "addr", "a", "localhost:5000", "Registry address")
	registryStartCmd.Flags().StringVarP(&registryStorage, "storage", "s", "", "Registry storage path")

	// Flags for stop and status commands
	registryStopCmd.Flags().StringVarP(&registryAddr, "addr", "a", "localhost:5000", "Registry address")
	registryStatusCmd.Flags().StringVarP(&registryAddr, "addr", "a", "localhost:5000", "Registry address")
	registryStatusCmd.Flags().StringVarP(&registryStorage, "storage", "s", "", "Registry storage path")
}
