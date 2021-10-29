package cmd

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/spf13/cobra"

	"go.alexhamlin.co/zeroimage/internal/ociarchive"
	"go.alexhamlin.co/zeroimage/internal/tarlayer"
)

var buildCmd = &cobra.Command{
	Use:   "build [flags] ENTRYPOINT",
	Short: "Build an image from an entrypoint binary",
	Args:  cobra.ExactArgs(1),
	Run:   runBuild,
}

var (
	buildFrom        string
	buildFromArchive string
	buildOutput      string
	buildTargetArch  string
	buildTargetOS    string
)

func init() {
	rootCmd.AddCommand(buildCmd)

	buildCmd.Flags().StringVar(&buildFrom, "from", "", "Use an existing image from a registry as a base")
	buildCmd.Flags().StringVar(&buildFromArchive, "from-archive", "", "Use an existing image archive as a base")
	buildCmd.Flags().StringVarP(&buildOutput, "output", "o", "", "Write the image archive to this path (default [ENTRYPOINT].tar)")
	buildCmd.Flags().StringVar(&buildTargetArch, "target-arch", runtime.GOARCH, "Set the target architecture of the image")
	buildCmd.Flags().StringVar(&buildTargetOS, "target-os", runtime.GOOS, "Set the target OS of the image")

	buildCmd.MarkFlagFilename("from-archive", "tar")
	buildCmd.MarkFlagFilename("output", "tar")
}

func runBuild(_ *cobra.Command, args []string) {
	entrypointSourcePath := args[0]
	entrypointBase := filepath.Base(entrypointSourcePath)
	entrypointTargetPath := "/" + entrypointBase

	if buildOutput == "" {
		buildOutput = entrypointSourcePath + ".tar"
	}

	var image v1.Image
	if buildFrom == "" && buildFromArchive == "" {
		log.Println("Building image from scratch")
		image = empty.Image
	} else if buildFrom != "" {
		log.Printf("Using base image from registry: %s", buildFrom)
		ref, err := name.ParseReference(buildFrom)
		if err != nil {
			log.Fatal("Unable to use registry image: ", err)
		}
		image, err = remote.Image(ref)
		if err != nil {
			log.Fatal("Unable to use registry image: ", err)
		}

		configFile, err := image.ConfigFile()
		if err != nil {
			log.Fatal("Unable to use registry image: ", err)
		}
		if configFile.OS != buildTargetOS || configFile.Architecture != buildTargetArch {
			log.Fatalf(
				"Base image platform %s/%s does not match output platform %s/%s",
				configFile.OS, configFile.Architecture,
				buildTargetOS, buildTargetArch,
			)
		}
	} else {
		log.Printf("Loading base image: %s", buildFromArchive)
		base, err := os.Open(buildFromArchive)
		if err != nil {
			log.Fatal("Unable to load base image: ", err)
		}
		archive, err := ociarchive.LoadArchive(base)
		if err != nil {
			log.Fatal("Unable to load base image: ", err)
		}
		base.Close()
		image, err = archive.Image()
		if err != nil {
			log.Fatal("Unable to load base image: ", err)
		}

		configFile, err := image.ConfigFile()
		if err != nil {
			log.Fatal("Unable to load base image: ", err)
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
	layer, err := builder.Finish()
	if err != nil {
		log.Fatal("Failed to build entrypoint layer: ", err)
	}
	entrypoint.Close()

	image, err = mutate.Append(image, mutate.Addendum{
		Layer: layer,
		History: v1.History{
			Created:   v1.Time{Time: time.Now().UTC()},
			CreatedBy: "zeroimage",
			Comment:   "entrypoint layer",
		},
	})
	if err != nil {
		log.Fatal("Failed to add entrypoint layer: ", err)
	}

	configFile, err := image.ConfigFile()
	if err != nil {
		log.Fatal("Unable to read image config: ", err)
	}
	configFile.OS = buildTargetOS
	configFile.Architecture = buildTargetArch
	configFile.Config.Entrypoint = []string{entrypointTargetPath}
	configFile.Config.Cmd = nil
	image, err = mutate.ConfigFile(image, configFile)
	if err != nil {
		log.Fatal("Failed to configure image: ", err)
	}

	log.Printf("Writing image: %s", buildOutput)
	output, err := os.Create(buildOutput)
	if err != nil {
		log.Fatal("Unable to create output file: ", err)
	}
	if err := ociarchive.WriteImage(image, output); err != nil {
		log.Fatal("Failed to write image: ", err)
	}
	if err := output.Close(); err != nil {
		log.Fatal("Failed to write image: ", err)
	}
}
