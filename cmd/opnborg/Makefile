all:
	go run main.go

check: 
	gofmt -w -s .
	CGO_ENABLED=0 go vet .
	CGO_ENABLED=0 staticcheck
