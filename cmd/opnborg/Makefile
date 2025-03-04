all:
	go run main.go

check: 
	gofmt -w -s .
	go vet .
	staticcheck
	golangci-lint run
