// The zeroimage tool builds a "from scratch" OCI image archive from a single
// statically linked executable.
//
// The images produced by this tool may not satisfy the requirements of many
// applications. Among other things, they do not include a standard directory
// layout, user database, time zone database, TLS root certificates, etc. Your
// application must be prepared to handle the fact that it is running in, quite
// frankly, a broken environment.
package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"flag"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	specsv1 "github.com/opencontainers/image-spec/specs-go/v1"

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

	var layerZip bytes.Buffer
	layerZipWriter := gzip.NewWriter(&layerZip)
	if _, err := io.Copy(layerZipWriter, &layerTar); err != nil {
		log.Fatal("compressing layer:", err)
	}
	if err := layerZipWriter.Close(); err != nil {
		log.Fatal("compressing layer:", err)
	}
	layerZipDigest := digest.FromBytes(layerZip.Bytes())

	now := time.Now()
	imageConfig := specsv1.Image{
		Created:      &now,
		Architecture: *flagArch,
		OS:           *flagOS,
		Config: specsv1.ImageConfig{
			Entrypoint: []string{"/" + entrypointPath},
		},
		RootFS: specsv1.RootFS{
			Type:    "layers",
			DiffIDs: []digest.Digest{digest.FromBytes(layerTar.Bytes())},
		},
	}

	imageConfigJSON, err := json.Marshal(imageConfig)
	if err != nil {
		log.Fatal("encoding config:", err)
	}
	imageConfigDigest := digest.FromBytes(imageConfigJSON)

	manifest := specsv1.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		Config: specsv1.Descriptor{
			MediaType: specsv1.MediaTypeImageConfig,
			Digest:    imageConfigDigest,
			Size:      int64(len(imageConfigJSON)),
		},
		Layers: []specsv1.Descriptor{{
			MediaType: specsv1.MediaTypeImageLayerGzip,
			Digest:    layerZipDigest,
			Size:      int64(layerZip.Len()),
		}},
	}

	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		log.Fatal("encoding manifest:", err)
	}
	manifestDigest := digest.FromBytes(manifestJSON)

	index := specsv1.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		Manifests: []specsv1.Descriptor{{
			MediaType: specsv1.MediaTypeImageManifest,
			Digest:    manifestDigest,
			Size:      int64(len(manifestJSON)),
		}},
	}

	indexJSON, err := json.Marshal(index)
	if err != nil {
		log.Fatal("encoding index:", err)
	}

	layout := specsv1.ImageLayout{Version: specsv1.ImageLayoutVersion}
	layoutJSON, err := json.Marshal(layout)
	if err != nil {
		log.Fatal("encoding layout:", err)
	}

	output, err := os.Create(*flagOutput)
	if err != nil {
		log.Fatal("opening output:", err)
	}

	builder := tarbuild.NewBuilder(output)
	builder.AddDirectory("blobs/")
	builder.AddDirectory("blobs/sha256/")
	builder.AddFileContent("blobs/sha256/"+layerZipDigest.Encoded(), layerZip.Bytes())
	builder.AddFileContent("blobs/sha256/"+imageConfigDigest.Encoded(), imageConfigJSON)
	builder.AddFileContent("blobs/sha256/"+manifestDigest.Encoded(), manifestJSON)
	builder.AddFileContent("index.json", indexJSON)
	builder.AddFileContent(specsv1.ImageLayoutFile, layoutJSON)
	if err := builder.Close(); err != nil {
		log.Fatal("building image:", err)
	}

	if err := output.Close(); err != nil {
		log.Fatal("writing image:", err)
	}
}
