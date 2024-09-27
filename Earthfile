VERSION 0.8
FROM golang:1.22-bookworm
ENV DO_NOT_TRACK=1
WORKDIR /workspace

all:
  COPY (+build/debco --GOARCH=amd64) ./dist/debco-linux-amd64
  COPY (+build/debco --GOARCH=arm64) ./dist/debco-linux-arm64
  COPY (+build/debco --GOARCH=riscv64) ./dist/debco-linux-riscv64
  COPY (+build/debco --GOOS=darwin --GOARCH=amd64) ./dist/debco-darwin-amd64
  COPY (+build/debco --GOOS=darwin --GOARCH=arm64) ./dist/debco-darwin-arm64
  COPY (+build/debco --GOOS=windows --GOARCH=amd64) ./dist/debco-windows-amd64.exe
  COPY (+package/*.deb --GOARCH=amd64) ./dist/
  COPY (+package/*.deb --GOARCH=arm64) ./dist/
  COPY (+package/*.deb --GOARCH=riscv64) ./dist/
  RUN cd dist && find . -type f | sort | xargs sha256sum >> ../sha256sums.txt
  SAVE ARTIFACT ./dist/* AS LOCAL dist/
  SAVE ARTIFACT ./sha256sums.txt AS LOCAL dist/sha256sums.txt

build:
  ARG GOOS=linux
  ARG GOARCH=amd64
  COPY go.mod go.sum ./
  RUN go mod download
  COPY . .
  ARG VERSION=dev
  RUN CGO_ENABLED=0 go build --ldflags "-s -X 'github.com/immutos/debco/internal/constants.Version=${VERSION}'" -o debco main.go
  SAVE ARTIFACT ./debco AS LOCAL dist/debco-${GOOS}-${GOARCH}

tidy:
  LOCALLY
  ENV GOTOOLCHAIN=go1.22.1
  RUN go mod tidy
  RUN go fmt ./...

lint:
  FROM golangci/golangci-lint:v1.59.1
  WORKDIR /workspace
  COPY . ./
  RUN golangci-lint run --timeout 5m ./...

test:
  FROM +tools
  ARG TARGETARCH
  COPY +build/debco ./dist/debco-linux-${TARGETARCH}
  COPY . ./
  WITH DOCKER
    RUN go test -coverprofile=coverage.out -v ./...
  END
  SAVE ARTIFACT ./coverage.out AS LOCAL coverage.out

package:
  FROM debian:bookworm
  # Use bookworm-backports for newer golang versions
  RUN echo "deb http://deb.debian.org/debian bookworm-backports main" > /etc/apt/sources.list.d/backports.list
  RUN apt update
  # Tooling
  RUN apt install -y git curl devscripts dpkg-dev debhelper-compat git-buildpackage libfaketime dh-sequence-golang \
    golang-any=2:1.22~3~bpo12+1 golang-go=2:1.22~3~bpo12+1 golang-src=2:1.22~3~bpo12+1 \
    gcc-aarch64-linux-gnu gcc-riscv64-linux-gnu
  # Add our apt repositories.
  RUN curl -fsL -o /etc/apt/keyrings/apt-dpeckett-dev-keyring.asc https://apt.dpeckett.dev/signing_key.asc \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/apt-dpeckett-dev-keyring.asc] http://apt.dpeckett.dev $(. /etc/os-release && echo $VERSION_CODENAME) stable" > /etc/apt/sources.list.d/apt-dpeckett-dev.list \
    && curl -fsL -o /etc/apt/keyrings/apt-immutos-com-keyring.asc https://apt.immutos.com/signing_key.asc \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/apt-immutos-com-keyring.asc] http://apt.immutos.com $(. /etc/os-release && echo $VERSION_CODENAME) stable" > /etc/apt/sources.list.d/apt-immutos-com.list \
    && apt update
  # Build Dependencies
  RUN apt install -y \
    golang-github-protonmail-go-crypto-dev \
    golang-github-adrg-xdg-dev \
    golang-github-containerd-containerd-dev \
    golang-github-docker-docker-dev \
    golang-github-docker-go-connections-dev \
    golang-github-dpeckett-archivefs-dev \
    golang-github-dpeckett-deb822-dev \
    golang-github-dpeckett-telemetry-dev \
    golang-github-dpeckett-uncompr-dev \
    golang-github-gofrs-flock-dev \
    golang-github-google-btree-dev \
    golang-github-gregjones-httpcache-dev \
    golang-github-grpc-ecosystem-grpc-opentracing-dev \
    golang-github-jaguilar-vt100-dev=0.0~git20240719.6f69db9-1 \
    golang-github-moby-patternmatcher-dev \
    golang-github-opencontainers-image-spec-dev=1.1.0-2~bpo12+1 \
    golang-github-otiai10-copy-dev \
    golang-github-pierrec-lz4-dev=4.1.18-1~bpo12+1 \
    golang-github-rogpeppe-go-internal-dev \
    golang-github-google-shlex-dev \
    golang-github-stretchr-testify-dev \
    golang-github-tonistiigi-fsutil-dev=0.0~git20240902.85aeae2-1 \
    golang-github-tonistiigi-units-dev \
    golang-github-urfave-cli-v2-dev \
    golang-github-vbauerster-mpb-dev=8.6.1-3~bpo12+1 \
    golang-golang-x-sync-dev \
    golang-golang-x-term-dev \
    golang-gopkg-yaml.v3-dev
  RUN mkdir -p /workspace/debco
  WORKDIR /workspace/debco
  COPY . .
  RUN if [ -n "$(git status --porcelain)" ]; then echo "Please commit your changes."; exit 1; fi
  RUN if [ -z "$(git describe --tags --exact-match 2>/dev/null)" ]; then echo "Current commit is not tagged."; exit 1; fi
  COPY debian/scripts/generate-changelog.sh /usr/local/bin/generate-changelog.sh
  RUN chmod +x /usr/local/bin/generate-changelog.sh
  ENV DEBEMAIL="damian@pecke.tt"
  ENV DEBFULLNAME="Damian Peckett"
  RUN /usr/local/bin/generate-changelog.sh
  RUN VERSION=$(git describe --tags --abbrev=0 | tr -d 'v') \
    && tar -czf ../debco_${VERSION}.orig.tar.gz --exclude-vcs .
  ARG GOARCH
  RUN dpkg-buildpackage -d -us -uc --host-arch=${GOARCH}
  SAVE ARTIFACT /workspace/*.deb AS LOCAL dist/

tools:
  RUN apt update
  RUN apt install -y ca-certificates curl jq libgpgme-dev libassuan-dev \
    libbtrfs-dev libdevmapper-dev libostree-dev libseccomp-dev pkg-config
  RUN curl -fsSL https://get.docker.com | bash