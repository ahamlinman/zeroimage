// Package image provides common types to represent container images and their
// filesystem layers, based on the Go types defined by the OCI Image Layout
// Specification.
package image

import (
	"context"
	"io"

	"github.com/opencontainers/go-digest"
	specsv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// Image represents a container image targeting a single platform.
type Image struct {
	Layers []Layer
	// Config represents the OCI image configuration for this image.
	Config specsv1.Image
	// Platform represents the "platform" value for this image in the "manifests"
	// array of an OCI image index.
	Platform specsv1.Platform
	// Annotations represents the "annotations" value for the OCI image manifest
	// associated with this image.
	Annotations map[string]string
}

// Layer represents a single filesystem layer in a container image.
type Layer struct {
	Descriptor specsv1.Descriptor
	DiffID     digest.Digest
	Blob       func(context.Context) (io.ReadCloser, error)
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
