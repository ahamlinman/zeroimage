// The zeroimage tool builds a "from scratch" OCI image archive from a single
// statically linked executable.
//
// The images produced by this tool may not satisfy the requirements of many
// applications. See the zeroimage README for a discussion of the caveats
// associated with this tool.
package main

import (
	"flag"
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

var (
	flagEntrypoint = flag.String("entrypoint", "", "Path to the entrypoint binary")
	flagOS         = flag.String("os", runtime.GOOS, "OS to write to the image manifest")
	flagArch       = flag.String("arch", runtime.GOARCH, "Architecture to write to the image manifest")
	flagOutput     = flag.String("output", "", "Path to write the tar output archive to")
)

func main() {
	flag.Parse()
	if *flagEntrypoint == "" || *flagOutput == "" {
		flag.Usage()
		os.Exit(1)
	}

	entrypointPath := filepath.Base(*flagEntrypoint)
	image := ocibuild.Image{
		Config: specsv1.Image{
			OS:           *flagOS,
			Architecture: *flagArch,
			Config: specsv1.ImageConfig{
				Entrypoint: []string{"/" + entrypointPath},
			},
		},
	}

	entrypoint, err := os.Open(*flagEntrypoint)
	if err != nil {
		log.Fatal("reading entrypoint:", err)
	}
	layer := image.NewLayer()
	layer.AddFile(entrypointPath, entrypoint)
	if err := layer.Close(); err != nil {
		log.Fatal("building entrypoint layer:", err)
	}
	entrypoint.Close()

	output, err := os.Create(*flagOutput)
	if err != nil {
		log.Fatal("opening output:", err)
	}
	if err := image.WriteArchive(output); err != nil {
		log.Fatal("writing image:", err)
	}
	if err := output.Close(); err != nil {
		log.Fatal("writing image:", err)
	}
}
