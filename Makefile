# Local builds. The release workflow uses the same flags so a binary built here
# matches what CI produces for a tag.

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.Version=$(VERSION)

.PHONY: build test run dist clean fmt vet check

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o phantom-manager .

# Runs the manager against the working tree, opening the Windows browser.
run:
	go run -ldflags "$(LDFLAGS)" .

test:
	go test -race ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

check: fmt vet test

# The release artifact: one static binary plus its checksum.
dist:
	mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
		go build -trimpath -ldflags "$(LDFLAGS)" -o dist/phantom-manager .
	cd dist && sha256sum phantom-manager > phantom-manager.sha256

clean:
	rm -rf dist phantom-manager
