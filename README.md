# debco

A declarative, BuildKit powered, Debian base system builder. 

Inspired by [apko](https://github.com/chainguard-dev/apko), [multistrap](https://wiki.debian.org/Multistrap), 
[debootstrap](https://wiki.debian.org/Debootstrap), and [cloud-init](https://cloudinit.readthedocs.io/en/latest/).

## Features

* Declarative - specify your base system in a YAML file.
* Reproducible - run the same command, get the same image.
* Secure - no need to trust a third party base image.
* Fast - uses [BuildKit](https://docs.docker.com/build/buildkit/) for caching and parallelism.
* Portable - build images on any platform that supports Docker.

## Installation

### From APT

Add my apt repository to your system:

*Currently packages are only published for Debian 12 (Bookworm).*

```shell
curl -fsL https://apt.pecke.tt/signing_key.asc | sudo tee /etc/apt/keyrings/apt-pecke-tt-keyring.asc > /dev/null
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/apt-pecke-tt-keyring.asc] http://apt.pecke.tt $(. /etc/os-release && echo $VERSION_CODENAME) stable" | sudo tee /etc/apt/sources.list.d/apt-pecke-tt.list > /dev/null
```

Then install debco:

```shell
sudo apt update
sudo apt install debco
```

### GitHub Releases

Download statically linked binaries from the GitHub releases page: 

[Latest Release](https://github.com/dpeckett/debco/releases/latest)

## Usage

### Prerequisites

* Docker

### Building a Image

To create a minimal Debian image:

```shell
debco build -f examples/bookworm-ultraslim.yaml
```

The resulting OCI archive will be saved to `debian-image.tar`.

### Running the Image

You will need a recent release of the [Skopeo](https://github.com/containers/skopeo) 
(eg. v1.15.1) to copy the image into your Docker daemon cache as Docker does not 
have native support for loading OCI images.

```shell
skopeo copy oci-archive:debian-image.tar docker-daemon:debco/debian:bookworm-ultraslim
```

You can then run the image with:

```shell
docker run --rm -it debco/debian:bookworm-ultraslim sh
```

### Using a Prebuilt Image

For convenience the debco build pipeline publishes a bookworm-ultraslim image.
This image is intended for experimentation purposes only. You should build your
own base images using the `debco build` command.

```shell
docker run --rm -it ghcr.io/dpeckett/debco/debian:bookworm-ultraslim sh
```

## Telemetry

By default debco gathers anonymous crash and usage statistics. This anonymized
data is processed on our servers within the EU and is not shared with third
parties. You can opt out of telemetry by setting the `DO_NOT_TRACK=1`
environment variable.

## Limitations

* [Debian Bookworm](https://www.debian.org/releases/bookworm/) and newer.