package cmd

import (
	"log"
	"os"
	"runtime"

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

var (
	flagFromArchive string
	flagTargetArch  string
	flagTargetOS    string
)

func init() {
	rootCmd.PersistentFlags().StringVar(&flagFromArchive, "from-archive", "", "Use a Docker tar archive as the base image")
	rootCmd.PersistentFlags().StringVar(&flagTargetArch, "target-arch", runtime.GOARCH, "Set the target architecture of the image")
	rootCmd.PersistentFlags().StringVar(&flagTargetOS, "target-os", runtime.GOOS, "Set the target OS of the image")

	rootCmd.MarkPersistentFlagFilename("from-archive", "tar")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
