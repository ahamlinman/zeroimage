// The zeroimage tool builds a "from scratch" OCI image archive from a single
// statically linked executable.
//
// The images produced by this tool may not satisfy the requirements of many
// applications. See the zeroimage README for a discussion of the caveats
// associated with this tool.
package main

import (
	"bytes"
	"compress/gzip"
	"flag"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"

	// Required by github.com/opencontainers/go-digest
	_ "crypto/sha256"
	_ "crypto/sha512"

	"github.com/opencontainers/go-digest"
	specsv1 "github.com/opencontainers/image-spec/specs-go/v1"

	"go.alexhamlin.co/zeroimage/internal/ocibuild"
	"go.alexhamlin.co/zeroimage/internal/tarbuild"
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

	entrypoint, err := os.Open(*flagEntrypoint)
	if err != nil {
		log.Fatal("reading entrypoint:", err)
	}

	entrypointPath := filepath.Base(*flagEntrypoint)

	var layerTar bytes.Buffer
	layerBuilder := tarbuild.NewBuilder(&layerTar)
	layerBuilder.AddFile(entrypointPath, entrypoint)
	if err := layerBuilder.Close(); err != nil {
		log.Fatal("building layer archive:", err)
	}
	layerTarDigest := digest.FromBytes(layerTar.Bytes())

	var layerZip bytes.Buffer
	layerZipWriter := gzip.NewWriter(&layerZip)
	if _, err := io.Copy(layerZipWriter, &layerTar); err != nil {
		log.Fatal("compressing layer:", err)
	}
	if err := layerZipWriter.Close(); err != nil {
		log.Fatal("compressing layer:", err)
	}

	image := ocibuild.Image{
		Config: specsv1.Image{
			OS:           *flagOS,
			Architecture: *flagArch,
			Config: specsv1.ImageConfig{
				Entrypoint: []string{"/" + entrypointPath},
			},
		},
	}
	image.AppendLayer(ocibuild.Layer{
		Blob:   layerZip.Bytes(),
		DiffID: layerTarDigest,
		Descriptor: specsv1.Descriptor{
			MediaType: specsv1.MediaTypeImageLayerGzip,
			Digest:    digest.FromBytes(layerZip.Bytes()),
			Size:      int64(layerZip.Len()),
		},
	})

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
