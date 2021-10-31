package image

import (
	"context"
	"io"

	"github.com/opencontainers/go-digest"
	specsv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

type Image struct {
	Config specsv1.Image
	Layers []Layer
}

type Layer struct {
	Descriptor specsv1.Descriptor
	DiffID     digest.Digest
	Blob       func(context.Context) (io.ReadCloser, error)
}

func (img *Image) AppendLayer(layer Layer, hist ...specsv1.History) {
	img.Layers = append(img.Layers, layer)
	img.Config.RootFS.DiffIDs = append(img.Config.RootFS.DiffIDs, layer.DiffID)
	img.Config.History = append(img.Config.History, hist...)
}
