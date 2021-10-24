# zeroimage

zeroimage builds a container image similar to what the following Dockerfile
would produce, without using Docker at all:

```dockerfile
FROM scratch
COPY entrypoint /entrypoint
ENTRYPOINT ["/entrypoint"]
```

Assuming that `/entrypoint` is a statically linked executable, this effectively
produces the most minimal image that a container runtime could use to launch it.
In spite of the many caveats listed below, this can help drive down startup
times on serverless container platforms like AWS Lambda.

## Caveats

> Yeah, but your scientists were so preoccupied with whether or not they could,
> they didn't stop to think if they should.
>
> — Dr. Ian Malcolm, _Jurassic Park_

**Please be warned:** There are a _significant_ number of caveats associated
with this kind of approach, and if you are not careful about the fact that your
application is arguably running in a broken environment, things are probably not
going to go well.

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

## Usage

```sh
# Build the image
zeroimage -entrypoint ./my-program -output my-image.tar

# Upload it to a registry
skopeo copy oci-archive:my-image.tar docker://registry.example.com/my-image:latest
```

The output file (`my-image.tar` in the example) is an OCI image archive, which
you can load into a container runtime like [Podman][podman] or upload to a
registry using [Skopeo][skopeo].

[podman]: https://podman.io/
[skopeo]: https://github.com/containers/skopeo

## Future Work

- Docker is not happy when Skopeo tries to load one of these images into it.
  Figure out what Docker thinks is missing.
- Support starting from a base image instead of nothing at all, to enable
  building more "proper" distroless containers.
