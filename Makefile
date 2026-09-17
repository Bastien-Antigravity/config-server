VERSION := $(shell cat VERSION.txt 2>/dev/null || echo "0.0.1")

.PHONY: all build test version clean

all: build

version:
	@echo $(VERSION)

build:
	@echo "Building repository (version $(VERSION))..."
	@if [ -f "go.mod" ]; then \
		mkdir -p bin && \
		go build -o bin/config-server ./cmd/config-server && \
		go build ./...; \
	fi
	@if [ -f "Cargo.toml" ]; then cargo build --release; fi

test:
	@echo "Running tests (version $(VERSION))..."
	@if [ -f "go.mod" ]; then go test -v ./...; fi
	@if [ -f "Cargo.toml" ]; then cargo test; fi

clean:
	@echo "Cleaning build artifacts..."
	@rm -rf dist build *.egg-info target/ bin/
