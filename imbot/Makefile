# Makefile for imbot package

.PHONY: all build test clean fmt lint deps help

# Variables
BINARY_NAME=imbot
PACKAGE_PATH=./pkg
GO=go
GOFLAGS=-v
TESTFLAGS=-race -cover -coverprofile=coverage.out -covermode=atomic
VETFLAGS=...

# Default target
all: deps fmt lint test build

## build: Build the package
build:
	@echo "Building imbot package..."
	$(GO) build $(GOFLAGS) ./...

## test: Run tests
test:
	@echo "Running tests..."
	$(GO) test $(TESTFLAGS) ./...

## test-verbose: Run tests with verbose output
test-verbose:
	@echo "Running tests with verbose output..."
	$(GO) test -v $(TESTFLAGS) ./...

## test-coverage: Run tests and show coverage
test-coverage: test
	@echo "Coverage report:"
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

## clean: Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GO) clean
	rm -f coverage.out coverage.html
	rm -f $(BINARY_NAME)

## fmt: Format code
fmt:
	@echo "Formatting code..."
	$(GO) fmt ./...

## lint: Run linter
lint:
	@echo "Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Run: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

## vet: Run go vet
vet:
	@echo "Running go vet..."
	$(GO) vet $(VETFLAGS) ./...

## deps: Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GO) mod download
	$(GO) mod tidy

## examples: Build examples
examples:
	@echo "Building examples..."
	$(GO) build $(GOFLAGS) ./examples/basic/...
	$(GO) build $(GOFLAGS) ./examples/multi_platform/...

## install: Install the package
install:
	@echo "Installing imbot package..."
	$(GO) install $(GOFLAGS) ./...

## help: Show this help message
help:
	@echo "Available targets:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'

# Development helpers
run-example-telegram:
	@echo "Running Telegram bot example..."
	$(GO) run ./examples/basic/telegram-bot.go

run-example-multi:
	@echo "Running multi-platform example..."
	$(GO) run ./examples/multi_platform/main.go

# CI targets
ci: deps fmt vet test-coverage

.PHONY: all build test test-verbose test-coverage clean fmt lint vet deps examples install help run-example-telegram run-example-multi ci
