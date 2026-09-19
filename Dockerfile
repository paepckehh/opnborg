# syntax=docker/dockerfile:1

# Build stage. The base image tracks the current Go 1.26 patch line; if the
# image toolchain is ever older than go.mod requires, GOTOOLCHAIN=auto makes
# the Go command download the matching toolchain so the build never breaks.
ARG GO_VERSION=1.26
FROM golang:${GO_VERSION} AS build

# VERSION is injected via -ldflags so the shipped binary reports its release
# tag (CLI banner, WebUI footer, HTTP User-Agent). The ghcr.io workflow passes
# the git tag here; local builds fall back to the SemVer constant in api.go.
ARG VERSION
WORKDIR /opnborg
ENV CGO_ENABLED=0 GOTOOLCHAIN=auto

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    set -eu; \
    versionFlag=""; \
    if [ -n "$VERSION" ]; then \
      versionFlag="-X paepcke.de/opnborg.SemVer=$VERSION"; \
    fi; \
    go build -trimpath \
      -ldflags="-w -s $versionFlag" \
      -o /out/opnborg ./cmd/opnborg

# Runtime stage. The binary is fully static (CGO_ENABLED=0), so the minimal
# distroless static image (no shell, no package manager, but with
# ca-certificates and tzdata) is sufficient and keeps the image tiny.
FROM gcr.io/distroless/static-debian12
ARG VERSION

LABEL org.opencontainers.image.title="opnborg" \
      org.opencontainers.image.description="OPNsense firewall configuration backup, monitoring and sync daemon" \
      org.opencontainers.image.source="https://github.com/paepckehh/opnborg" \
      org.opencontainers.image.url="https://paepcke.de/opnborg" \
      org.opencontainers.image.licenses="BSD-3-Clause" \
      org.opencontainers.image.version="${VERSION}"

COPY --from=build /out/opnborg /usr/bin/opnborg

# Container defaults: store backups under /var/opnborg (mount it as a volume)
# and bind the WebUI to all interfaces so it is reachable through published
# ports; override either with -e. Never expose 6464 to untrusted networks.
ENV OPN_PATH=/var/opnborg \
    OPN_HTTPD_SERVER=0.0.0.0:6464

WORKDIR /var/opnborg
VOLUME ["/var/opnborg"]
EXPOSE 6464

ENTRYPOINT ["/usr/bin/opnborg"]
