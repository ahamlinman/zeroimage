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

type Builder struct {
	*tarbuild.Builder

	buf      bytes.Buffer
	zw       *gzip.Writer
	tarHash  hash.Hash
	gzipHash hash.Hash
}

func NewBuilder() *Builder {
	lb := &Builder{
		tarHash:  digest.Canonical.Hash(),
		gzipHash: digest.Canonical.Hash(),
	}
	lb.zw = gzip.NewWriter(io.MultiWriter(&lb.buf, lb.gzipHash))
	lb.Builder = tarbuild.NewBuilder(io.MultiWriter(lb.zw, lb.tarHash))
	return lb
}

func (lb *Builder) Finish() (image.Layer, error) {
	if err := lb.Builder.Close(); err != nil {
		return image.Layer{}, err
	}
	if err := lb.zw.Close(); err != nil {
		return image.Layer{}, err
	}

	return image.Layer{
		Descriptor: specsv1.Descriptor{
			MediaType: specsv1.MediaTypeImageLayerGzip,
			Digest:    digest.NewDigest(digest.Canonical, lb.gzipHash),
			Size:      int64(lb.buf.Len()),
		},
		DiffID: digest.NewDigest(digest.Canonical, lb.tarHash),
		Blob: func(_ context.Context) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(lb.buf.Bytes())), nil
		},
	}, nil
}
