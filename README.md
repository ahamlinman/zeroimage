# zeroimage

```sh
zeroimage build --from gcr.io/distroless/static:latest some-program
```

…is like building the following Dockerfile:

```dockerfile
FROM gcr.io/distroless/static:latest
COPY some-program /some-program
ENTRYPOINT ["/some-program"]
```

…without using Docker at all.

zeroimage is a lightweight container image builder for single-binary programs.
It can produce single-layer `FROM scratch`-style images, or extend an existing
base image to include your program, without ever touching a full container
runtime.

### Why zeroimage?

- It doesn't depend on a container runtime, which can simplify image builds for
  cross-compiled programs.
- It's unopinionated about the rest of your toolchain, and supports
  single-binary entrypoints compiled from any language.
- In spite of their caveats (see below), it supports `FROM scratch`-style images
  that produce the smallest possible output, which can be helpful on serverless
  container platforms like AWS Lambda.

[imports]: https://pkg.go.dev/go.alexhamlin.co/zeroimage?tab=imports

## Usage

zeroimage currently only produces `.tar` archives whose contents comply with the
[OCI Image Format Specification][oci]. Not all container tools support OCI image
archives! Docker in particular uses a proprietary `.tar` layout that is not
OCI-compatible. You will probably use zeroimage alongside a tool like
[Skopeo][skopeo] to push and pull OCI image archives to and from registries, or
to load images into a container runtime.

zeroimage can consume base images from OCI image archives, or from a remote
registry. zeroimage will automatically authenticate to private registries using
your Docker login credentials if necessary (the same credentials managed by
`docker login`).

**Example:** Publish an image with a [distroless][distroless] base layer, which
contains a basic Linux system layout but no shell or package manager:

```sh
# Build a new OCI image archive using the base image and your entrypoint. By
# default zeroimage will name the output file "some-program.tar", based on the
# entrypoint name.
zeroimage --from gcr.io/distroless/static:latest some-program

# Publish the new image to your own registry.
skopeo copy oci-archive:some-program.tar docker://registry.example.com/some-program:latest
```

**Example:** Extend a base image stored in a tar archive on disk:

```sh
# Download the base image into an OCI image archive with Skopeo.
skopeo copy docker://alpine:latest oci-archive:alpine.tar

# Build a new OCI image archive using the base image and your entrypoint. By
# default zeroimage will name the output file "some-program.tar", based on the
# entrypoint name.
zeroimage build --from-archive distroless-base.tar some-program
```

**Example:** Build a `FROM scratch`-style image and load it into Docker:

```sh
# Without a base image, zeroimage will produce a "FROM scratch"-style image that
# literally just contains the entrypoint binary.
zeroimage build some-program

# Since "docker load" does not support OCI image archives, use Skopeo to load
# the image into a Docker daemon for testing.
skopeo copy oci-archive:some-program.tar docker-daemon:registry.example.com/some-program:latest
```

[oci]: https://github.com/opencontainers/image-spec
[skopeo]: https://github.com/containers/skopeo
[distroless]: https://github.com/GoogleContainerTools/distroless

## Caveats of `FROM scratch`-Style Images

While zeroimage supports `FROM scratch`-style images with no base layer at all,
there are serious caveats associated with this approach for which you may need
to specially prepare your application.

Most notably, the entrypoint binary must be _completely_ statically linked. Even
languages that are capable of producing such binaries do not usually do this by
default. For example, you might need to set `CGO_ENABLED=0` in your environment
while building a Go binary, or switch to a musl-based target while building a
Rust binary.

Other notable caveats include, but are not limited to:

- There are no user or group databases (`/etc/passwd` or `/etc/group`) in the
  image.
- There is no timezone database in the image. (In Go 1.15+, you can work around
  this with the `timetzdata` build tag.)
- There are no TLS root certificates in the image.

## Future Work

- Include built in support for pushing to OCI registries, to reduce the heavy
  dependency on external tools like Skopeo.
- Instead of building `FROM scratch` by default, provide a built-in minimal base
  that removes some of the caveats noted above. For example, automatically
  bundle a standard `/etc/passwd` and a known set of TLS roots by default.
