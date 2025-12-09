.PHONY: all build test test-unit test-integration test-coverage test-race bench clean lint fmt help

# Build variables
BINARY_NAME=analyzer
BUILD_DIR=bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.GitCommit=$(GIT_COMMIT)"

# Go commands
GO=go
GOTEST=$(GO) test
GOBUILD=$(GO) build
GOCLEAN=$(GO) clean
GOMOD=$(GO) mod

# Default target
all: build

## Build the application
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/analyzer

## Build for multiple platforms
build-all: build-linux build-darwin

build-linux:
	@echo "Building for Linux..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/analyzer

build-darwin:
	@echo "Building for macOS..."
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/analyzer
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd/analyzer

## Run all tests
test:
	@echo "Running all tests..."
	$(GOTEST) -v ./...

## Run unit tests only (short mode)
test-unit:
	@echo "Running unit tests..."
	$(GOTEST) -short -v ./...

## Run integration tests
test-integration:
	@echo "Running integration tests..."
	$(GOTEST) -v -run Integration ./...

## Run tests with coverage report
test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -coverprofile=coverage.out -covermode=atomic ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"
	@$(GO) tool cover -func=coverage.out | grep total | awk '{print "Total coverage: " $$3}'

## Run tests with race detector
test-race:
	@echo "Running tests with race detector..."
	$(GOTEST) -race -v ./...

## Run benchmarks
bench:
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchmem ./...

## Update golden files
update-golden:
	@echo "Updating golden files..."
	UPDATE_GOLDEN=true $(GOTEST) ./... -run Golden

## Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

## Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download

## Tidy dependencies
tidy:
	@echo "Tidying dependencies..."
	$(GOMOD) tidy

## Verify dependencies
verify:
	@echo "Verifying dependencies..."
	$(GOMOD) verify

## Run linter
lint:
	@echo "Running linter..."
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

## Format code
fmt:
	@echo "Formatting code..."
	$(GO) fmt ./...

## Run vet
vet:
	@echo "Running go vet..."
	$(GO) vet ./...

## CI target - runs on CI/CD
ci: deps fmt vet lint test-race test-coverage
	@echo "CI checks complete"
	@$(GO) tool cover -func=coverage.out | grep total | awk '{print $$3}' | sed 's/%//' | \
		xargs -I {} sh -c 'if [ {} -lt 70 ]; then echo "Coverage {} is below 70%"; exit 1; fi'

## Generate mocks (requires mockery)
generate-mocks:
	@echo "Generating mocks..."
	@if command -v mockery > /dev/null; then \
		mockery --dir=internal/parser --name=Parser --output=internal/mock --outpkg=mock; \
		mockery --dir=internal/analyzer --name=Analyzer --output=internal/mock --outpkg=mock; \
	else \
		echo "mockery not installed. Install with: go install github.com/vektra/mockery/v2@latest"; \
	fi

## Run the application
run: build
	@echo "Running $(BINARY_NAME)..."
	./$(BUILD_DIR)/$(BINARY_NAME)

## Show help
help:
	@echo "Available targets:"
	@echo ""
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'
	@echo ""
	@echo "Usage: make [target]"
