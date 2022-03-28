package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"time"

	specsv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/spf13/cobra"

	"go.alexhamlin.co/zeroimage/internal/image"
	"go.alexhamlin.co/zeroimage/internal/ociarchive"
	"go.alexhamlin.co/zeroimage/internal/registry"
	"go.alexhamlin.co/zeroimage/internal/tarlayer"
)

var buildCmd = &cobra.Command{
	Use:   "build [flags] ENTRYPOINT",
	Short: "Build an image from an entrypoint binary",
	Args:  cobra.ExactArgs(1),
	Run:   runBuild,
}

var (
	defaultPlatform  = runtime.GOOS + "/" + runtime.GOARCH
	layerCreatorName = "zeroimage"
)

func init() {
	if info, ok := debug.ReadBuildInfo(); ok {
		layerCreatorName = info.Main.Path
	}
}

var (
	buildFrom        string
	buildFromArchive string
	buildOutput      string
	buildPlatform    string
	buildPush        string
)

func init() {
	rootCmd.AddCommand(buildCmd)

	buildCmd.Flags().StringVar(&buildFrom, "from", "", "Use an image from a remote registry as a base")
	buildCmd.Flags().StringVar(&buildFromArchive, "from-archive", "", "Use an existing image archive as a base")
	buildCmd.Flags().StringVarP(&buildOutput, "output", "o", "", "Write the image archive to this path (default [ENTRYPOINT].tar)")
	buildCmd.Flags().StringVar(&buildPlatform, "platform", defaultPlatform, "Select the desired platform for the image")
	buildCmd.Flags().StringVar(&buildPush, "push", "", "Push the image to this tag in a remote registry")

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

	platform, err := image.ParsePlatform(buildPlatform)
	if err != nil {
		log.Fatal("Could not parse target platform: ", err)
	}

	img, err := loadBaseImage(platform)
	if err != nil {
		log.Fatal("Unable to load base image: ", err)
	}

	log.Printf("Adding entrypoint: %s", entrypointTargetPath)
	entrypoint, err := os.Open(entrypointSourcePath)
	if err != nil {
		log.Fatal("Unable to read entrypoint: ", err)
	}
	builder := tarlayer.NewBuilder()
	builder.Add(entrypointTargetPath, entrypoint)
	entrypoint.Close()
	layer, err := builder.Finish()
	if err != nil {
		log.Fatal("Failed to build entrypoint layer: ", err)
	}

	img.AppendLayer(layer)
	img.Config.History = append(img.Config.History, specsv1.History{
		Created:   now(),
		CreatedBy: layerCreatorName,
		Comment:   "entrypoint: " + entrypointTargetPath,
	})

	img.Config.Created = now()
	img.Config.Config.Entrypoint = []string{entrypointTargetPath}
	img.Config.Config.Cmd = nil

	err = outputImage(img)
	if err != nil {
		log.Fatal("Failed to output image: ", err)
	}
}

func now() *time.Time {
	now := time.Now().UTC()
	return &now
}

func loadBaseImage(platform specsv1.Platform) (image.Image, error) {
	if buildFromArchive == "" && buildFrom == "" {
		var img image.Image
		img.SetPlatform(platform)
		return img, nil
	}

	var (
		index image.Index
		err   error
	)
	if buildFromArchive != "" {
		index, err = loadBaseFromArchive()
	}
	if buildFrom != "" {
		index, err = loadBaseFromRegistry()
	}
	if err != nil {
		return image.Image{}, err
	}

	index = index.SelectByPlatform(platform)
	if len(index) == 0 {
		return image.Image{}, fmt.Errorf("image does not support %s", image.FormatPlatform(platform))
	}
	if len(index) > 1 {
		// TODO: What is Docker's behavior here? Does it parse the variants to pick
		// the highest version, or does it just pick the last one in the list?
		matches := make([]string, len(index))
		for i, entry := range index {
			matches[i] = image.FormatPlatform(entry.Platform)
		}
		return image.Image{}, fmt.Errorf(
			"cannot decide between multiple matches for %s: %v",
			image.FormatPlatform(platform), matches,
		)
	}

	return index[0].GetImage(context.TODO())
}

func loadBaseFromArchive() (image.Index, error) {
	log.Printf("Loading base image archive: %s", buildFromArchive)

	base, err := os.Open(buildFromArchive)
	if err != nil {
		log.Fatal("Unable to load base archive: ", err)
	}
	defer base.Close()

	return ociarchive.Load(base)
}

func loadBaseFromRegistry() (image.Index, error) {
	log.Printf("Loading base image from registry: %s", buildFrom)
	return registry.Load(context.TODO(), buildFrom)
}

func outputImage(img image.Image) error {
	if buildPush != "" {
		return outputImageToRegistry(img)
	}
	return outputImageToArchive(img)
}

func outputImageToRegistry(img image.Image) error {
	log.Printf("Pushing image to registry: %s", buildPush)
	return registry.PushImage(context.TODO(), img, buildPush)
}

func outputImageToArchive(img image.Image) error {
	log.Printf("Writing image archive: %s", buildOutput)
	output, err := os.Create(buildOutput)
	if err != nil {
		return err
	}
	if err := ociarchive.WriteImage(img, output); err != nil {
		return err
	}
	return output.Close()
}
