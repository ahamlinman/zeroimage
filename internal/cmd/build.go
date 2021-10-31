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

	buildCmd.Flags().StringVar(&buildFrom, "from", "", "Use an image from a remote registry as a base")
	buildCmd.Flags().StringVar(&buildFromArchive, "from-archive", "", "Use an OCI image archive as a base")
	buildCmd.Flags().StringVarP(&buildOutput, "output", "o", "", "Write an OCI image archive to this path (default [ENTRYPOINT].tar)")
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

	image, err := loadBuildBase()
	if err != nil {
		log.Fatal("Unable to load base image: ", err)
	}

	log.Printf("Using entrypoint: %s", entrypointTargetPath)
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
	if configFile.OS != buildTargetOS || configFile.Architecture != buildTargetArch {
		log.Fatalf(
			"Base image platform %s/%s does not match output platform %s/%s",
			configFile.OS, configFile.Architecture,
			buildTargetOS, buildTargetArch,
		)
	}
	configFile.Config.Entrypoint = []string{entrypointTargetPath}
	configFile.Config.Cmd = nil
	image, err = mutate.ConfigFile(image, configFile)
	if err != nil {
		log.Fatal("Failed to configure image: ", err)
	}

	log.Printf("Writing image to archive: %s", buildOutput)
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

func loadBuildBase() (v1.Image, error) {
	switch {
	case buildFromArchive != "":
		log.Printf("Using base image from archive: %s", buildFromArchive)
		return loadArchiveImageBase()
	case buildFrom != "":
		log.Printf("Using base image from registry: %s", buildFrom)
		return loadRegistryBuildBase()
	default:
		log.Println("Building image from scratch")
		return loadScratchBuildBase()
	}
}

func loadScratchBuildBase() (v1.Image, error) {
	return mutate.ConfigFile(empty.Image, &v1.ConfigFile{
		OS:           buildTargetOS,
		Architecture: buildTargetArch,
	})
}

func loadRegistryBuildBase() (v1.Image, error) {
	ref, err := name.ParseReference(buildFrom)
	if err != nil {
		log.Fatal("Unable to use registry image: ", err)
	}

	return remote.Image(ref)
}

func loadArchiveImageBase() (v1.Image, error) {
	base, err := os.Open(buildFromArchive)
	if err != nil {
		log.Fatal("Unable to load base image: ", err)
	}
	defer base.Close()

	archive, err := ociarchive.LoadArchive(base)
	if err != nil {
		log.Fatal("Unable to load base image: ", err)
	}

	return archive.Image()
}
