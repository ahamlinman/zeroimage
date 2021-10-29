package ociarchive

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	registryv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/partial"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/opencontainers/go-digest"
	specsv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// Archive represents a container image, loaded into memory from a tar archive
// whose contents comply with the OCI Image Format Specification and that
// contains a single manifest, that implements the v1.Image interface of the
// go-containerregistry module.
type Archive struct {
	Layout *specsv1.ImageLayout
	Index  *specsv1.Index
	Blobs  map[digest.Digest][]byte
}

func LoadArchive(r io.Reader) (*Archive, error) {
	var a Archive
	if err := a.populateFromTar(tar.NewReader(r)); err != nil {
		return nil, fmt.Errorf("invalid archive: %w", err)
	}

	if a.Layout == nil || a.Layout.Version == "" {
		return nil, fmt.Errorf("invalid archive: missing or invalid %s", specsv1.ImageLayoutFile)
	}
	if a.Index == nil {
		return nil, errors.New("invalid archive: missing index.json")
	}

	return &a, nil
}

// Image returns a v1.Image for the go-containerregistry module representing the
// contents of this archive. The archive must contain a single manifest and
// compressed layers, and no referenced blobs may be missing from the archive.
func (a *Archive) Image() (registryv1.Image, error) {
	manifest, err := a.ParsedManifest()
	if err != nil {
		return nil, err
	}

	for _, layer := range manifest.Layers {
		if layer.MediaType != string(types.OCILayer) {
			return nil, errors.New("one or more image layers is not gzip compressed")
		}
	}

	return partial.CompressedToImage(archiveImage{a})
}

// ParsedManifest returns the parsed image manifest if the image contains a
// single manifest. Otherwise, it returns an error.
func (a *Archive) ParsedManifest() (specsv1.Manifest, error) {
	blob, err := a.RawManifest()
	if err != nil {
		return specsv1.Manifest{}, err
	}

	var manifest specsv1.Manifest
	err = json.Unmarshal(blob, &manifest)
	return manifest, err
}

// RawManifest returns the raw bytes of the image manifest if the image contains
// a single manifest. Otherwise, it returns an error.
func (a *Archive) RawManifest() ([]byte, error) {
	if len(a.Index.Manifests) != 1 {
		return nil, errors.New("archives without exactly 1 manifest are not supported")
	}

	dgst := a.Index.Manifests[0].Digest
	blob, ok := a.Blobs[dgst]
	if !ok {
		return nil, errors.New("referenced manifest does not exist in archive")
	}
	return blob, nil
}

func (a *Archive) populateFromTar(tr *tar.Reader) error {
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
			err = a.populateBlob(header.Name, tr)
		case header.Name == "index.json":
			err = json.NewDecoder(tr).Decode(&a.Index)
		case header.Name == specsv1.ImageLayoutFile:
			err = json.NewDecoder(tr).Decode(&a.Layout)
		default:
			// The spec does not seem to preclude the presence of additional files in
			// the layout, as long as all of the required files are there.
		}
		if err != nil {
			return err
		}
	}
}

func (a *Archive) populateBlob(name string, r io.Reader) error {
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

	if a.Blobs == nil {
		a.Blobs = make(map[digest.Digest][]byte)
	}
	a.Blobs[dgst] = buf.Bytes()
	return nil
}

// compressedLayer is an internal implementation of the v1.CompressedImageCore
// interface of go-containerregistry.
type archiveImage struct {
	*Archive
}

func (a archiveImage) MediaType() (types.MediaType, error) {
	if len(a.Index.Manifests) != 1 {
		return "", errors.New("archives without exactly 1 manifest are not supported")
	}
	return types.MediaType(a.Index.Manifests[0].MediaType), nil
}

func (a archiveImage) RawConfigFile() ([]byte, error) {
	manifest, err := a.ParsedManifest()
	if err != nil {
		return nil, err
	}

	dgst := manifest.Config.Digest
	blob, ok := a.Blobs[dgst]
	if !ok {
		return nil, errors.New("referenced config does not exist in archive")
	}
	return blob, nil
}

func (a archiveImage) LayerByDigest(hash registryv1.Hash) (partial.CompressedLayer, error) {
	dgst := digest.NewDigestFromEncoded(digest.Algorithm(hash.Algorithm), hash.Hex)
	blob, ok := a.Blobs[dgst]
	if !ok {
		return nil, errors.New("referenced layer does not exist in archive")
	}
	return compressedLayer{hash, blob}, nil
}

// compressedLayer is an internal implementation of the v1.CompressedLayer
// interface of go-containerregistry.
type compressedLayer struct {
	digest registryv1.Hash
	blob   []byte
}

func (cl compressedLayer) Digest() (registryv1.Hash, error) {
	return cl.digest, nil
}

func (cl compressedLayer) Compressed() (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(cl.blob)), nil
}

func (cl compressedLayer) Size() (int64, error) {
	return int64(len(cl.blob)), nil
}

func (cl compressedLayer) MediaType() (types.MediaType, error) {
	return types.OCILayer, nil
}
