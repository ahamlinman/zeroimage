// Package tarlayer builds container image layers by constructing tar archives.
package tarlayer

import (
	"bytes"
	"compress/gzip"
	"context"
	"hash"
	"io"

	"github.com/opencontainers/go-digest"
	specsv1 "github.com/opencontainers/image-spec/specs-go/v1"

	"go.alexhamlin.co/zeroimage/internal/image"
	"go.alexhamlin.co/zeroimage/internal/tarbuild"
)

// Builder wraps a tarbuild.Builder to create a compressed container image
// layer, computing the digest and diff ID of the layer as it is built.
type Builder struct {
	*tarbuild.Builder

	buf      bytes.Buffer
	zw       *gzip.Writer
	tarHash  hash.Hash
	gzipHash hash.Hash
}

// NewBuilder initializes a Builder that writes a compressed tar archive to an
// in memory buffer.
func NewBuilder() *Builder {
	b := &Builder{
		tarHash:  digest.Canonical.Hash(),
		gzipHash: digest.Canonical.Hash(),
	}
	b.zw = gzip.NewWriter(io.MultiWriter(&b.buf, b.gzipHash))
	b.Builder = tarbuild.NewBuilder(io.MultiWriter(b.zw, b.tarHash))
	return b
}

// Finish closes the embedded tarbuild.Builder, and returns a container image
// layer if all entries were successfully added to the tar archive.
func (b *Builder) Finish() (image.Layer, error) {
	if err := b.Builder.Close(); err != nil {
		return image.Layer{}, err
	}
	if err := b.zw.Close(); err != nil {
		return image.Layer{}, err
	}

	return image.Layer{
		Descriptor: specsv1.Descriptor{
			MediaType: specsv1.MediaTypeImageLayerGzip,
			Digest:    digest.NewDigest(digest.Canonical, b.gzipHash),
			Size:      int64(b.buf.Len()),
		},
		DiffID: digest.NewDigest(digest.Canonical, b.tarHash),
		Blob: func(_ context.Context) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(b.buf.Bytes())), nil
		},
	}, nil
}
