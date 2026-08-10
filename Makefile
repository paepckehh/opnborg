PROJECT=$(shell basename $(CURDIR))

# Program version injected into the binary at build time via -ldflags. The
# value is the most recent git tag (e.g. v0.0.22) so the web UI navbar shows
# the released version. Falls back to "v0.0.0-dev" when no tag exists yet.
VERSION ?= $(shell git describe --tags --abbrev=0 2>/dev/null || echo v0.0.0-dev)
LDFLAGS := -X paepcke.de/$(PROJECT)/internal/version.Version=$(VERSION)

all: build

build:
	touch $(PROJECT) && rm $(PROJECT)
	go build -ldflags "$(LDFLAGS)" -o ./${PROJECT} ./cmd/$(PROJECT)

update: 
	git pull
	git pull --force --tags 

deploy-test-nix: update build 
	sudo -v
	sudo systemctl stop $(PROJECT)2.service
	sudo mv -f ./$(PROJECT) /nix/persist/root/bin 
	sudo systemctl start $(PROJECT)2.service

run: update build 
	OPN_TARGETS="testopn" \
	OPN_APIKEY="..." \
	OPN_APISECRET="..." \
	OPN_HTTPD_SERVER="0.0.0.0:8080" \
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
