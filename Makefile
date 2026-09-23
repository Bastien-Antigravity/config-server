# =============================================================================
# Antigravity Ecosystem Makefile: config-server
# Build, test, and lifecycle automation for config-server Go daemon.
# =============================================================================

VERSION := $(shell cat VERSION.txt 2>/dev/null || echo "0.0.1")

.PHONY: all build test race vet version clean

all: build

version:
	@echo $(VERSION)

build:
	@echo "Building config-server (version $(VERSION))..."
	@mkdir -p bin
	go build -ldflags="-s -w -X 'github.com/Bastien-Antigravity/config-server/src/server.ServerVersion=$(VERSION)'" -o bin/config-server ./cmd/config-server

test:
	@echo "Running tests (version $(VERSION))..."
	go test -v ./...

race:
	@echo "Running race detector (version $(VERSION))..."
	go test -race ./...

vet:
	@echo "Running go vet..."
	go vet ./...

clean:
	@echo "Cleaning build artifacts..."
	@rm -rf bin/
