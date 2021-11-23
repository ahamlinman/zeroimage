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

## Usage

zeroimage works with remote registries that implement the [OCI Distribution
Specification][oci-distribution], or with `.tar` archives whose contents comply
with the [OCI Image Format Specification][oci-format]. Many popular container
tools and image registries support the OCI standards, with the notable
exceptions of Docker and Docker Hub (as of November 2021). Docker can only load
and save `.tar` archives in a proprietary format, and while Docker Hub supports
pushing and pulling OCI images it does not allow you to browse them properly in
the web UI. You can use [Skopeo][skopeo] to work with Docker and Docker Hub, as
shown in the examples below.

zeroimage uses your local Docker login credentials (the same credentials managed
by `docker login`) to authenticate with remote registries. You can also use
`zeroimage login` and `zeroimage logout` to manage these credentials. See the
CLI help for these commands for details.

**Example:** Publish an image with a [distroless][distroless] base layer, which
contains a basic Linux system layout but no shell or package manager:

```sh
# Build a new image using the base image and your entrypoint, and push it
# directly to a private registry. You can use "zeroimage login" to configure
# credentials for the private registry.
zeroimage build \
  --from gcr.io/distroless/static:latest \
  --push registry.example.com/some-program:latest \
  some-program
```

**Example:** Publish a `FROM scratch`-style image using a cross-compiled binary:

```sh
# Build and push an image with an ARM v8 entrypoint using the Busybox multi
# platform image from Docker Hub as a base. This works even on non-ARM hosts, as
# long as the entrypoint is properly cross-compiled. Note that zeroimage can
# only build images targeting a single platform.
zeroimage build \
  --from busybox:latest \
  --platform linux/arm64/v8 \
  --push registry.example.com/some-program-arm64:latest \
  some-program-arm64
```

**Example:** Extend a base image stored in a tar archive on disk:

```sh
# Download the base image into an OCI image archive with Skopeo.
skopeo copy docker://alpine:latest oci-archive:alpine.tar

# Build a new OCI image archive using the base image and your entrypoint. By
# default zeroimage will name the output file "some-program.tar", based on the
# entrypoint name.
zeroimage build --from-archive alpine.tar some-program

# Push the image to Docker Hub with Skopeo, converting OCI manifests to Docker
# v2 manifests so that Docker Hub can display the image correctly.
skopeo copy --format v2s2 oci-archive:some-program.tar docker://example/some-program:latest
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

[oci-distribution]: https://github.com/opencontainers/distribution-spec
[oci-format]: https://github.com/opencontainers/image-spec
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

- Instead of building `FROM scratch` by default, provide a built-in minimal base
  that removes some of the caveats noted above. For example, automatically
  bundle a standard `/etc/passwd` and a known set of TLS roots by default.
