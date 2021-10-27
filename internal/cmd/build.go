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
	Use:   "build [flags] IMAGE ENTRYPOINT",
	Short: "Build a container image from an entrypoint binary",
	Args:  cobra.ExactArgs(2),
	Run:   run,
}

var (
	buildFromArchive string
	buildOutput      string
	buildTargetArch  string
	buildTargetOS    string
)

func init() {
	rootCmd.AddCommand(buildCmd)

	buildCmd.Flags().StringVar(&buildFromArchive, "from-archive", "", "Use a Docker tar archive as the base image")
	buildCmd.Flags().StringVar(&buildOutput, "output", "", "Write a Docker tar archive as output")
	buildCmd.Flags().StringVar(&buildTargetArch, "target-arch", runtime.GOARCH, "Set the target architecture of the image")
	buildCmd.Flags().StringVar(&buildTargetOS, "target-os", runtime.GOOS, "Set the target OS of the image")

	buildCmd.MarkFlagFilename("from-archive", "tar")
	buildCmd.MarkFlagFilename("output", "tar")
}

func run(_ *cobra.Command, args []string) {
	var (
		targetImageName      = args[0]
		entrypointSourcePath = args[1]
	)

	targetImageReference, err := name.ParseReference(targetImageName)
	if err != nil {
		log.Fatal("Invalid image reference: ", err)
	}

	entrypointBase := filepath.Base(entrypointSourcePath)
	entrypointTargetPath := "/" + entrypointBase

	if buildOutput == "" {
		buildOutput = entrypointSourcePath + ".tar"
	}

	var image v1.Image
	if buildFromArchive == "" {
		log.Println("Building image from scratch")
		image = empty.Image
	} else {
		buildFromArchive = filepath.Clean(buildFromArchive)
		log.Printf("Loading base image from %s", buildFromArchive)
		opener := func() (io.ReadCloser, error) { return os.Open(buildFromArchive) }
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
	configFile = configFile.DeepCopy()
	configFile.Created = v1.Time{Time: time.Now().UTC()}
	configFile.Config.Entrypoint = []string{entrypointTargetPath}
	configFile.Config.Cmd = nil
	image, err = mutate.ConfigFile(image, configFile)
	if err != nil {
		log.Fatal("Failed to set entrypoint in image config: ", err)
	}

	log.Printf("Writing archive of %s to %s", targetImageReference, buildOutput)
	err = tarball.WriteToFile(buildOutput, targetImageReference, image)
	if err != nil {
		log.Fatal("Unable to write image: ", err)
	}
}
