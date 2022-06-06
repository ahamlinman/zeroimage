package cmd

import (
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/docker/cli/cli/config"
	"github.com/docker/cli/cli/config/types"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/spf13/cobra"
)

var loginCmd = &cobra.Command{
	Use:   "login [flags] [REGISTRY]",
	Short: "Save login credentials for a remote registry",
	Args:  cobra.ExactArgs(1),
	Run:   runLogin,
}

var (
	loginUsername      string
	loginPasswordStdin bool
)

func init() {
	rootCmd.AddCommand(loginCmd)

	loginCmd.Flags().StringVarP(&loginUsername, "username", "u", "", "The username to log in with")
	loginCmd.Flags().BoolVar(&loginPasswordStdin, "password-stdin", false, "Take the password from stdin")
}

func runLogin(_ *cobra.Command, args []string) {
	if loginUsername == "" {
		log.Fatal("Must provide a username to log in with")
	}
	if !loginPasswordStdin {
		log.Fatal("Must provide password via stdin")
	}

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

	rawPassword, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		log.Fatal("Unable to read password: ", err)
	}
	password := strings.TrimSuffix(string(rawPassword), "\n")
	password = strings.TrimSuffix(password, "\r")

	conf, err := config.Load(os.Getenv("DOCKER_CONFIG"))
	if err != nil {
		log.Fatal("Unable to read Docker configuration: ", err)
	}

	creds := conf.GetCredentialsStore(serverAddress)
	err = creds.Store(types.AuthConfig{
		ServerAddress: serverAddress,
		Username:      loginUsername,
		Password:      password,
	})
	if err != nil {
		log.Fatal("Unable to save login credentials: ", err)
	}

	err = conf.Save()
	if err != nil {
		log.Fatal("Unable to save login credentials: ", err)
	}

	log.Print("Login credentials saved to Docker configuration")
}
