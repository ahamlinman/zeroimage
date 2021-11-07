// Package registry works with remote registries that implement the OCI
// Distribution Specification.
package registry

import (
	"context"
	"net/http"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
)

const httpTimeout = 10 * time.Second

func newTransport(ctx context.Context, name name.Reference, scopes ...string) (http.RoundTripper, error) {
	authenticator, err := authn.DefaultKeychain.Resolve(name.Context())
	if err != nil {
		// TODO: Report that we hit this fallback?
		authenticator = authn.Anonymous
	}

	imgScopes := make([]string, len(scopes))
	for i, scope := range scopes {
		imgScopes[i] = name.Scope(scope)
	}

	return transport.NewWithContext(
		ctx,
		name.Context().Registry,
		authenticator,
		http.DefaultTransport,
		imgScopes,
	)
}
