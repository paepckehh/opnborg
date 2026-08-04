FROM golang:1.26 AS app
WORKDIR /opnborg
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 go build -ldflags="-w -s" ./cmd/opnborg

FROM gcr.io/distroless/base
COPY --from=app /opnborg/opnborg /opnborg
ENTRYPOINT ["/opnborg"]
