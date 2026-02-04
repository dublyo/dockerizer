# Makefile for Dublyo Dockerizer

# Variables
BINARY_NAME=dockerizer
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS=-ldflags "-s -w -X github.com/dublyo/dockerizer/internal/cli.Version=$(VERSION) -X github.com/dublyo/dockerizer/internal/cli.BuildTime=$(BUILD_TIME) -X github.com/dublyo/dockerizer/internal/cli.GitCommit=$(GIT_COMMIT)"

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Directories
CMD_DIR=./cmd/dockerizer
BUILD_DIR=./build
DIST_DIR=./dist

# OS/Arch for cross-compilation
PLATFORMS=linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

.PHONY: all build clean test coverage lint fmt vet install uninstall release help deps

# Default target
all: clean lint test build

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_DIR)
	@echo "Built: $(BUILD_DIR)/$(BINARY_NAME)"

# Build for all platforms
build-all:
	@echo "Building for all platforms..."
	@mkdir -p $(DIST_DIR)
	@for platform in $(PLATFORMS); do \
		GOOS=$${platform%/*} GOARCH=$${platform#*/} \
		$(GOBUILD) $(LDFLAGS) -o $(DIST_DIR)/$(BINARY_NAME)-$${platform%/*}-$${platform#*/}$(if $(findstring windows,$${platform%/*}),.exe,) $(CMD_DIR); \
		echo "Built: $(DIST_DIR)/$(BINARY_NAME)-$${platform%/*}-$${platform#*/}"; \
	done

# Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -rf $(BUILD_DIR) $(DIST_DIR)

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v -race ./...

# Run tests with coverage
coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -race -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Run linter
lint:
	@echo "Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed, skipping..."; \
	fi

# Format code
fmt:
	@echo "Formatting code..."
	$(GOCMD) fmt ./...

# Run go vet
vet:
	@echo "Running go vet..."
	$(GOCMD) vet ./...

# Get dependencies
deps:
	@echo "Getting dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

# Install locally
install: build
	@echo "Installing $(BINARY_NAME)..."
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(GOPATH)/bin/$(BINARY_NAME) 2>/dev/null || \
		cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)
	@echo "Installed to: $$(which $(BINARY_NAME) 2>/dev/null || echo '/usr/local/bin/$(BINARY_NAME)')"

# Uninstall
uninstall:
	@echo "Uninstalling $(BINARY_NAME)..."
	rm -f $(GOPATH)/bin/$(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)

# Quick development run
run: build
	$(BUILD_DIR)/$(BINARY_NAME) $(ARGS)

# Run detect on current directory
detect: build
	$(BUILD_DIR)/$(BINARY_NAME) detect .

# Generate release artifacts
release: clean build-all
	@echo "Creating release archives..."
	@mkdir -p $(DIST_DIR)/archives
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform#*/}; \
		binary=$(DIST_DIR)/$(BINARY_NAME)-$$os-$$arch; \
		if [ "$$os" = "windows" ]; then binary="$$binary.exe"; fi; \
		if [ -f "$$binary" ]; then \
			if [ "$$os" = "windows" ]; then \
				zip -j $(DIST_DIR)/archives/$(BINARY_NAME)-$$os-$$arch.zip $$binary; \
			else \
				tar -czvf $(DIST_DIR)/archives/$(BINARY_NAME)-$$os-$$arch.tar.gz -C $(DIST_DIR) $$(basename $$binary); \
			fi; \
		fi; \
	done
	@echo "Release artifacts in: $(DIST_DIR)/archives"

# Help
help:
	@echo "Dublyo Dockerizer - Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make              Build after cleaning, linting, and testing"
	@echo "  make build        Build the binary"
	@echo "  make build-all    Build for all platforms"
	@echo "  make clean        Remove build artifacts"
	@echo "  make test         Run tests"
	@echo "  make coverage     Run tests with coverage report"
	@echo "  make lint         Run golangci-lint"
	@echo "  make fmt          Format code"
	@echo "  make vet          Run go vet"
	@echo "  make deps         Download and tidy dependencies"
	@echo "  make install      Install binary locally"
	@echo "  make uninstall    Remove installed binary"
	@echo "  make run ARGS=.   Run with arguments"
	@echo "  make detect       Run detect on current directory"
	@echo "  make release      Create release archives"
	@echo "  make help         Show this help"
