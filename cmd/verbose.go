package cmd

import (
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func doVerbose(cmd *cobra.Command, args []string) {
	if verbose {
		logrus.SetLevel(logrus.DebugLevel)
	}
}
