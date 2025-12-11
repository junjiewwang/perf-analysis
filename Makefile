# Makefile for perf-analysis

# Version info
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u '+%Y-%m-%d_%H:%M:%S')

# Go parameters
GOCMD = go
GOBUILD = $(GOCMD) build
GOCLEAN = $(GOCMD) clean
GOTEST = $(GOCMD) test
GOGET = $(GOCMD) get
GOMOD = $(GOCMD) mod

# Binary names
CLI_BINARY = perf-analysis
ANALYZER_BINARY = perf-analyzer

# Build directory
BUILD_DIR = build

# ldflags for version injection - CLI
CLI_LDFLAGS = -ldflags "-X 'github.com/perf-analysis/cmd/cli/cmd.Version=$(VERSION)' \
						-X 'github.com/perf-analysis/cmd/cli/cmd.GitCommit=$(GIT_COMMIT)' \
						-X 'github.com/perf-analysis/cmd/cli/cmd.BuildTime=$(BUILD_TIME)'"

# ldflags for version injection - Analyzer
ANALYZER_LDFLAGS = -ldflags "-X 'main.Version=$(VERSION)' \
							 -X 'main.GitCommit=$(GIT_COMMIT)' \
							 -X 'main.BuildTime=$(BUILD_TIME)'"

# Main packages
CLI_PACKAGE = ./cmd/cli
ANALYZER_PACKAGE = ./cmd/analyzer

.PHONY: all build build-cli build-analyzer clean test deps help install dev release

# Default target - build all binaries
all: build

# Build all binaries with version info
build: build-cli build-analyzer

# Build CLI binary
build-cli:
	@echo "Building $(CLI_BINARY)..."
	@echo "  Version:    $(VERSION)"
	@echo "  Git Commit: $(GIT_COMMIT)"
	@echo "  Build Time: $(BUILD_TIME)"
	$(GOBUILD) $(CLI_LDFLAGS) -o ./$(CLI_BINARY) $(CLI_PACKAGE)
	@echo "Build complete: ./$(CLI_BINARY)"

# Build Analyzer binary
build-analyzer:
	@echo "Building $(ANALYZER_BINARY)..."
	@echo "  Version:    $(VERSION)"
	@echo "  Git Commit: $(GIT_COMMIT)"
	@echo "  Build Time: $(BUILD_TIME)"
	$(GOBUILD) $(ANALYZER_LDFLAGS) -o ./$(ANALYZER_BINARY) $(ANALYZER_PACKAGE)
	@echo "Build complete: ./$(ANALYZER_BINARY)"

# Development build (faster, no version injection)
dev:
	@echo "Building $(CLI_BINARY) (dev mode)..."
	$(GOBUILD) -o ./$(CLI_BINARY) $(CLI_PACKAGE)
	@echo "Building $(ANALYZER_BINARY) (dev mode)..."
	$(GOBUILD) -o ./$(ANALYZER_BINARY) $(ANALYZER_PACKAGE)

# Install to GOPATH/bin
install:
	@echo "Installing $(CLI_BINARY)..."
	$(GOBUILD) $(CLI_LDFLAGS) -o $(GOPATH)/bin/$(CLI_BINARY) $(CLI_PACKAGE)
	@echo "Installing $(ANALYZER_BINARY)..."
	$(GOBUILD) $(ANALYZER_LDFLAGS) -o $(GOPATH)/bin/$(ANALYZER_BINARY) $(ANALYZER_PACKAGE)

# Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -f ./$(CLI_BINARY) ./$(ANALYZER_BINARY)
	rm -rf $(BUILD_DIR)

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

# Build for multiple platforms
release: clean
	@echo "Building releases..."
	@mkdir -p $(BUILD_DIR)
	
	@echo "=== Building CLI ==="
	@echo "Building $(CLI_BINARY) for Linux amd64..."
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(CLI_LDFLAGS) -o $(BUILD_DIR)/$(CLI_BINARY)-linux-amd64 $(CLI_PACKAGE)
	
	@echo "Building $(CLI_BINARY) for Linux arm64..."
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(CLI_LDFLAGS) -o $(BUILD_DIR)/$(CLI_BINARY)-linux-arm64 $(CLI_PACKAGE)
	
	@echo "Building $(CLI_BINARY) for macOS amd64..."
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(CLI_LDFLAGS) -o $(BUILD_DIR)/$(CLI_BINARY)-darwin-amd64 $(CLI_PACKAGE)
	
	@echo "Building $(CLI_BINARY) for macOS arm64..."
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(CLI_LDFLAGS) -o $(BUILD_DIR)/$(CLI_BINARY)-darwin-arm64 $(CLI_PACKAGE)
	
	@echo "Building $(CLI_BINARY) for Windows amd64..."
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(CLI_LDFLAGS) -o $(BUILD_DIR)/$(CLI_BINARY)-windows-amd64.exe $(CLI_PACKAGE)
	
	@echo ""
	@echo "=== Building Analyzer ==="
	@echo "Building $(ANALYZER_BINARY) for Linux amd64..."
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(ANALYZER_LDFLAGS) -o $(BUILD_DIR)/$(ANALYZER_BINARY)-linux-amd64 $(ANALYZER_PACKAGE)
	
	@echo "Building $(ANALYZER_BINARY) for Linux arm64..."
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(ANALYZER_LDFLAGS) -o $(BUILD_DIR)/$(ANALYZER_BINARY)-linux-arm64 $(ANALYZER_PACKAGE)
	
	@echo "Building $(ANALYZER_BINARY) for macOS amd64..."
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(ANALYZER_LDFLAGS) -o $(BUILD_DIR)/$(ANALYZER_BINARY)-darwin-amd64 $(ANALYZER_PACKAGE)
	
	@echo "Building $(ANALYZER_BINARY) for macOS arm64..."
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(ANALYZER_LDFLAGS) -o $(BUILD_DIR)/$(ANALYZER_BINARY)-darwin-arm64 $(ANALYZER_PACKAGE)
	
	@echo "Building $(ANALYZER_BINARY) for Windows amd64..."
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(ANALYZER_LDFLAGS) -o $(BUILD_DIR)/$(ANALYZER_BINARY)-windows-amd64.exe $(ANALYZER_PACKAGE)
	
	@echo ""
	@echo "Release builds complete in $(BUILD_DIR)/"
	@ls -la $(BUILD_DIR)/

# Show help
help:
	@echo "Available targets:"
	@echo "  make build          - Build all binaries with version info (default)"
	@echo "  make build-cli      - Build CLI only"
	@echo "  make build-analyzer - Build Analyzer only"
	@echo "  make dev            - Fast build without version info"
	@echo "  make install        - Install to GOPATH/bin"
	@echo "  make clean          - Remove build artifacts"
	@echo "  make test           - Run tests"
	@echo "  make deps           - Download dependencies"
	@echo "  make release        - Build for multiple platforms"
	@echo "  make help           - Show this help"
	@echo ""
	@echo "Version variables (can be overridden):"
	@echo "  VERSION=$(VERSION)"
	@echo "  GIT_COMMIT=$(GIT_COMMIT)"
	@echo "  BUILD_TIME=$(BUILD_TIME)"
