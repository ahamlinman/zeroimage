package ociarchive

import (
	"encoding/json"
	"io"

	registryv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	specsv1 "github.com/opencontainers/image-spec/specs-go/v1"

	"go.alexhamlin.co/zeroimage/internal/tarbuild"
)

// WriteArchive writes image to w as a tar archive whose contents are compliant
// with the OCI Image Format Specification.
func WriteImage(image registryv1.Image, w io.Writer) error {
	iw := imageWriter{
		tar:   tarbuild.NewBuilder(w),
		image: image,
	}
	return iw.WriteArchive()
}

type imageWriter struct {
	tar   *tarbuild.Builder
	image registryv1.Image
}

func (iw *imageWriter) WriteArchive() error {
	layers, err := iw.image.Layers()
	if err != nil {
		return err
	}

	var layerDescriptors []specsv1.Descriptor
	for _, layer := range layers {
		mediaType, err := layer.MediaType()
		if err != nil {
			return err
		}
		digestHash, err := layer.Digest()
		if err != nil {
			return err
		}
		size, err := layer.Size()
		if err != nil {
			return err
		}
		lr, err := layer.Compressed()
		if err != nil {
			return err
		}

		dgst := digest.NewDigestFromEncoded(digest.Algorithm(digestHash.Algorithm), digestHash.Hex)
		path := "blobs/" + string(dgst.Algorithm()) + "/" + dgst.Encoded()

		layerDescriptors = append(layerDescriptors, specsv1.Descriptor{
			MediaType: string(mediaType),
			Digest:    dgst,
			Size:      size,
		})
		iw.tar.Add(path, tarbuild.File{
			Reader: lr,
			Size:   size,
			Mode:   0644,
		})
	}

	configFile, err := iw.image.ConfigFile()
	if err != nil {
		return err
	}

	manifest := specsv1.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		Config:    iw.addJSONBlob(specsv1.MediaTypeImageConfig, configFile),
		Layers:    layerDescriptors,
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

func (iw *imageWriter) addBlob(digest digest.Digest, blob []byte) {
	path := "blobs/" + string(digest.Algorithm()) + "/" + digest.Encoded()
	iw.tar.AddContent(path, blob)
}

func (iw *imageWriter) addJSONBlob(mediaType string, v interface{}) specsv1.Descriptor {
	encoded := mustJSONMarshal(v)
	desc := specsv1.Descriptor{
		MediaType: mediaType,
		Digest:    digest.FromBytes(encoded),
		Size:      int64(len(encoded)),
	}
	iw.addBlob(desc.Digest, encoded)
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
