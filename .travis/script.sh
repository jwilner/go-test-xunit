#!/usr/bin/env bash

set -e

function main {
    go test ./...

    mkdir -p target
    GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -o target/go-test-xunit-darwin-amd64
    GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o target/go-test-xunit-linux-amd64
}

main
