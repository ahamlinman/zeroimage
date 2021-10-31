package cmd

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	specsv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/spf13/cobra"

	"go.alexhamlin.co/zeroimage/internal/ocibuild"
)

var buildCmd = &cobra.Command{
	Use:   "build [flags] ENTRYPOINT",
	Short: "Build an image from an entrypoint binary",
	Args:  cobra.ExactArgs(1),
	Run:   runBuild,
}

var (
	buildFromArchive string
	buildOutput      string
	buildTargetArch  string
	buildTargetOS    string
)

func init() {
	rootCmd.AddCommand(buildCmd)

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

	var image *ocibuild.Image
	if buildFromArchive == "" {
		log.Println("Building image from scratch")
		image = &ocibuild.Image{
			Config: specsv1.Image{OS: buildTargetOS, Architecture: buildTargetArch},
		}
	} else {
		log.Printf("Loading base image: %s", buildFromArchive)
		base, err := os.Open(buildFromArchive)
		if err != nil {
			log.Fatal("Unable to load base image: ", err)
		}
		image, err = ocibuild.LoadArchive(base)
		if err != nil {
			log.Fatal("Unable to load base image: ", err)
		}
		base.Close()
		if image.Config.OS != buildTargetOS || image.Config.Architecture != buildTargetArch {
			log.Fatalf(
				"Base image platform %s/%s does not match output platform %s/%s",
				image.Config.OS, image.Config.Architecture,
				buildTargetOS, buildTargetArch,
			)
		}
	}

	log.Printf("Adding entrypoint: %s", entrypointTargetPath)
	entrypoint, err := os.Open(entrypointSourcePath)
	if err != nil {
		log.Fatal("Unable to read entrypoint: ", err)
	}
	builder := ocibuild.NewLayerBuilder()
	builder.Add(entrypointTargetPath, entrypoint)
	layer, err := builder.Finish()
	if err != nil {
		log.Fatal("Failed to build entrypoint layer: ", err)
	}

	image.AppendLayer(layer, specsv1.History{
		Created:   now(),
		CreatedBy: "zeroimage",
		Comment:   "entrypoint layer",
	})
	image.Config.Config.Entrypoint = []string{entrypointTargetPath}
	image.Config.Config.Cmd = nil

	log.Printf("Writing image: %s", buildOutput)
	output, err := os.Create(buildOutput)
	if err != nil {
		log.Fatal("Unable to create output file: ", err)
	}
	if err := image.WriteArchive(output); err != nil {
		log.Fatal("Failed to write image: ", err)
	}
	if err := output.Close(); err != nil {
		log.Fatal("Failed to write image: ", err)
	}
}

func now() *time.Time {
	now := time.Now().UTC()
	return &now
}
