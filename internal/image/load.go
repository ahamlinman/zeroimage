package image

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	specsv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// Loader represents a source of manifest and blob information for container
// images.
type Loader interface {
	// OpenRootManifest returns a reader for a JSON-encoded entrypoint manifest,
	// which may be an OCI image index or a compatible image manifest.
	OpenRootManifest(context.Context) (io.ReadCloser, error)
	// OpenBlob returns a reader for the blob matching the provided digest.
	OpenBlob(context.Context, digest.Digest) (io.ReadCloser, error)
}

func Load(ctx context.Context, l Loader) (Index, error) {
	loader := loader{Loader: l}
	if err := loader.InitRootIndex(ctx); err != nil {
		return nil, err
	}
	return loader.BuildIndex(ctx)
}

type loader struct {
	Loader

	rootIndex     specsv1.Index
	nestedIndexes map[digest.Digest]specsv1.Index
	manifests     map[digest.Digest]specsv1.Manifest
	configs       map[digest.Digest]Config
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

	var mediaType struct {
		MediaType string `json:"mediaType"`
	}
	err = json.Unmarshal(rootContent, &mediaType)
	if err != nil {
		return err
	}

	if mediaType.MediaType == specsv1.MediaTypeImageManifest {
		return l.initRootWithManifest(rootContent)
	} else {
		return json.Unmarshal(rootContent, &l.rootIndex)
	}
}

func (l *loader) initRootWithManifest(content []byte) error {
	var manifest specsv1.Manifest
	err := json.Unmarshal(content, &manifest)
	if err != nil {
		return err
	}

	if l.manifests == nil {
		l.manifests = make(map[digest.Digest]specsv1.Manifest)
	}
	dgst := digest.FromBytes(content)
	l.manifests[dgst] = manifest

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

func (l *loader) getAllManifestDescriptors(ctx context.Context) ([]specsv1.Descriptor, error) {
	indexes, err := l.getAllIndexes(ctx)
	if err != nil {
		return nil, err
	}

	var descriptors []specsv1.Descriptor
	for _, idx := range indexes {
		for _, desc := range idx.Manifests {
			if desc.MediaType == specsv1.MediaTypeImageManifest {
				descriptors = append(descriptors, desc)
			}
		}
	}
	return descriptors, nil
}

func (l *loader) getAllIndexes(ctx context.Context) ([]specsv1.Index, error) {
	indexes := []specsv1.Index{l.rootIndex}
	for _, desc := range l.rootIndex.Manifests {
		if desc.MediaType == specsv1.MediaTypeImageIndex {
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
	err := l.readJSONBlob(ctx, dgst, &nested)
	if err != nil {
		return specsv1.Index{}, err
	}

	if l.nestedIndexes == nil {
		l.nestedIndexes = make(map[digest.Digest]specsv1.Index)
	}
	l.nestedIndexes[dgst] = nested
	return nested, nil
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
	if manifest, ok := l.manifests[dgst]; ok {
		return manifest, nil
	}

	var manifest specsv1.Manifest
	err := l.readJSONBlob(ctx, dgst, &manifest)
	if err != nil {
		return specsv1.Manifest{}, err
	}

	if l.manifests == nil {
		l.manifests = make(map[digest.Digest]specsv1.Manifest)
	}
	l.manifests[dgst] = manifest
	return manifest, nil
}

func (l *loader) getConfig(ctx context.Context, dgst digest.Digest) (Config, error) {
	if config, ok := l.configs[dgst]; ok {
		return config, nil
	}

	var config Config
	err := l.readJSONBlob(ctx, dgst, &config)
	if err != nil {
		return Config{}, err
	}

	if l.configs == nil {
		l.configs = make(map[digest.Digest]Config)
	}
	l.configs[dgst] = config
	return config, nil
}

func (l *loader) readJSONBlob(ctx context.Context, dgst digest.Digest, v interface{}) error {
	rdr, err := l.OpenBlob(ctx, dgst)
	if err != nil {
		return err
	}
	defer rdr.Close()
	return json.NewDecoder(rdr).Decode(v)
}
