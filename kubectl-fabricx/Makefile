# Makefile for kubectl-fabricx plugin

.PHONY: build clean test install

# Binary name
BINARY_NAME=kubectl-fabricx

# Build the plugin
build:
	go build -o $(BINARY_NAME) .

# Clean build artifacts
clean:
	rm -f $(BINARY_NAME)

# Run tests
test:
	go test ./...

# Install the plugin
install: build
	cp $(BINARY_NAME) ~/.local/bin/

# Build for different platforms
build-linux:
	GOOS=linux GOARCH=amd64 go build -o $(BINARY_NAME)-linux-amd64 .

build-darwin:
	GOOS=darwin GOARCH=amd64 go build -o $(BINARY_NAME)-darwin-amd64 .

build-darwin-arm64:
	GOOS=darwin GOARCH=arm64 go build -o $(BINARY_NAME)-darwin-arm64 .

# Build all platforms
build-all: build-linux build-darwin build-darwin-arm64

# Format code
fmt:
	go fmt ./...

# Lint code
lint:
	golangci-lint run

# Dependencies
deps:
	go mod tidy
	go mod download

# Help
help:
	@echo "Available targets:"
	@echo "  build          - Build the plugin"
	@echo "  clean          - Clean build artifacts"
	@echo "  test           - Run tests"
	@echo "  install        - Install the plugin"
	@echo "  build-linux    - Build for Linux"
	@echo "  build-darwin   - Build for macOS (Intel)"
	@echo "  build-darwin-arm64 - Build for macOS (Apple Silicon)"
	@echo "  build-all      - Build for all platforms"
	@echo "  fmt            - Format code"
	@echo "  lint           - Lint code"
	@echo "  deps           - Update dependencies" 