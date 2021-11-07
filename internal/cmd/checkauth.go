package cmd

import (
	"context"
	"log"

	"github.com/spf13/cobra"

	"go.alexhamlin.co/zeroimage/internal/registry"
)

var checkAuthCmd = &cobra.Command{
	Use:   "check-auth [flags] IMAGE",
	Short: "Check the current permissions for a remote registry",
	Args:  cobra.ExactArgs(1),
	Run:   runCheckAuth,
}

var (
	checkAuthPush bool
)

func init() {
	rootCmd.AddCommand(checkAuthCmd)

	checkAuthCmd.Flags().BoolVar(&checkAuthPush, "push", false, "Check that the image can be pushed")
}

func runCheckAuth(_ *cobra.Command, args []string) {
	if !checkAuthPush {
		log.Fatal("Must provide at least one scope to check")
	}

	err := registry.CheckPushAuth(context.Background(), args[0])
	if err != nil {
		// TODO: Separate different kinds of errors with different exit codes.
		log.Fatal("Auth check failed: ", err)
	}
	log.Print("Verified push access for ", args[0])
}
