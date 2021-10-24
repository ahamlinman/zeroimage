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
	flagArch   = flag.String("arch", runtime.GOARCH, "Set the target architecture for the image")
	flagOS     = flag.String("os", runtime.GOOS, "Set the target OS for the image")
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

	log.SetPrefix("zeroimage error: ")
	log.SetFlags(0)

	entrypointPath := flag.Arg(0)
	entrypointBase := filepath.Base(entrypointPath)

	if *flagOutput == "" {
		*flagOutput = entrypointPath + ".tar"
	}

	image := ocibuild.Image{
		Config: specsv1.Image{
			OS:           *flagOS,
			Architecture: *flagArch,
			Config: specsv1.ImageConfig{
				Entrypoint: []string{"/" + entrypointBase},
			},
		},
	}

	entrypoint, err := os.Open(entrypointPath)
	if err != nil {
		log.Fatal("reading entrypoint: ", err)
	}
	layer := image.NewLayer()
	layer.AddFile(entrypointBase, entrypoint)
	if err := layer.Close(); err != nil {
		log.Fatal("building entrypoint layer: ", err)
	}
	entrypoint.Close()

	output, err := os.Create(*flagOutput)
	if err != nil {
		log.Fatal("opening output: ", err)
	}
	if err := image.WriteArchive(output); err != nil {
		log.Fatal("writing image: ", err)
	}
	if err := output.Close(); err != nil {
		log.Fatal("writing image: ", err)
	}
}
