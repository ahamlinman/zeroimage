package image

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	specsv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// Supported JSON manifest formats. The Docker-specific formats are compatible
// enough with the OCI-specified definitions that unmarshaling into the Go type
// defined by OCI should give us at least enough data to be useful.
var (
	SupportedIndexMediaTypes = []string{
		specsv1.MediaTypeImageIndex,
		"application/vnd.docker.distribution.manifest.list.v2+json",
	}
	SupportedManifestMediaTypes = []string{
		specsv1.MediaTypeImageManifest,
		"application/vnd.docker.distribution.manifest.v2+json",
	}
)

var (
	supportedIndexMediaTypes    = toStringSet(SupportedIndexMediaTypes)
	supportedManifestMediaTypes = toStringSet(SupportedManifestMediaTypes)
)

func toStringSet(ss []string) map[string]bool {
	set := make(map[string]bool, len(ss))
	for _, s := range ss {
		set[s] = true
	}
	return set
}

// Loader represents a source of manifest and blob information for container
// images.
type Loader interface {
	// OpenRootManifest returns a reader for a JSON-encoded entrypoint manifest,
	// which may be an OCI image index, Docker v2 manifest list, OCI image
	// manifest, or Docker v2 image manifest.
	OpenRootManifest(context.Context) (io.ReadCloser, error)
	// OpenManifest returns a reader for a JSON-encoded manifest that matches the
	// provided digest, which may be an OCI image index, Docker v2 manifest list,
	// OCI image manifest, or Docker v2 image manifest.
	OpenManifest(context.Context, digest.Digest) (io.ReadCloser, error)
	// OpenBlob returns a reader for a blob whose content matches the provided
	// digest.
	OpenBlob(context.Context, digest.Digest) (io.ReadCloser, error)
}

// Load builds an image index using the provided Loader. Methods on the returned
// Index, as well as on all Images loaded from the Index, will use the same
// Loader to access image configuration and filesystem layer blobs.
func Load(ctx context.Context, l Loader) (Index, error) {
	loader := loader{Loader: l}
	if err := loader.InitRootIndex(ctx); err != nil {
		return nil, err
	}
	return loader.BuildIndex(ctx)
}

type loader struct {
	Loader

	// Only modified during initialization, safe to avoid locking.
	rootIndex     specsv1.Index
	nestedIndexes map[digest.Digest]specsv1.Index

	// May be modified concurrently as images are accessed. Our use case fits well
	// with sync.Map: each entry only needs to be written once, and different
	// goroutines are likely touching different images in the index, which means
	// they would touch disjoint keys in these maps.
	manifests sync.Map
	configs   sync.Map
}

func (l *loader) InitRootIndex(ctx context.Context) error {
	rdr, err := l.OpenRootManifest(ctx)
	if err != nil {
		return err
	}
	defer rdr.Close()

	rootContent, err := io.ReadAll(rdr)
	if err != nil {
		return err
	}

	var root struct {
		MediaType string          `json:"mediaType"`
		Manifests json.RawMessage `json:"manifests"`
	}
	err = json.Unmarshal(rootContent, &root)
	if err != nil {
		return err
	}

	if supportedIndexMediaTypes[root.MediaType] || len(root.Manifests) > 0 {
		return json.Unmarshal(rootContent, &l.rootIndex)
	} else if supportedManifestMediaTypes[root.MediaType] {
		return l.initRootWithManifest(rootContent)
	} else {
		return fmt.Errorf("unsupported manifest type %s", root.MediaType)
	}
}

func (l *loader) initRootWithManifest(content []byte) error {
	var manifest specsv1.Manifest
	err := json.Unmarshal(content, &manifest)
	if err != nil {
		return err
	}

	dgst := digest.FromBytes(content)
	l.manifests.Store(dgst, manifest)

	// Depending on the implementation of the loader, this might implicitly rely
	// on manifest loads always checking the "cache" first. A remote registry
	// should always allow pulling manifests by digest, for example, but some
	// custom loader may or may not do that for what it thinks is the "root"
	// manifest. Not sure how big of a deal that might be.
	l.rootIndex = specsv1.Index{
		Versioned: specs.Versioned{SchemaVersion: 2},
		Manifests: []specsv1.Descriptor{{
			MediaType: specsv1.MediaTypeImageManifest,
			Digest:    dgst,
			Size:      int64(len(content)),
		}},
	}
	return nil
}

