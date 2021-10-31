package ocibuild

import (
	"bytes"
	"compress/gzip"
	"context"
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
	Blob       func(context.Context) (io.ReadCloser, error)
}

// LayerBuilder provides an interface to create a new filesystem layer in an
// image.
type LayerBuilder struct {
	*tarbuild.Builder

	img      *Image
	buf      bytes.Buffer
	zw       *gzip.Writer
	tarHash  hash.Hash
	gzipHash hash.Hash
}

// NewLayer returns a tar archive builder that will append a new layer to img
// when closed.
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

	layer := Layer{
		Descriptor: specsv1.Descriptor{
			MediaType: specsv1.MediaTypeImageLayerGzip,
			Digest:    digest.NewDigest(digest.Canonical, lb.gzipHash),
			Size:      int64(lb.buf.Len()),
		},
		DiffID: digest.NewDigest(digest.Canonical, lb.tarHash),
		Blob: func(_ context.Context) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(lb.buf.Bytes())), nil
		},
	}

	lb.img.Layers = append(lb.img.Layers, layer)
	lb.img.Config.RootFS.DiffIDs = append(lb.img.Config.RootFS.DiffIDs, layer.DiffID)
	lb.img.Config.History = append(lb.img.Config.History, specsv1.History{
		Created:   now(),
		CreatedBy: "zeroimage",
	})

	return nil
}

// WriteArchive writes the image as a tar archive to w, with the creation time
// set to the current time.
func (img *Image) WriteArchive(w io.Writer) error {
	iw := imageWriter{
		tar:   tarbuild.NewBuilder(w),
		image: img,
	}
	return iw.WriteArchive()
}

type imageWriter struct {
	tar   *tarbuild.Builder
	image *Image
}

func (iw *imageWriter) WriteArchive() error {
	for _, layer := range iw.image.Layers {
		blob, err := layer.Blob(context.TODO())
		if err != nil {
			return err
		}
		err = iw.addBlob(layer.Descriptor, blob)
		if err != nil {
			return err
		}
	}

	config := iw.image.Config // shallow copy
	config.Created = now()

	manifest := specsv1.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		Config:    iw.addJSONBlob(specsv1.MediaTypeImageConfig, config),
	}
	for _, layer := range iw.image.Layers {
		manifest.Layers = append(manifest.Layers, layer.Descriptor)
	}

	iw.addJSONFile("index.json", specsv1.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		Manifests: []specsv1.Descriptor{
			iw.addJSONBlob(specsv1.MediaTypeImageManifest, manifest),
		},
	})

	iw.addJSONFile(specsv1.ImageLayoutFile, specsv1.ImageLayout{
		Version: specsv1.ImageLayoutVersion,
	})

	return iw.tar.Close()
}

func (iw *imageWriter) addBlob(desc specsv1.Descriptor, blob io.Reader) error {
	digest := desc.Digest
	path := "blobs/" + string(digest.Algorithm()) + "/" + digest.Encoded()
	return iw.tar.Add(path, tarbuild.File{
		Reader: blob,
		Mode:   0644,
		Size:   desc.Size,
	})
}

func (iw *imageWriter) addBlobContent(digest digest.Digest, content []byte) {
	path := "blobs/" + string(digest.Algorithm()) + "/" + digest.Encoded()
	iw.tar.AddContent(path, content)
}

func (iw *imageWriter) addJSONBlob(mediaType string, v interface{}) specsv1.Descriptor {
	encoded := mustJSONMarshal(v)
	desc := specsv1.Descriptor{
		MediaType: mediaType,
		Digest:    digest.FromBytes(encoded),
		Size:      int64(len(encoded)),
	}
	iw.addBlobContent(desc.Digest, encoded)
	return desc
}

func (iw *imageWriter) addJSONFile(path string, v interface{}) {
	encoded := mustJSONMarshal(v)
	iw.tar.AddContent(path, encoded)
}

// mustJSONMarshal returns the JSON encoding of v, or panics if v cannot be
// encoded as JSON.
//
// JSON encoding is generally not expected to fail for the Go types defined by
// the OCI image spec, as they are explicitly designed to represent JSON
// documents.
func mustJSONMarshal(v interface{}) []byte {
	encoded, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return encoded
}

func now() *time.Time {
	now := time.Now().UTC()
	return &now
}
