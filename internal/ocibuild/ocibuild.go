package ocibuild

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"hash"
	"io"
	"time"

	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	specsv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"go.alexhamlin.co/zeroimage/internal/tarbuild"
)

// Image represents a full OCI image, including the contents of all layers.
type Image struct {
	Config specsv1.Image
	Layers []Layer
}

// Layer represents a single layer in an OCI image, including its full content.
type Layer struct {
	Descriptor specsv1.Descriptor
	DiffID     digest.Digest
	Blob       []byte
}

// AppendLayer adds the provided layer to the image.
//
// If the digest or size fields of the layer's descriptor are empty, AppendLayer
// will populate them from the provided blob.
func (img *Image) AppendLayer(layer Layer) {
	if layer.Descriptor.Digest == "" {
		layer.Descriptor.Digest = digest.FromBytes(layer.Blob)
	}
	if layer.Descriptor.Size == 0 {
		layer.Descriptor.Size = int64(len(layer.Blob))
	}

	img.Layers = append(img.Layers, layer)
	img.Config.RootFS.DiffIDs = append(img.Config.RootFS.DiffIDs, layer.DiffID)
	img.Config.History = append(img.Config.History, specsv1.History{
		Created:   now(),
		CreatedBy: "zeroimage",
	})
}

// LayerBuilder provides an interface to create a tar-based filesystem layer in
// an image.
type LayerBuilder struct {
	*tarbuild.Builder

	img      *Image
	buf      bytes.Buffer
	zw       *gzip.Writer
	tarHash  hash.Hash
	gzipHash hash.Hash
}

// NewLayer creates a tar archive builder that will append a new filesystem
// layer to img when closed.
func (img *Image) NewLayer() *LayerBuilder {
	lb := &LayerBuilder{
		img:      img,
		tarHash:  digest.Canonical.Hash(),
		gzipHash: digest.Canonical.Hash(),
	}
	lb.zw = gzip.NewWriter(io.MultiWriter(&lb.buf, lb.gzipHash))
	lb.Builder = tarbuild.NewBuilder(io.MultiWriter(lb.zw, lb.tarHash))
	return lb
}

// Close appends the layer created by lb to the associated image.
func (lb *LayerBuilder) Close() error {
	if err := lb.Builder.Close(); err != nil {
		return err
	}
	if err := lb.zw.Close(); err != nil {
		return err
	}
	lb.img.AppendLayer(Layer{
		Blob:   lb.buf.Bytes(),
		DiffID: digest.NewDigest(digest.Canonical, lb.tarHash),
		Descriptor: specsv1.Descriptor{
			MediaType: specsv1.MediaTypeImageLayerGzip,
			Digest:    digest.NewDigest(digest.Canonical, lb.gzipHash),
			Size:      int64(lb.buf.Len()),
		},
	})
	return nil
}

// WriteArchive writes the image as a tar archive to w, with the creation time
// set to the current time.
func (img *Image) WriteArchive(w io.Writer) error {
	iw := imageWriter{
		tar:             tarbuild.NewBuilder(w),
		image:           img,
		hasAlgorithmDir: make(map[digest.Algorithm]bool),
	}
	return iw.WriteArchive()
}

type imageWriter struct {
	tar             *tarbuild.Builder
	image           *Image
	hasAlgorithmDir map[digest.Algorithm]bool
}

func (iw *imageWriter) WriteArchive() error {
	iw.tar.AddDirectory("blobs/")

	for _, layer := range iw.image.Layers {
		iw.addBlob(layer.Descriptor.Digest, layer.Blob)
	}

	config := iw.image.Config
	config.Created = now()
	configDescriptor, err := iw.addJSONBlob(specsv1.MediaTypeImageConfig, config)
	if err != nil {
		return err
	}

	manifest := specsv1.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		Config:    configDescriptor,
	}
	for _, layer := range iw.image.Layers {
		manifest.Layers = append(manifest.Layers, layer.Descriptor)
	}
	manifestDescriptor, err := iw.addJSONBlob(specsv1.MediaTypeImageManifest, manifest)
	if err != nil {
		return err
	}

	index := specsv1.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		Manifests: []specsv1.Descriptor{manifestDescriptor},
	}
	if err := iw.addJSONFile("index.json", index); err != nil {
		return err
	}

	layout := specsv1.ImageLayout{Version: specsv1.ImageLayoutVersion}
	if err := iw.addJSONFile(specsv1.ImageLayoutFile, layout); err != nil {
		return err
	}

	return iw.tar.Close()
}

func (iw *imageWriter) addBlob(digest digest.Digest, blob []byte) {
	path := "blobs/" + string(digest.Algorithm()) + "/" + digest.Encoded()
	iw.ensureAlgorithmDirectory(digest.Algorithm())
	iw.tar.AddFileContent(path, blob)
}

func (iw *imageWriter) addJSONBlob(mediaType string, v interface{}) (specsv1.Descriptor, error) {
	encoded, err := json.Marshal(v)
	if err != nil {
		return specsv1.Descriptor{}, err
	}
	desc := specsv1.Descriptor{
		MediaType: mediaType,
		Digest:    digest.FromBytes(encoded),
		Size:      int64(len(encoded)),
	}
	iw.addBlob(desc.Digest, encoded)
	return desc, nil
}

func (iw *imageWriter) addJSONFile(path string, v interface{}) error {
	encoded, err := json.Marshal(v)
	if err != nil {
		return err
	}
	iw.tar.AddFileContent(path, encoded)
	return nil
}

func (iw *imageWriter) ensureAlgorithmDirectory(alg digest.Algorithm) {
	if iw.hasAlgorithmDir[alg] {
		return
	}
	iw.tar.AddDirectory("blobs/" + string(alg) + "/")
	iw.hasAlgorithmDir[alg] = true
}

func now() *time.Time {
	now := time.Now()
	return &now
}
