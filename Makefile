PROJECT=$(shell basename $(CURDIR))

# Program version injected into the binary at build time via -ldflags. The
# value is the most recent git tag (e.g. v0.1.218) so the CLI banner and web
# UI footer show the released version. When the checkout has no tag (source
# tarball, shallow export) the -X flag is omitted entirely so the SemVer
# constant in api.go stays in effect; passing an empty value would blank it.
VERSION ?= $(shell git describe --tags --abbrev=0 2>/dev/null || echo '')
LDFLAGS := -s -w $(if $(VERSION),-X paepcke.de/$(PROJECT).SemVer=$(VERSION))

all: build

build:
	touch $(PROJECT) && rm $(PROJECT)
	go build -trimpath -ldflags "$(LDFLAGS)" -o ./${PROJECT} ./cmd/$(PROJECT)

update: 
	git pull
	git pull --force --tags 

deploy-test-nix: update build 
	sudo -v
	sudo systemctl stop $(PROJECT).service
	sudo mv -f ./$(PROJECT) /nix/persist/root/bin 
	sudo systemctl start $(PROJECT).service

run: update build 
	OPN_TARGETS="testopn" \
	OPN_APIKEY="..." \
	OPN_APISECRET="..." \
	OPN_HTTPD_SERVER="0.0.0.0:8080" \
	OPENAPI_DESC_URL="http://192.168.2.222:11434/v1" \
        OPENAPI_DESC_MODEL="gpt-oss-120b-MXFP4-Q8" \
        OPENAPI_DESC_TOKEN="FakeIT!!"\
	./$(PROJECT)

deps:
	rm -rf go.mod go.sum
	go mod init paepcke.de/$(PROJECT)
	go mod tidy -v
	git config core.fileMode false

check:
	gofmt -l .
	go vet ./...
	go mod tidy -diff

test:
	go test -race -count=1 ./...
