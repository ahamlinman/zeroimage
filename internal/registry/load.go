package registry

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/opencontainers/go-digest"
	specsv1 "github.com/opencontainers/image-spec/specs-go/v1"

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
		[]string{transport.PullScope},
	)
	if err != nil {
		return nil, err
	}

	return image.Load(ctx, loader{
		RoundTripper: transport,
		Name:         name,
	})
}

type loader struct {
	http.RoundTripper
	Name name.Reference
}

func (l loader) OpenRootManifest(ctx context.Context) (io.ReadCloser, error) {
	manifestURL := l.formatURL("/v2/%s/manifests/%s", l.Name.Context().RepositoryStr(), l.Name.Identifier())
	req, err := http.NewRequest(http.MethodGet, manifestURL.String(), nil)
	if err != nil {
		return nil, err
	}

	accept := []string{specsv1.MediaTypeImageIndex, specsv1.MediaTypeImageManifest}
	req.Header.Set("Accept", strings.Join(accept, ","))

	resp, err := l.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	if err := transport.CheckError(resp, http.StatusOK); err != nil {
		return nil, err
	}

	return resp.Body, nil
}

func (l loader) OpenBlob(ctx context.Context, dgst digest.Digest) (io.ReadCloser, error) {
	return nil, errors.New("not implemented")
}

func (l loader) formatURL(format string, a ...interface{}) url.URL {
	return url.URL{
		Scheme: l.Name.Context().Registry.Scheme(),
		Host:   l.Name.Context().RegistryStr(),
		Path:   fmt.Sprintf(format, a...),
	}
}
