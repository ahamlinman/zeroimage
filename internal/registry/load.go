package registry

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/opencontainers/go-digest"

	"go.alexhamlin.co/zeroimage/internal/image"
)

func Load(ctx context.Context, reference string) (image.Index, error) {
	name, err := name.ParseReference(reference)
	if err != nil {
		return nil, err
	}
	authenticator, err := authn.DefaultKeychain.Resolve(name.Context())
	if err != nil {
		authenticator = authn.Anonymous
	}
	transport, err := transport.NewWithContext(
		ctx,
		name.Context().Registry,
		authenticator,
		http.DefaultTransport,
		[]string{name.Scope(transport.PullScope)},
	)
	if err != nil {
		return nil, err
	}

	return image.Load(ctx, loader{
		Client: http.Client{
			Transport: transport,
			Timeout:   10 * time.Second,
		},
		Name: name,
	})
}

type loader struct {
	http.Client
	Name name.Reference
}

func (l loader) OpenRootManifest(ctx context.Context) (io.ReadCloser, error) {
	manifestURL := l.formatURL("/v2/%s/manifests/%s", l.Name.Context().RepositoryStr(), l.Name.Identifier())
	req, err := http.NewRequest(http.MethodGet, manifestURL.String(), nil)
	if err != nil {
		return nil, err
	}

	var accept []string
	accept = append(accept, image.SupportedIndexMediaTypes...)
	accept = append(accept, image.SupportedManifestMediaTypes...)
	req.Header.Set("Accept", strings.Join(accept, ","))

	resp, err := l.Do(req)
	if err != nil {
		return nil, err
	}
	if err := transport.CheckError(resp, http.StatusOK); err != nil {
		return nil, err
	}

	return resp.Body, nil
}

func (l loader) OpenManifest(ctx context.Context, dgst digest.Digest) (io.ReadCloser, error) {
	manifestURL := l.formatURL("/v2/%s/manifests/%s", l.Name.Context().RepositoryStr(), dgst)
	req, err := http.NewRequest(http.MethodGet, manifestURL.String(), nil)
	if err != nil {
		return nil, err
	}

	var accept []string
	accept = append(accept, image.SupportedIndexMediaTypes...)
	accept = append(accept, image.SupportedManifestMediaTypes...)
	req.Header.Set("Accept", strings.Join(accept, ","))

	resp, err := l.Do(req)
	if err != nil {
		return nil, err
	}
	if err := transport.CheckError(resp, http.StatusOK); err != nil {
		return nil, err
	}

	return resp.Body, nil
}

func (l loader) OpenBlob(ctx context.Context, dgst digest.Digest) (io.ReadCloser, error) {
	blobURL := l.formatURL("/v2/%s/blobs/%s", l.Name.Context().RepositoryStr(), dgst)
	req, err := http.NewRequest(http.MethodGet, blobURL.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := l.Do(req)
	if err != nil {
		return nil, err
	}
	if err := transport.CheckError(resp, http.StatusOK); err != nil {
		return nil, err
	}

	return resp.Body, nil
}

func (l loader) formatURL(format string, a ...interface{}) url.URL {
	return url.URL{
		Scheme: l.Name.Context().Registry.Scheme(),
		Host:   l.Name.Context().RegistryStr(),
		Path:   fmt.Sprintf(format, a...),
	}
}
