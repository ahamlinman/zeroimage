package tarlayer

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"

	"go.alexhamlin.co/zeroimage/internal/tarbuild"
)

// Layer is a container image layer constructed from a tar archive, that
// implements the v1.Layer interface of the go-containerregistry module.
type Layer struct {
	blob   []byte
	digest v1.Hash
	diffID v1.Hash
}

func (l Layer) Digest() (v1.Hash, error) {
	return l.digest, nil
}

func (l Layer) DiffID() (v1.Hash, error) {
	return l.diffID, nil
}

func (l Layer) Compressed() (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(l.blob)), nil
}

func (l Layer) Uncompressed() (io.ReadCloser, error) {
	return gzip.NewReader(bytes.NewReader(l.blob))
}

func (l Layer) Size() (int64, error) {
	return int64(len(l.blob)), nil
}

func (l Layer) MediaType() (types.MediaType, error) {
	return types.OCILayer, nil
}

// Builder constructs a new Layer using a tar archive builder.
type Builder struct {
	*tarbuild.Builder

	buf      bytes.Buffer
	zw       *gzip.Writer
	tarHash  hash.Hash
	gzipHash hash.Hash
}

// NewBuilder creates a Builder.
func NewBuilder() *Builder {
	lb := &Builder{
		tarHash:  sha256.New(),
		gzipHash: sha256.New(),
	}
	lb.zw = gzip.NewWriter(io.MultiWriter(&lb.buf, lb.gzipHash))
	lb.Builder = tarbuild.NewBuilder(io.MultiWriter(lb.zw, lb.tarHash))
	return lb
}

// Finish closes the tar archive builder and returns a new Layer containing the
// archive's contents.
func (lb *Builder) Finish() (v1.Layer, error) {
	if err := lb.Builder.Close(); err != nil {
		return nil, err
	}
	if err := lb.zw.Close(); err != nil {
		return nil, err
	}

	layer := Layer{
		blob: lb.buf.Bytes(),
		digest: v1.Hash{
			Algorithm: "sha256",
			Hex:       hex.EncodeToString(lb.gzipHash.Sum(nil)),
		},
		diffID: v1.Hash{
			Algorithm: "sha256",
			Hex:       hex.EncodeToString(lb.tarHash.Sum(nil)),
		},
	}
	return layer, nil
}
