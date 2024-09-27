#!/bin/sh
# include ('source') configuration from example-env-config.sh
. ./example-env-config.sh

# local repository mode
go mod tidy
go run cmd/opnborg/main.go

# if not in local repository, use:
# go run paepcke.de/opnborg/cmd/opnborg@latest
