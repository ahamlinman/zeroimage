package ociarchive

import (
	"context"
	"encoding/json"
	"io"

	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	specsv1 "github.com/opencontainers/image-spec/specs-go/v1"

	"go.alexhamlin.co/zeroimage/internal/image"
	"go.alexhamlin.co/zeroimage/internal/tarbuild"
)

// WriteImage writes a single container image as a tar archive whose contents
// comply with the OCI Image Layout Specification.
func WriteImage(img image.Image, w io.Writer) error {
	iw := imageWriter{
		tar:   tarbuild.NewBuilder(w),
		image: img,
	}
	return iw.WriteImage()
}

type imageWriter struct {
	tar   *tarbuild.Builder
	image image.Image
}

func (iw *imageWriter) WriteImage() error {
	for _, layer := range iw.image.Layers {
		blob, err := layer.OpenBlob(context.TODO())
		if err != nil {
			return err
		}
		err = iw.addBlob(layer.Descriptor, blob)
		if err != nil {
			return err
		}
	}

	manifest := specsv1.Manifest{
		Versioned:   specs.Versioned{SchemaVersion: 2},
		Config:      iw.addJSONBlob(specsv1.MediaTypeImageConfig, iw.image.Config),
		Annotations: iw.image.Annotations,
	}
	for _, layer := range iw.image.Layers {
		manifest.Layers = append(manifest.Layers, layer.Descriptor)
	}

	manifestDesc := iw.addJSONBlob(specsv1.MediaTypeImageManifest, manifest)
	manifestDesc.Platform = &iw.image.Platform

	iw.addJSONFile("index.json", specsv1.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		Manifests: []specsv1.Descriptor{manifestDesc},
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
