# zeroimage

`zeroimage some-program` is like building the following Docker image:

```dockerfile
FROM scratch
COPY some-program /some-program
ENTRYPOINT ["/some-program"]
```

…without actually using Docker.

Assuming that `some-program` is a statically linked executable, zeroimage
effectively produces the most minimal image that a container runtime could use
to launch it. In spite of the many caveats listed below, this can help drive
down startup times on serverless container platforms like AWS Lambda. Since
zeroimage simply writes a tar archive without ever talking to a container
runtime, it's great for cross-platform image builds.

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

zeroimage produces tar archive files compliant with the [OCI Image Format
Specification][oci]. You can use [Skopeo][skopeo] to work with these files:

```sh
# Write a container image archive to some-program.tar.
# You can pass the -output flag to write to a different path.
# You can also use -os and -arch to override the target platform of the image.
# Run "zeroimage -help" for usage.
zeroimage some-program

# Upload the image directly to a container registry.
skopeo copy oci-archive:some-program.tar docker://registry.example.com/some-program:latest

# Load the image into Docker (with a tag).
# Note that "docker load" does NOT support the OCI archive format!
# "skopeo copy" will convert the image to Docker's proprietary format.
skopeo copy oci-archive:some-program.tar docker-daemon:registry.example.com/some-program:latest
```

[oci]: https://github.com/opencontainers/image-spec
[skopeo]: https://github.com/containers/skopeo

## Future Work

- Support starting from a base image instead of nothing at all, to enable
  building more "proper" distroless containers.
