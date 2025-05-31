FROM golang:1.24 as app
RUN mkdir -p /opnborg
WORKDIR /opnborg
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-w -s" ./cmd/opnborg

FROM gcr.io/distroless/base
COPY --from=app /opnborg/opnborg /
ENTRYPOINT ["/opnborg"]
