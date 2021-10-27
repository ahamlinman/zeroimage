// The zeroimage tool builds a "from scratch" OCI image archive from a single
// statically linked executable.
//
// The images produced by this tool may not satisfy the requirements of many
// applications. See the zeroimage README for a discussion of the caveats
// associated with this tool.
package main

import (
	"flag"
	"fmt"
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

	"go.alexhamlin.co/zeroimage/internal/tarlayer"
)

const usageHeader = `zeroimage [options] ENTRYPOINT

Build a single layer OCI image archive using ENTRYPOINT as the entrypoint.
`

var (
	flagArch   = flag.String("arch", runtime.GOARCH, "Set the target architecture of the image")
	flagBase   = flag.String("base", "", "Image archive to use as a base (optional)")
	flagOS     = flag.String("os", runtime.GOOS, "Set the target OS of the image")
	flagOutput = flag.String("output", "", `Write the image archive to this path (default [ENTRYPOINT].tar)`)
)

func init() {
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), usageHeader)
		flag.PrintDefaults()
	}
}

func main() {
	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(1)
	}

	log.SetPrefix("[zeroimage] ")
	log.SetFlags(0)

	entrypointSourcePath := flag.Arg(0)
	entrypointBase := filepath.Base(entrypointSourcePath)
	entrypointTargetPath := "/" + entrypointBase

	if *flagOutput == "" {
		*flagOutput = entrypointSourcePath + ".tar"
	}

	var (
		image v1.Image
		err   error
	)

	if *flagBase == "" {
		log.Println("Building image from scratch")
		image = empty.Image
	} else {
		log.Printf("Loading base image from archive: %s", *flagBase)
		opener := func() (io.ReadCloser, error) { return os.Open(*flagBase) }
		image, err = tarball.Image(opener, nil)
		if err != nil {
			log.Fatalf("Unable to load base image: %s", err)
		}

		configFile, err := image.ConfigFile()
		if err != nil {
			log.Fatal("Unable to load base image config: ", err)
		}
		if configFile.OS != *flagOS || configFile.Architecture != *flagArch {
			log.Fatalf(
				"Base image platform %s/%s does not match output platform %s/%s",
				configFile.OS, configFile.Architecture,
				*flagOS, *flagArch,
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

	log.Printf("Writing image archive: %s", *flagOutput)
	tag := name.MustParseReference("zeroimage:latest")
	err = tarball.WriteToFile(*flagOutput, tag, image)
	if err != nil {
		log.Fatal("Unable to write image: ", err)
	}
}
