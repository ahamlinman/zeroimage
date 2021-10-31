package ociarchive

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"reflect"
	"strings"

	"github.com/opencontainers/go-digest"
	specsv1 "github.com/opencontainers/image-spec/specs-go/v1"

	"go.alexhamlin.co/zeroimage/internal/image"
)

// LoadArchive loads a container image from a tar archive whose contents comply
// with the OCI Image Layout Specification, and that contains exactly one image
// manifest.
//
// The current implementation of LoadArchive buffers all of the archive's blobs
// in memory, and requires that all blobs referenced by manifests appear in the
// archive itself.
func LoadArchive(r io.Reader) (image.Index, error) {
	var ll loadedLayout
	if err := ll.populateFromTar(tar.NewReader(r)); err != nil {
		return nil, fmt.Errorf("invalid archive: %w", err)
	}
	if ll.Layout == nil || ll.Layout.Version == "" {
		return nil, fmt.Errorf("invalid archive: missing or invalid %s", specsv1.ImageLayoutFile)
	}
	if ll.Index == nil {
		return nil, errors.New("invalid archive: missing index.json")
	}

	manifestDescriptors, err := ll.manifestDescriptors()
	if err != nil {
		return nil, fmt.Errorf("invalid index: %w", err)
	}

	var idx image.Index
	for _, md := range manifestDescriptors {
		md := md

		platform, err := ll.loadImagePlatform(md)
		if err != nil {
			return nil, fmt.Errorf("invalid manifest: %w", err)
		}

		idx = append(idx, image.IndexEntry{
			Platform: platform,
			Image: func(_ context.Context) (image.Image, error) {
				img := image.Image{Platform: platform}

				var manifest specsv1.Manifest
				if err := ll.extractJSON(md.Digest, &manifest); err != nil {
					return image.Image{}, err
				}
				img.Annotations = manifest.Annotations

				var config specsv1.Image
				if err := ll.extractJSON(manifest.Config.Digest, &config); err != nil {
					return image.Image{}, err
				}
				img.Config = config

				if len(config.RootFS.DiffIDs) != len(manifest.Layers) {
					return image.Image{}, errors.New("manifest layer count does not match diff ID count")
				}
				for i, layerDesc := range manifest.Layers {
					blob, ok := ll.Blobs[layerDesc.Digest]
					if !ok {
						return image.Image{}, fmt.Errorf("blob %s missing from archive", layerDesc.Digest)
					}
					img.Layers = append(img.Layers, image.Layer{
						Descriptor: layerDesc,
						DiffID:     config.RootFS.DiffIDs[i],
						Blob: func(_ context.Context) (io.ReadCloser, error) {
							return io.NopCloser(bytes.NewReader(blob)), nil
						},
					})
				}

				return img, nil
			},
		})
	}

	return idx, nil
}

type loadedLayout struct {
	Layout *specsv1.ImageLayout
	Index  *specsv1.Index
	Blobs  map[digest.Digest][]byte
}

func (ll *loadedLayout) loadImagePlatform(manifestDesc specsv1.Descriptor) (specsv1.Platform, error) {
	if manifestDesc.Platform != nil && !reflect.DeepEqual(*manifestDesc.Platform, specsv1.Platform{}) {
		return *manifestDesc.Platform, nil
	}

	var manifest specsv1.Manifest
	if err := ll.extractJSON(manifestDesc.Digest, &manifest); err != nil {
		return specsv1.Platform{}, err
	}
	var config extendedImage
	if err := ll.extractJSON(manifest.Config.Digest, &config); err != nil {
		return specsv1.Platform{}, err
	}

	return specsv1.Platform{
		OS:           config.OS,
		Architecture: config.Architecture,
		OSVersion:    config.OSVersion,
		OSFeatures:   config.OSFeatures,
		Variant:      config.Variant,
	}, nil
}

func (ll *loadedLayout) manifestDescriptors() ([]specsv1.Descriptor, error) {
	indices := []specsv1.Index{*ll.Index}
	for _, desc := range ll.Index.Manifests {
		if desc.MediaType != specsv1.MediaTypeImageIndex {
			continue
		}
		var index specsv1.Index
		if err := ll.extractJSON(desc.Digest, &index); err != nil {
			return nil, err
		}
		indices = append(indices, index)
	}

	var mds []specsv1.Descriptor
	for _, idx := range indices {
		for _, desc := range idx.Manifests {
			if desc.MediaType == specsv1.MediaTypeImageManifest {
				mds = append(mds, desc)
			}
		}
	}
	return mds, nil
}

func (ll *loadedLayout) extractJSON(dgst digest.Digest, v interface{}) error {
	blob, ok := ll.Blobs[dgst]
	if !ok {
		return fmt.Errorf("blob %s not found", dgst)
	}
	return json.Unmarshal(blob, v)
}

func (ll *loadedLayout) populateFromTar(tr *tar.Reader) error {
	for {
		header, err := tr.Next()
		switch {
		case errors.Is(err, io.EOF):
			return nil
		case err != nil:
			return err
		}

		switch {
		case strings.HasPrefix(header.Name, "blobs/") && header.Typeflag == tar.TypeReg:
			err = ll.populateBlob(header.Name, tr)
		case header.Name == "index.json":
			err = json.NewDecoder(tr).Decode(&ll.Index)
		case header.Name == specsv1.ImageLayoutFile:
			err = json.NewDecoder(tr).Decode(&ll.Layout)
		default:
			// The spec does not seem to preclude the presence of additional files in
			// the layout, as long as all of the required files are there.
		}
		if err != nil {
			return err
		}
	}
}

func (ll *loadedLayout) populateBlob(name string, r io.Reader) error {
	pathAlg := path.Base(path.Dir(name))
	pathDigest := path.Base(name)
	dgst := digest.NewDigestFromEncoded(digest.Algorithm(pathAlg), pathDigest)
	if err := dgst.Validate(); err != nil {
		return fmt.Errorf("blob name %q does not match any supported digest format: %w", name, err)
	}

	var buf bytes.Buffer
	verifier := dgst.Verifier()
	if _, err := io.Copy(io.MultiWriter(&buf, verifier), r); err != nil {
		return fmt.Errorf("reading %q: %w", name, err)
	}
	if !verifier.Verified() {
		return fmt.Errorf("content of %q does not match digest indicated by name", name)
	}

	if ll.Blobs == nil {
		ll.Blobs = make(map[digest.Digest][]byte)
	}
	ll.Blobs[dgst] = buf.Bytes()
	return nil
}
