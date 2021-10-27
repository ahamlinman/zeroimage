package cmd

import (
	"log"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "zeroimage",
	Short: "Build lightweight container images for single-binary programs",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		log.SetPrefix("[zeroimage] ")
		log.SetFlags(0)
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
