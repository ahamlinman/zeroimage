package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	specsv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"golang.org/x/sync/errgroup"

	"go.alexhamlin.co/zeroimage/internal/image"
)

const concurrentLayerUploads = 3

// Push pushes a single container image to a remote OCI registry, using
// credentials from the local Docker keychain to authenticate to the registry if
// necessary.
func Push(ctx context.Context, img image.Image, reference string) error {
	tag, err := name.NewTag(reference)
	if err != nil {
		return err
	}

	transport, err := newTransport(ctx, tag, transport.PushScope)
	if err != nil {
		return err
	}

	p := pusher{
		Tag: tag,
		Client: http.Client{
			Transport: transport,
			Timeout:   httpTimeout,
		},
	}
	return p.Push(ctx, img)
}

type pusher struct {
	Tag    name.Tag
	Client http.Client
}

func (p *pusher) Push(ctx context.Context, img image.Image) error {
	layersCh := make(chan image.Layer, len(img.Layers))
	for _, layer := range img.Layers {
		layersCh <- layer
	}
	close(layersCh)

	eg, ectx := errgroup.WithContext(ctx)

	var configDesc specsv1.Descriptor
	eg.Go(func() (err error) {
		configDesc, err = p.uploadConfig(ectx, img.Config)
		return
	})

	for i := 0; i < concurrentLayerUploads; i++ {
		eg.Go(func() error {
			for layer := range layersCh {
				err := p.uploadLayer(ectx, layer)
				if err != nil {
					return err
				}
			}
			return nil
		})
	}

	err := eg.Wait()
	if err != nil {
		return err
	}

	return p.uploadManifest(ctx, img, configDesc)
}

func (p *pusher) uploadConfig(ctx context.Context, config image.Config) (specsv1.Descriptor, error) {
	configJSON, err := json.Marshal(config)
	if err != nil {
		return specsv1.Descriptor{}, err
	}

	desc := specsv1.Descriptor{
		MediaType: specsv1.MediaTypeImageConfig,
		Digest:    digest.FromBytes(configJSON),
		Size:      int64(len(configJSON)),
	}
	return desc, p.uploadBlob(ctx, desc.Digest, desc.Size, bytes.NewReader(configJSON))
}

func (p *pusher) uploadLayer(ctx context.Context, layer image.Layer) error {
	r, err := layer.OpenBlob(ctx)
	if err != nil {
		return err
	}
	defer r.Close()
	return p.uploadBlob(ctx, layer.Descriptor.Digest, layer.Descriptor.Size, r)
}

func (p *pusher) uploadBlob(ctx context.Context, dgst digest.Digest, size int64, r io.Reader) error {
	if ok, err := p.hasBlob(ctx, dgst); ok || err != nil {
		return err
	}

	uploadURL, err := p.getBlobUploadURL(ctx)
	if err != nil {
		return err
	}

	query, err := url.ParseQuery(uploadURL.RawQuery)
	if err != nil {
		return err
	}
	query.Add("digest", dgst.String())
	uploadURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL.String(), r)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Length", strconv.FormatInt(size, 10))

	resp, err := p.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return transport.CheckError(resp, http.StatusCreated)
}

func (p *pusher) hasBlob(ctx context.Context, dgst digest.Digest) (ok bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, p.url("/blobs/%s", dgst).String(), nil)
	if err != nil {
		return false, err
	}

	resp, err := p.Client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, transport.CheckError(resp, http.StatusOK, http.StatusNotFound)
}

func (p *pusher) getBlobUploadURL(ctx context.Context) (u *url.URL, err error) {
	uploadURL := p.url("/blobs/uploads/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := transport.CheckError(resp, http.StatusAccepted); err != nil {
		return nil, err
	}

	return uploadURL.Parse(resp.Header.Get("Location"))
}

func (p *pusher) uploadManifest(ctx context.Context, img image.Image, configDesc specsv1.Descriptor) error {
	manifest := specsv1.Manifest{
		Versioned:   specs.Versioned{SchemaVersion: 2},
		Config:      configDesc,
		Annotations: img.Annotations,
	}
	for _, layer := range img.Layers {
		manifest.Layers = append(manifest.Layers, layer.Descriptor)
	}

	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return err
	}

	uploadURL := p.url("/manifests/%s", p.Tag.TagStr())
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL.String(), bytes.NewReader(manifestJSON))
	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", specsv1.MediaTypeImageManifest)
	req.Header.Add("Content-Length", strconv.Itoa(len(manifestJSON)))

	resp, err := p.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return transport.CheckError(resp, http.StatusCreated)
}

func (p *pusher) url(format string, v ...interface{}) *url.URL {
	return &url.URL{
		Scheme: p.Tag.Scheme(),
		Host:   p.Tag.RegistryStr(),
		Path:   "/v2/" + p.Tag.RepositoryStr() + fmt.Sprintf(format, v...),
	}
}