func (l *loader) BuildIndex(ctx context.Context) (Index, error) {
	manifestDescriptors, err := l.getAllManifestDescriptors(ctx)
	if err != nil {
		return nil, err
	}

	idx := make(Index, len(manifestDescriptors))
	for i, md := range manifestDescriptors {
		md := md
		platform, err := l.getPlatformByManifestDescriptor(ctx, md)
		if err != nil {
			return nil, err
		}
		idx[i] = IndexEntry{
			Platform: platform,
			GetImage: func(ctx context.Context) (Image, error) {
				return l.BuildImage(ctx, md)
			},
		}
	}
	return idx, nil
}

func (l *loader) getAllManifestDescriptors(ctx context.Context) ([]specsv1.Descriptor, error) {
	indexes, err := l.getAllIndexes(ctx)
	if err != nil {
		return nil, err
	}

	var descriptors []specsv1.Descriptor
	for _, idx := range indexes {
		for _, desc := range idx.Manifests {
			if supportedManifestMediaTypes[desc.MediaType] {
				descriptors = append(descriptors, desc)
			}
		}
	}
	return descriptors, nil
}

func (l *loader) getAllIndexes(ctx context.Context) ([]specsv1.Index, error) {
	indexes := []specsv1.Index{l.rootIndex}
	for _, desc := range l.rootIndex.Manifests {
		if supportedIndexMediaTypes[desc.MediaType] {
			nested, err := l.getNestedIndex(ctx, desc.Digest)
			if err != nil {
				return nil, fmt.Errorf("loading nested index: %w", err)
			}
			indexes = append(indexes, nested)
		}
	}
	return indexes, nil
}

func (l *loader) getNestedIndex(ctx context.Context, dgst digest.Digest) (specsv1.Index, error) {
	if nested, ok := l.nestedIndexes[dgst]; ok {
		return nested, nil
	}

	var nested specsv1.Index
	err := l.readJSONManifest(ctx, dgst, &nested)
	if err != nil {
		return specsv1.Index{}, err
	}

	if l.nestedIndexes == nil {
		l.nestedIndexes = make(map[digest.Digest]specsv1.Index)
	}
	l.nestedIndexes[dgst] = nested
	return nested, nil
}

func (l *loader) BuildImage(ctx context.Context, manifestDescriptor specsv1.Descriptor) (Image, error) {
	platform, err := l.getPlatformByManifestDescriptor(ctx, manifestDescriptor)
	if err != nil {
		return Image{}, err
	}

	manifest, err := l.getManifest(ctx, manifestDescriptor.Digest)
	if err != nil {
		return Image{}, err
	}

	config, err := l.getConfig(ctx, manifest.Config.Digest)
	if err != nil {
		return Image{}, err
	}

	if len(manifest.Layers) != len(config.RootFS.DiffIDs) {
		return Image{}, errors.New("manifest layer count does not match diff ID count")
	}

	layers := make([]Layer, len(manifest.Layers))
	for i, layerDesc := range manifest.Layers {
		layerDesc := layerDesc
		layerDesc.MediaType = normalizeLayerMediaType(layerDesc.MediaType)

		if isNondistributableMediaType(layerDesc.MediaType) {
			// TODO: It definitely feels wrong that this affects the process of
			// *loading* the image rather than *pushing* it, however I'm not
			// convinced that any of the rest of the program is prepared to handle
			// this kind of layer. Should revisit this in the future.
			return Image{}, errors.New("image contains nondistributable layers")
		}

		layers[i] = Layer{
			Descriptor: layerDesc,
			DiffID:     config.RootFS.DiffIDs[i],
			OpenBlob: func(ctx context.Context) (io.ReadCloser, error) {
				return l.OpenBlob(ctx, layerDesc.Digest)
			},
		}
	}

	return Image{
		Layers:      layers,
		Config:      config,
		Platform:    platform,
		Annotations: manifest.Annotations,
	}, nil
}

