package registry

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"

	"go.alexhamlin.co/zeroimage/internal/image"
)

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
	return errors.New("not implemented")
}
