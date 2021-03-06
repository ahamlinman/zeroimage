package registry

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/opencontainers/go-digest"

	"go.alexhamlin.co/zeroimage/internal/image"
)

// Load loads an image index identified by a Docker-style reference from a
// remote OCI registry, using credentials from the local Docker keychain to
// authenticate to the registry if necessary.
func Load(ctx context.Context, reference string) (image.Index, error) {
	name, err := name.ParseReference(reference)
	if err != nil {
		return nil, err
	}

	transport, err := newTransport(ctx, name, transport.PullScope)
	if err != nil {
		return nil, err
	}

	return image.Load(ctx, loader{
		Name: name,
		Client: http.Client{
			Transport: transport,
			Timeout:   httpTimeout,
		},
	})
}

type loader struct {
	Name   name.Reference
	Client http.Client
}

func (l loader) RootDigest() (dgst digest.Digest, ok bool) {
	if ndgst, ok := l.Name.(name.Digest); ok {
		dgst, err := digest.Parse(ndgst.DigestStr())
		if err != nil {
			panic(fmt.Errorf("invalid digest in remote image reference: %v", err))
		}
		return dgst, true
	}
	return digest.FromString(""), false
}

func (l loader) OpenRootManifest(ctx context.Context) (io.ReadCloser, error) {
	req := l.newGetRequest(ctx, "manifests", l.Name.Identifier())
	req.Header.Set("Accept", strings.Join(acceptedManifestTypes, ","))
	return l.doRequest(req)
}

func (l loader) OpenManifest(ctx context.Context, dgst digest.Digest) (io.ReadCloser, error) {
	req := l.newGetRequest(ctx, "manifests", dgst.String())
	req.Header.Set("Accept", strings.Join(acceptedManifestTypes, ","))
	return l.doRequest(req)
}

func (l loader) OpenBlob(ctx context.Context, dgst digest.Digest) (io.ReadCloser, error) {
	return l.doRequest(l.newGetRequest(ctx, "blobs", dgst.String()))
}

var acceptedManifestTypes []string

func init() {
	acceptedManifestTypes = append(acceptedManifestTypes, image.SupportedIndexMediaTypes...)
	acceptedManifestTypes = append(acceptedManifestTypes, image.SupportedManifestMediaTypes...)
}

func (l loader) newGetRequest(ctx context.Context, kind, identifer string) *http.Request {
	url := l.formatURL(kind, identifer)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url.String(), nil)
	if err != nil {
		panic(err)
	}
	return req
}

func (l loader) formatURL(kind, identifier string) url.URL {
	return url.URL{
		Scheme: l.Name.Context().Registry.Scheme(),
		Host:   l.Name.Context().RegistryStr(),
		Path:   fmt.Sprintf("/v2/%s/%s/%s", l.Name.Context().RepositoryStr(), kind, identifier),
	}
}

func (l loader) doRequest(req *http.Request) (io.ReadCloser, error) {
	resp, err := l.Client.Do(req)
	if err != nil {
		return nil, err
	}
	return resp.Body, transport.CheckError(resp, http.StatusOK)
}
