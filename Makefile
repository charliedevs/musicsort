.PHONY: build build-musicsort build-spotifym3u install install-musicsort install-spotifym3u clean test fmt vet lint

VERSION ?= 1.0.0
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

# Default target
build: build-musicsort build-spotifym3u

build-musicsort:
	@echo "Building musicsort..."
	go build $(LDFLAGS) -o musicsort ./cmd/musicsort

build-spotifym3u:
	@echo "Building spotifym3u..."
	go build $(LDFLAGS) -o spotifym3u ./cmd/spotifym3u

install: install-musicsort install-spotifym3u

install-musicsort: build-musicsort
	@echo "Installing musicsort to $$HOME/.local/bin/"
	mkdir -p $(HOME)/.local/bin
	cp musicsort $(HOME)/.local/bin/
	@echo "musicsort installed successfully"

install-spotifym3u: build-spotifym3u
	@echo "Installing spotifym3u to $$HOME/.local/bin/"
	mkdir -p $(HOME)/.local/bin
	cp spotifym3u $(HOME)/.local/bin/
	@echo "spotifym3u installed successfully"

clean:
	@echo "Cleaning binaries..."
	rm -f musicsort spotifym3u
	go clean

test:
	@echo "Running tests..."
	go test -v -cover ./...

fmt:
	@echo "Formatting Go files..."
	gofmt -w ./cmd ./internal

vet:
	@echo "Running go vet..."
	go vet ./...

lint: fmt vet
	@echo "Running linter..."
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./... || echo "golangci-lint not installed, skipping"

.DEFAULT_GOAL := build
