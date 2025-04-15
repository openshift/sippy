#!/bin/bash
set -ex

go version
go tool golangci-lint version -v
go tool golangci-lint --timeout 10m "${@}"
