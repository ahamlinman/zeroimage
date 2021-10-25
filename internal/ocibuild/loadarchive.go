package ocibuild

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/opencontainers/go-digest"
	specsv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// LoadArchive loads an OCI image from a tar archive into an Image.
//
// The archive must comply with the OCI Image Layout Specification, and must
// contain exactly one image. All blobs referenced by manifests must appear in
// the archive.
func LoadArchive(r io.Reader) (*Image, error) {
	var (
		ll       loadedLayout
		img      Image
		manifest specsv1.Manifest
	)

	// There's a big impedance mismatch between the OCI layout spec, which is all
	// about content-addressable blobs indexed by manifest files, and tar
	// archives, which are not great at facilitating random access to said blobs.
	// We start by loading the entire image into a structure that we can more
	// easily process, which is based directly on the expected layout defined by
	// https://github.com/opencontainers/image-spec/blob/main/image-layout.md.
	if err := ll.populateFromTar(tar.NewReader(r)); err != nil {
		return nil, fmt.Errorf("invalid archive: %w", err)
	}

	if ll.Layout == nil || ll.Layout.Version == "" {
		return nil, fmt.Errorf("invalid archive: missing or invalid %s", specsv1.ImageLayoutFile)
	}
	if ll.Index == nil {
		return nil, errors.New("invalid archive: missing index.json")
	}

	// In theory we could support multi-platform images by having the user of this
	// method request the specific platform they're looking for, but it's probably
	// not a critical feature. Base archives will probably come from skopeo, which
	// writes single-platform layouts by default.
	if len(ll.Index.Manifests) != 1 {
		return nil, errors.New("archive must contain exactly 1 manifest")
	}
	if ll.Index.Manifests[0].MediaType != specsv1.MediaTypeImageManifest {
		return nil, errors.New("unsupported media type for manifest")
	}

	err := ll.extractJSON(ll.Index.Manifests[0].Digest, &manifest)
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	if manifest.Config.MediaType != specsv1.MediaTypeImageConfig {
		return nil, errors.New("unsupported media type for image config")
	}

	// This is the part where we finally start loading the things we care about
	// into an ocibuilder.Image.

	err = ll.extractJSON(manifest.Config.Digest, &img.Config)
	if err != nil {
		return nil, fmt.Errorf("reading image config: %w", err)
	}

	if len(img.Config.RootFS.DiffIDs) != len(manifest.Layers) {
		return nil, fmt.Errorf("manifest layer count does not match DiffID count")
	}
	for i, layerDesc := range manifest.Layers {
		blob, ok := ll.Blobs[layerDesc.Digest]
		if !ok {
			// From the spec: "The blobs directory MAY be missing referenced blobs, in
			// which case the missing blobs SHOULD be fulfilled by an external blob
			// store." For our purposes, it should not be a big deal to ignore the
			// part about external blob stores.
			return nil, fmt.Errorf("blob %s not found", layerDesc.Digest)
		}
		img.Layers = append(img.Layers, Layer{
			Descriptor: layerDesc,
			DiffID:     img.Config.RootFS.DiffIDs[i],
			Blob:       blob,
		})
	}

	// Since ocibuild always appends history entries for new layers, we're going
	// to make sure that all existing layers have history entries even if the
	// source image did not provide them.
	layersWithoutHistory := len(img.Layers)
	for _, h := range img.Config.History {
		if layersWithoutHistory == 0 {
			break
		}
		if !h.EmptyLayer {
			layersWithoutHistory--
		}
	}
	for i := 0; i < layersWithoutHistory; i++ {
		img.Config.History = append(img.Config.History, specsv1.History{
			Created: img.Config.Created,
			Comment: "imported by zeroimage",
		})
	}

	return &img, nil
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
