#!/bin/sh

# include simple ('source') configuration from example-env-config-simple.sh
# . ./example-env-config-simple.sh

# include complex ('source') configuration from example-env-config-complex.sh
# . ./example-env-config-complex.sh
. ./example-env-config-dev.sh


# run via local installed binary
# opnborg 

# run via local repository mode
go mod tidy
go run cmd/opnborg/main.go

# run via latest commit online
# go run paepcke.de/opnborg/cmd/opnborg@main

# run via latest release online
# go run paepcke.de/opnborg/cmd/opnborg@latest
