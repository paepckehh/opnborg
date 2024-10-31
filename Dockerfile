FROM golang:buster as app
RUN mkdir -p /opnborg
WORKDIR /opnborg
COPY . .
RUN go build ./cmd/opnborg

FROM gcr.io/distroless/base
COPY --from=app /opnborg/opnborg /
ENTRYPOINT ["/opnborg"]
