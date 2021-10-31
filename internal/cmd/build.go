package cmd

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	specsv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/spf13/cobra"

	"go.alexhamlin.co/zeroimage/internal/image"
	"go.alexhamlin.co/zeroimage/internal/ociarchive"
	"go.alexhamlin.co/zeroimage/internal/tarlayer"
)

var buildCmd = &cobra.Command{
	Use:   "build [flags] ENTRYPOINT",
	Short: "Build an image from an entrypoint binary",
	Args:  cobra.ExactArgs(1),
	Run:   runBuild,
}

var defaultPlatform = runtime.GOOS + "/" + runtime.GOARCH

var (
	buildFromArchive    string
	buildOutput         string
	buildTargetPlatform string
)

func init() {
	rootCmd.AddCommand(buildCmd)

	buildCmd.Flags().StringVar(&buildFromArchive, "from-archive", "", "Use an existing image archive as a base")
	buildCmd.Flags().StringVarP(&buildOutput, "output", "o", "", "Write the image archive to this path (default [ENTRYPOINT].tar)")
	buildCmd.Flags().StringVar(&buildTargetPlatform, "target-platform", defaultPlatform, "Set the target platform of the image")

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

	targetPlatform, err := image.ParsePlatform(buildTargetPlatform)
	if err != nil {
		log.Fatal("Could not parse target platform: ", err)
	}

	var img image.Image
	if buildFromArchive == "" {
		log.Println("Building image from scratch")
		img.SetPlatform(targetPlatform)
	} else {
		log.Printf("Loading base image: %s", buildFromArchive)
		base, err := os.Open(buildFromArchive)
		if err != nil {
			log.Fatal("Unable to load base archive: ", err)
		}
		index, err := ociarchive.LoadArchive(base)
		if err != nil {
			log.Fatal("Unable to load base archive: ", err)
		}
		base.Close()
		platformIndex := index.SelectByPlatform(targetPlatform)
		if len(platformIndex) != 1 {
			log.Fatalf(
				"Could not find a single base image matching the %s platform",
				image.FormatPlatform(targetPlatform),
			)
		}
		img, err = platformIndex[0].GetImage(context.Background())
		if err != nil {
			log.Fatal("Unable to load base image: ", err)
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

	img.AppendLayer(layer)
	img.Config.History = append(img.Config.History, specsv1.History{
		Created:   now(),
		CreatedBy: "zeroimage",
		Comment:   "entrypoint layer",
	})

	img.Config.Created = now()
	img.Config.Config.Entrypoint = []string{entrypointTargetPath}
	img.Config.Config.Cmd = nil

	log.Printf("Writing image: %s", buildOutput)
	output, err := os.Create(buildOutput)
	if err != nil {
		log.Fatal("Unable to create output file: ", err)
	}
	if err := ociarchive.WriteArchive(img, output); err != nil {
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
