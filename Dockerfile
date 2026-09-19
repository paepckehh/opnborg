FROM golang:1.26 AS app
ENV GOTOOLCHAIN=auto
WORKDIR /opnborg
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 go build -trimpath -ldflags="-w -s" -o /bin/opnborg ./cmd/opnborg

FROM gcr.io/distroless/base
COPY --from=app /bin/opnborg /opnborg
ENTRYPOINT ["/opnborg"]
