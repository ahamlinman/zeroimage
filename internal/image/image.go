// Package image provides common types to represent container images and their
// filesystem layers, based on the Go types defined by the OCI Image Layout
// Specification.
package image

import (
	"context"
	"io"
	"sort"

	"github.com/containerd/containerd/platforms"
	"github.com/opencontainers/go-digest"
	specsv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// Index represents an OCI image index that references platform specific
// container images.
type Index []IndexEntry

// IndexEntry represents a reference to a platform specific container image in
// an OCI image index.
type IndexEntry struct {
	Platform specsv1.Platform
	GetImage func(context.Context) (Image, error)
}

// SelectByPlatform returns a new Index containing the subset of images in idx
// that are compatible with the provided platform, in order of decreasing
// preference, following standard platform matching rules as defined by
// https://pkg.go.dev/github.com/containerd/containerd/platforms.
func (idx Index) SelectByPlatform(platform specsv1.Platform) Index {
	matcher := platforms.Only(platform)

	var selected Index
	for _, img := range idx {
		if matcher.Match(img.Platform) {
			selected = append(selected, img)
		}
	}

	sort.Slice(selected, func(i, j int) bool {
		return matcher.Less(selected[i].Platform, selected[j].Platform)
	})

	return selected
}

// Image represents a platform specific container image.
type Image struct {
	Layers []Layer
	// Config represents the OCI image configuration for this image.
	Config Config
	// Platform represents the "platform" value for this image in the "manifests"
	// array of an OCI image index.
	Platform specsv1.Platform
	// Annotations represents the "annotations" value for the OCI image manifest
	// associated with this image.
	Annotations map[string]string
}

// Config represents an OCI image configuration structure, extended with
// properties defined by the spec but not implemented in the upstream Go type as
// of this writing.
type Config struct {
	specsv1.Image
	OSVersion  string   `json:"os.version,omitempty"`
	OSFeatures []string `json:"os.features,omitempty"`
	Variant    string   `json:"variant,omitempty"`
}

// Layer represents a single filesystem layer in a container image.
type Layer struct {
	Descriptor specsv1.Descriptor
	DiffID     digest.Digest
	OpenBlob   func(context.Context) (io.ReadCloser, error)
}

// AppendLayer appends layer to img.Layers and updates corresponding values of
// img.Config.
func (img *Image) AppendLayer(layer Layer) {
	img.Layers = append(img.Layers, layer)
	img.Config.RootFS.DiffIDs = append(img.Config.RootFS.DiffIDs, layer.DiffID)
}

// SetPlatform sets img.Platform and updates corresponding values of img.Config.
func (img *Image) SetPlatform(platform specsv1.Platform) {
	img.Platform = platform
	img.Config.OS = platform.OS
	img.Config.Architecture = platform.Architecture
}
