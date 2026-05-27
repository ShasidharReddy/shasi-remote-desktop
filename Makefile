.PHONY: all build build-macos build-windows build-linux test clean help

VERSION ?= 1.0.0
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME)"

all: clean test build

help:
	@echo "Shasi Remote Desktop - Build Commands"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  all           - Clean, test, and build all platforms"
	@echo "  build         - Build native binary (current OS)"
	@echo "  build-macos   - Build for macOS (ARM64 + x86_64)"
	@echo "  build-windows - Build for Windows (x86_64)"
	@echo "  build-linux   - Build for Linux (x86_64)"
	@echo "  test          - Run tests"
	@echo "  clean         - Remove build artifacts"
	@echo "  deps          - Download dependencies"

build:
	@echo "Building Shasi Remote Desktop..."
	@go build $(LDFLAGS) -o remote-desktop .
	@echo "✓ Built: ./remote-desktop"

build-macos:
	@echo "Building for macOS..."
	@mkdir -p bin
	@GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o bin/remote-desktop-darwin-arm64 .
	@GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o bin/remote-desktop-darwin-x86_64 .
	@echo "✓ Built: bin/remote-desktop-darwin-arm64 (Apple Silicon)"
	@echo "✓ Built: bin/remote-desktop-darwin-x86_64 (Intel)"

build-windows:
	@echo "Building for Windows..."
	@mkdir -p bin
	@GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o bin/remote-desktop-windows-amd64.exe .
	@echo "✓ Built: bin/remote-desktop-windows-amd64.exe"

build-linux:
	@echo "Building for Linux..."
	@mkdir -p bin
	@GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o bin/remote-desktop-linux-amd64 .
	@echo "✓ Built: bin/remote-desktop-linux-amd64"

test:
	@echo "Running tests..."
	@go test -v ./...
	@echo "✓ Tests passed"

deps:
	@echo "Downloading dependencies..."
	@go mod download
	@go mod tidy
	@echo "✓ Dependencies downloaded"

clean:
	@echo "Cleaning build artifacts..."
	@rm -f remote-desktop
	@rm -rf bin/
	@go clean
	@echo "✓ Cleaned"

install: build
	@echo "Installing..."
	@cp remote-desktop $(GOPATH)/bin/
	@echo "✓ Installed to $(GOPATH)/bin/"

.PHONY: run-server run-agent run-viewer
run-server:
	./remote-desktop -mode server -host 0.0.0.0 -port 8080

run-agent:
	./remote-desktop -mode agent -host localhost -port 8080 -agent-id "test-agent"

run-viewer:
	./remote-desktop -mode viewer -host localhost -port 8080 -agent-id "test-agent"
