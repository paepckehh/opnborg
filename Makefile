PROJECT=$(shell basename $(CURDIR))

all:
	make -C cmd/$(PROJECT) all

deps:
	@echo "reminder: bump api.com hardcoded version before release!"
	@echo "########################################################"
	rm go.mod go.sum
	go mod init paepcke.de/$(PROJECT)
	go mod tidy -v	

check: 
	gofmt -w -s .
	CGO_ENABLED=0 go vet .
	CGO_ENABLED=0 staticcheck
	make -C cmd/$(PROJECT) check
