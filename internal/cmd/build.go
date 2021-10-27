package cmd

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/spf13/cobra"

	"go.alexhamlin.co/zeroimage/internal/tarlayer"
)

var buildCmd = &cobra.Command{
	Use:   "build [flags] ENTRYPOINT",
	Short: "Build a container image from an entrypoint binary",
	Args:  cobra.MinimumNArgs(1),
	Run:   run,
}

var (
	buildBaseArchive   string
	buildOutputArchive string
	buildTargetArch    string
	buildTargetOS      string
)

func init() {
	rootCmd.AddCommand(buildCmd)

	buildCmd.Flags().StringVar(&buildBaseArchive, "base-archive", "", "Use a Docker tar archive as the base image")
	buildCmd.Flags().StringVar(&buildOutputArchive, "output-archive", "", "Write a Docker tar archive as output")
	buildCmd.Flags().StringVar(&buildTargetArch, "target-arch", runtime.GOARCH, "Set the target architecture of the image")
	buildCmd.Flags().StringVar(&buildTargetOS, "target-os", runtime.GOOS, "Set the target OS of the image")

	buildCmd.MarkFlagFilename("base-archive", "tar")
	buildCmd.MarkFlagFilename("output-archive", "tar")
}

func run(_ *cobra.Command, args []string) {
	entrypointSourcePath := args[0]
	entrypointBase := filepath.Base(entrypointSourcePath)
	entrypointTargetPath := "/" + entrypointBase

	if buildOutputArchive == "" {
		buildOutputArchive = entrypointSourcePath + ".tar"
	}

	var (
		image v1.Image
		err   error
	)

	if buildBaseArchive == "" {
		log.Println("Building image from scratch")
		image = empty.Image
	} else {
		log.Printf("Loading base image from archive: %s", buildBaseArchive)
		opener := func() (io.ReadCloser, error) { return os.Open(buildBaseArchive) }
		image, err = tarball.Image(opener, nil)
		if err != nil {
			log.Fatalf("Unable to load base image: %s", err)
		}

		configFile, err := image.ConfigFile()
		if err != nil {
			log.Fatal("Unable to load base image config: ", err)
		}
		if configFile.OS != buildTargetOS || configFile.Architecture != buildTargetArch {
			log.Fatalf(
				"Base image platform %s/%s does not match output platform %s/%s",
				configFile.OS, configFile.Architecture,
				buildTargetOS, buildTargetArch,
			)
		}
	}

	log.Printf("Adding entrypoint: %s", entrypointTargetPath)
	entrypoint, err := os.Open(entrypointSourcePath)
	if err != nil {
		log.Fatal("Unable to read entrypoint: ", err)
	}
	builder := tarlayer.NewBuilder()
	builder.Add(entrypointTargetPath, entrypoint)
	entrypointLayer, err := builder.Finish()
	if err != nil {
		log.Fatal("Failed to build entrypoint layer: ", err)
	}
	entrypoint.Close()

	image, err = mutate.Append(image, mutate.Addendum{
		Layer: entrypointLayer,
		History: v1.History{
			Created:   v1.Time{Time: time.Now().UTC()},
			CreatedBy: "zeroimage",
			Comment:   "entrypoint layer",
		},
	})
	if err != nil {
		log.Fatal("Failed to add entrypoint layer to image: ", err)
	}

	configFile, err := image.ConfigFile()
	if err != nil {
		log.Fatal("Unable to load image config: ", err)
	}
	config := configFile.Config
	config.Entrypoint = []string{entrypointTargetPath}
	config.Cmd = nil
	image, err = mutate.Config(image, config)
	if err != nil {
		log.Fatal("Failed to set entrypoint in image config: ", err)
	}

	log.Printf("Writing image archive: %s", buildOutputArchive)
	tag := name.MustParseReference("zeroimage:latest")
	err = tarball.WriteToFile(buildOutputArchive, tag, image)
	if err != nil {
		log.Fatal("Unable to write image: ", err)
	}
}
