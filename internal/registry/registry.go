// Package registry works with remote registries that implement the OCI
// Distribution Specification.
package registry

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
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

// CheckPushAuth validates that the current authentication configuration allows
// pushing blobs to a given repository. It returns a non-nil error if an upload
// could not be initiated for any reason.
func CheckPushAuth(ctx context.Context, reference string) error {
	name, err := name.ParseReference(reference)
	if err != nil {
		return err
	}

	tport, err := newTransport(ctx, name, transport.PushScope)
	if err != nil {
		return err
	}

	// This strategy comes from a similar method in go-containerregistry:
	// https://github.com/google/go-containerregistry/blob/v0.6.0/pkg/v1/remote/check.go
	//
	// Instead of *just* checking the /v2/ route, we try to initiate an actual
	// blob upload, since some registries might give us a token with push scope
	// even though we don't have push permissions. We're avoiding the version in
	// the /v1/remote package itself since it pulls in a lot of dependencies that
	// aren't relevant to us.

	client := http.Client{
		Transport: tport,
		Timeout:   httpTimeout,
	}
	uploadURL := url.URL{
		Scheme: name.Context().Scheme(),
		Host:   name.Context().RegistryStr(),
		Path:   fmt.Sprintf("/v2/%s/blobs/uploads/", name.Context().RepositoryStr()),
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL.String(), nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	err = transport.CheckError(resp, http.StatusAccepted)
	if err != nil {
		return err
	}

	// All of the following is a best-effort attempt to cancel the upload. This is
	// technically not part of the OCI distribution spec, but it is an explicit
	// part of the Docker registry API, so it's relevant for at least Docker Hub.
	location, err := uploadURL.Parse(resp.Header.Get("Location"))
	if err != nil {
		return nil
	}
	req, err = http.NewRequest(http.MethodDelete, location.String(), nil)
	if err != nil {
		return nil
	}
	resp, err = client.Do(req)
	if err != nil {
		return nil
	}
	resp.Body.Close()
	return nil
}
