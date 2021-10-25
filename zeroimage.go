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
	"log"
	"os"
	"path/filepath"
	"runtime"

	// Required by github.com/opencontainers/go-digest
	_ "crypto/sha256"
	_ "crypto/sha512"

	specsv1 "github.com/opencontainers/image-spec/specs-go/v1"

	"go.alexhamlin.co/zeroimage/internal/ocibuild"
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
	entrypointTargetPath := "/bin/" + entrypointBase

	if *flagOutput == "" {
		*flagOutput = entrypointSourcePath + ".tar"
	}

	var image *ocibuild.Image
	if *flagBase == "" {
		log.Println("Building image from scratch")
		image = &ocibuild.Image{
			Config: specsv1.Image{OS: *flagOS, Architecture: *flagArch},
		}
	} else {
		log.Printf("Loading base image: %s", *flagBase)
		base, err := os.Open(*flagBase)
		if err != nil {
			log.Fatal("Unable to load base image: ", err)
		}
		image, err = ocibuild.LoadArchive(base)
		if err != nil {
			log.Fatal("Unable to load base image: ", err)
		}
		base.Close()
		if image.Config.OS != *flagOS || image.Config.Architecture != *flagArch {
			log.Fatalf(
				"Base image platform %s/%s does not match output platform %s/%s",
				image.Config.OS, image.Config.Architecture,
				*flagOS, *flagArch,
			)
		}
	}

	log.Printf("Adding entrypoint: %s", entrypointTargetPath)
	entrypoint, err := os.Open(entrypointSourcePath)
	if err != nil {
		log.Fatal("Unable to read entrypoint: ", err)
	}
	layer := image.NewLayer()
	layer.AddDirectory("bin/")
	layer.AddFile(entrypointTargetPath, entrypoint)
	if err := layer.Close(); err != nil {
		log.Fatal("Failed to build entrypoint layer: ", err)
	}
	entrypoint.Close()

	image.Config.Config.Entrypoint = []string{entrypointTargetPath}
	image.Config.Config.Cmd = nil

	log.Printf("Writing image: %s", *flagOutput)
	output, err := os.Create(*flagOutput)
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
