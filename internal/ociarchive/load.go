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
	return image.Load(context.Background(), ll)
}

type loadedLayout struct {
	Layout *specsv1.ImageLayout
	Index  *specsv1.Index
	Blobs  map[digest.Digest][]byte
}

func (ll loadedLayout) OpenRootManifest(_ context.Context) (io.ReadCloser, error) {
	index, err := json.Marshal(ll.Index)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(index)), nil
}

func (ll loadedLayout) OpenBlob(_ context.Context, dgst digest.Digest) (io.ReadCloser, error) {
	blob, ok := ll.Blobs[dgst]
	if !ok {
		return nil, fmt.Errorf("archive is missing blob %s", dgst)
	}
	return io.NopCloser(bytes.NewReader(blob)), nil
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
