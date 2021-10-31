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

// Image represents a container image targeting a single platform, including a
// configuration and a list of filesystem layers.
type Image struct {
	Config specsv1.Image
	Layers []Layer
}

// Layer represents a single filesystem layer in a container image.
type Layer struct {
	Descriptor specsv1.Descriptor
	DiffID     digest.Digest
	Blob       func(context.Context) (io.ReadCloser, error)
}

// AppendLayer appends layer to the Layers of img, and appends layer's DiffID to
// the diff IDs listed for the root filesystem in img's Config.
func (img *Image) AppendLayer(layer Layer) {
	img.Layers = append(img.Layers, layer)
	img.Config.RootFS.DiffIDs = append(img.Config.RootFS.DiffIDs, layer.DiffID)
}
