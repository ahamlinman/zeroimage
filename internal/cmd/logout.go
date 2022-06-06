package cmd

import (
	"log"
	"os"

	"github.com/docker/cli/cli/config"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/spf13/cobra"
)

var logoutCmd = &cobra.Command{
	Use:   "logout [REGISTRY]",
	Short: "Delete login credentials for a remote registry",
	Args:  cobra.ExactArgs(1),
	Run:   runLogout,
}

func init() {
	rootCmd.AddCommand(logoutCmd)
}

func runLogout(_ *cobra.Command, args []string) {
	// This is very much inspired by the implementation of "crane auth".
	// https://github.com/google/go-containerregistry/blob/v0.6.0/cmd/crane/cmd/auth.go

	registry, err := name.NewRegistry(args[0])
	if err != nil {
		log.Fatal("Invalid registry: ", err)
	}

	serverAddress := registry.Name()
	if serverAddress == name.DefaultRegistry {
		serverAddress = authn.DefaultAuthKey
	}

	conf, err := config.Load(os.Getenv("DOCKER_CONFIG"))
	if err != nil {
		log.Fatal("Unable to read Docker configuration: ", err)
	}

	creds := conf.GetCredentialsStore(serverAddress)
	err = creds.Erase(serverAddress)
	if err != nil {
		log.Fatal("Unable to erase login credentials: ", err)
	}

	err = conf.Save()
	if err != nil {
		log.Fatal("Unable to erase login credentials: ", err)
	}

	log.Print("Removed login credentials from Docker configuration")
}