func (l *loader) getPlatformByManifestDescriptor(ctx context.Context, md specsv1.Descriptor) (specsv1.Platform, error) {
	if md.Platform != nil {
		return *md.Platform, nil
	}

	manifest, err := l.getManifest(ctx, md.Digest)
	if err != nil {
		return specsv1.Platform{}, err
	}

	config, err := l.getConfig(ctx, manifest.Config.Digest)
	if err != nil {
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

func (l *loader) getManifest(ctx context.Context, dgst digest.Digest) (specsv1.Manifest, error) {
	if m, ok := l.manifests.Load(dgst); ok {
		return m.(specsv1.Manifest), nil
	}

	// TODO: Deduplicate these reads?
	var manifest specsv1.Manifest
	err := l.readJSONManifest(ctx, dgst, &manifest)
	if err != nil {
		return specsv1.Manifest{}, err
	}

	m, _ := l.manifests.LoadOrStore(dgst, manifest)
	return m.(specsv1.Manifest), nil
}

func (l *loader) getConfig(ctx context.Context, dgst digest.Digest) (Config, error) {
	if c, ok := l.configs.Load(dgst); ok {
		return c.(Config), nil
	}

	// TODO: Deduplicate these reads?
	var config Config
	err := l.readJSONBlob(ctx, dgst, &config)
	if err != nil {
		return Config{}, err
	}

	c, _ := l.configs.LoadOrStore(dgst, config)
	return c.(Config), nil
}

func (l *loader) readJSONManifest(ctx context.Context, dgst digest.Digest, v interface{}) error {
	rdr, err := l.OpenManifest(ctx, dgst)
	if err != nil {
		return err
	}
	defer rdr.Close()
	return json.NewDecoder(rdr).Decode(v)
}

func (l *loader) readJSONBlob(ctx context.Context, dgst digest.Digest, v interface{}) error {
	rdr, err := l.OpenBlob(ctx, dgst)
	if err != nil {
		return err
	}
	defer rdr.Close()
	return json.NewDecoder(rdr).Decode(v)
}

func normalizeLayerMediaType(mediaType string) string {
	// From my reading of both the Docker and OCI specifications, and my analysis
	// of real-world Docker images, I don't expect any issues with this direct
	// media type translation.
	//
	// https://github.com/moby/moby/blob/master/image/spec/v1.md
	// https://github.com/opencontainers/image-spec/blob/main/layer.md
	//
	// One of the examples in the Docker (Moby) spec seems to imply that you don't
	// actually need tar entries for newly added directories in the layer
	// changeset, where the OCI spec is more explicit about this. However, these
	// examples seem inconsistent with wording elsewhere in the Docker spec that
	// refers to "files and directories:"
	//
	// - "looking for files and directories that have been added, modified, or
	//   removed"
	// - "added and modified files and directories in their entirety"
	//
	// In practice, it seems that Docker consistently includes changeset entries
	// for newly added directories. Plus, both specs are clear that changesets are
	// extracted as normal tar archives outside of the special handling of
	// whiteout files, so I'd assume that runtimes have some general way to handle
	// this weird situation if it comes up in a crafted image.
	switch mediaType {
	case "application/vnd.docker.image.rootfs.diff.tar.gzip":
		return specsv1.MediaTypeImageLayerGzip
	case "application/vnd.docker.image.rootfs.foreign.diff.tar.gzip":
		return specsv1.MediaTypeImageLayerNonDistributableGzip
	default:
		return mediaType
	}
}

func isNondistributableMediaType(mediaType string) bool {
	// This should also cover the "+gzip" and "+zstd" suffixes. I can't imagine
	// the spec adding to the media subtype after the ".tar" part.
	return strings.HasPrefix(mediaType, specsv1.MediaTypeImageLayerNonDistributable)
}
