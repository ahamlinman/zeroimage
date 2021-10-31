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
func LoadArchive(r io.Reader) (image.Image, error) {
	var (
		ll       loadedLayout
		img      image.Image
		manifest specsv1.Manifest
	)

	// In theory we could take some kind of "opener" instead of a reader, and
	// avoid loading the entire archive into memory like this.
	if err := ll.populateFromTar(tar.NewReader(r)); err != nil {
		return image.Image{}, fmt.Errorf("invalid archive: %w", err)
	}

	if ll.Layout == nil || ll.Layout.Version == "" {
		return image.Image{}, fmt.Errorf("invalid archive: missing or invalid %s", specsv1.ImageLayoutFile)
	}
	if ll.Index == nil {
		return image.Image{}, errors.New("invalid archive: missing index.json")
	}

	// In theory we could set up images for all of the manifests in the archive.
	if len(ll.Index.Manifests) != 1 {
		return image.Image{}, errors.New("archive must contain exactly 1 manifest")
	}
	if ll.Index.Manifests[0].MediaType != specsv1.MediaTypeImageManifest {
		return image.Image{}, errors.New("unsupported media type for manifest")
	}

	err := ll.extractJSON(ll.Index.Manifests[0].Digest, &manifest)
	if err != nil {
		return image.Image{}, fmt.Errorf("reading manifest: %w", err)
	}

	if manifest.Config.MediaType != specsv1.MediaTypeImageConfig {
		return image.Image{}, errors.New("unsupported media type for image config")
	}

	// This is the part where we finally start loading the things we care about
	// into an image.Image.

	img.Annotations = manifest.Annotations
	if platform := ll.Index.Manifests[0].Platform; platform != nil {
		img.Platform = *platform
	}

	// TODO: May want to update img.Platform based on values in the config if
	// necessary, including fields not defined in specs-go as of this writing.
	err = ll.extractJSON(manifest.Config.Digest, &img.Config)
	if err != nil {
		return image.Image{}, fmt.Errorf("reading image config: %w", err)
	}

	if len(img.Config.RootFS.DiffIDs) != len(manifest.Layers) {
		return image.Image{}, fmt.Errorf("manifest layer count does not match DiffID count")
	}
	for i, layerDesc := range manifest.Layers {
		blob, ok := ll.Blobs[layerDesc.Digest]
		if !ok {
			// From the spec: "The blobs directory MAY be missing referenced blobs, in
			// which case the missing blobs SHOULD be fulfilled by an external blob
			// store." For the sake of simplicity we're going to let that SHOULD do
			// some work, at least for now.
			return image.Image{}, fmt.Errorf("blob %s not found", layerDesc.Digest)
		}
		img.Layers = append(img.Layers, image.Layer{
			Descriptor: layerDesc,
			DiffID:     img.Config.RootFS.DiffIDs[i],
			Blob: func(_ context.Context) (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(blob)), nil
			},
		})
	}

	return img, nil
}

type loadedLayout struct {
	Layout *specsv1.ImageLayout
	Index  *specsv1.Index
	Blobs  map[digest.Digest][]byte
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
			err = ll.addBlob(header.Name, tr)
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

func (ll *loadedLayout) addBlob(name string, r io.Reader) error {
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
