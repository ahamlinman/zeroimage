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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"go.alexhamlin.co/zeroimage/internal/tarbuild"
)

var (
	flagEntrypoint = flag.String("entrypoint", "", "Path to the entrypoint binary")
	flagOS         = flag.String("os", runtime.GOOS, "OS to write to the image manifest")
	flagArch       = flag.String("arch", runtime.GOARCH, "Architecture to write to the image manifest")
	flagOutput     = flag.String("output", "", "Path to write the .tar output archive to")
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

	config := ociConfig{
		Created:      time.Now().Format(time.RFC3339),
		Architecture: *flagArch,
		OS:           *flagOS,
		Config: ociConfigExecParams{
			Entrypoint: []string{"/" + entrypointPath},
		},
		RootFS: ociConfigRootFS{
			Type:    "layers",
			DiffIDs: []string{"sha256:" + sha256Hex(layerTar.Bytes())},
		},
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		log.Fatal("encoding config:", err)
	}

	manifest := ociManifest{
		SchemaVersion: 2,
		Config: ociDescriptor{
			MediaType: "application/vnd.oci.image.config.v1+json",
			Digest:    "sha256:" + sha256Hex(configJSON),
			Size:      len(configJSON),
		},
		Layers: []ociDescriptor{{
			MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
			Digest:    "sha256:" + sha256Hex(layerZip.Bytes()),
			Size:      layerZip.Len(),
		}},
	}

	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		log.Fatal("encoding manifest:", err)
	}

	index := ociIndex{
		SchemaVersion: 2,
		Manifests: []ociDescriptor{{
			MediaType: "application/vnd.oci.image.manifest.v1+json",
			Digest:    "sha256:" + sha256Hex(manifestJSON),
			Size:      len(manifestJSON),
		}},
	}

	indexJSON, err := json.Marshal(index)
	if err != nil {
		log.Fatal("encoding index:", err)
	}

	layout := ociLayout{ImageLayoutVersion: "1.0.0"}
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
	builder.AddFileContent("blobs/sha256/"+sha256Hex(layerZip.Bytes()), layerZip.Bytes())
	builder.AddFileContent("blobs/sha256/"+sha256Hex(configJSON), configJSON)
	builder.AddFileContent("blobs/sha256/"+sha256Hex(manifestJSON), manifestJSON)
	builder.AddFileContent("index.json", indexJSON)
	builder.AddFileContent("oci-layout", layoutJSON)
	if err := builder.Close(); err != nil {
		log.Fatal("building image:", err)
	}

	if err := output.Close(); err != nil {
		log.Fatal("writing image:", err)
	}
}

func sha256Hex(b []byte) string {
	sha := sha256.Sum256(b)
	return hex.EncodeToString(sha[:])
}

type ociLayout struct {
	ImageLayoutVersion string `json:"imageLayoutVersion"`
}

type ociIndex struct {
	SchemaVersion int             `json:"schemaVersion"`
	Manifests     []ociDescriptor `json:"manifests"`
}

type ociManifest struct {
	SchemaVersion int             `json:"schemaVersion"`
	Config        ociDescriptor   `json:"config"`
	Layers        []ociDescriptor `json:"layers"`
}

type ociDescriptor struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int    `json:"size"`
}

type ociConfig struct {
	Created      string              `json:"created"`
	Architecture string              `json:"architecture"`
	OS           string              `json:"os"`
	Config       ociConfigExecParams `json:"config"`
	RootFS       ociConfigRootFS     `json:"rootfs"`
}

type ociConfigExecParams struct {
	Entrypoint []string `json:"Entrypoint"`
}

type ociConfigRootFS struct {
	Type    string   `json:"type"`
	DiffIDs []string `json:"diff_ids"`
}
