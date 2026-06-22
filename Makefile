.PHONY: build run run-once install clean test test-coverage test-verbose test-one fmt lint help

# Binary names
BINARY_NAME=enphase-monitor

# Build the application
build:
	@echo "Building $(BINARY_NAME)..."
	go build -o $(BINARY_NAME)
	@echo "Build complete: ./$(BINARY_NAME)"

# Run in continuous monitoring mode
run: build
	@echo "Starting continuous monitoring..."
	./$(BINARY_NAME) --continuous

# Run single query and exit (default behavior)
run-once: build
	@echo "Running single query..."
	./$(BINARY_NAME)

# Install dependencies
install:
	@echo "Downloading dependencies..."
	go mod download
	@echo "Dependencies installed"

# Setup configuration
setup:
	@if [ ! -f config.yaml ]; then \
		echo "Creating config.yaml from template..."; \
		cp config.yaml.example config.yaml; \
		echo "Please edit config.yaml with your system IDs"; \
	else \
		echo "config.yaml already exists"; \
	fi
	@if [ ! -f credentials.yaml ]; then \
		echo "Creating credentials.yaml from template..."; \
		cp credentials.yaml.example credentials.yaml; \
		echo "Please edit credentials.yaml with your API key and OAuth credentials"; \
	else \
		echo "credentials.yaml already exists"; \
	fi

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f $(BINARY_NAME)
	rm -f coverage.out coverage.html
	go clean -testcache
	@echo "Clean complete"

# Run tests (validation tests require test data - see README.md Testing section)
test: build
	@echo "Running tests..."
	go test -v ./...

# Run tests with coverage report
test-coverage:
	@echo "Running tests with coverage..."
	go test -coverprofile=coverage.out ./...
	@echo "Coverage report generated: coverage.out"
	@echo "To view HTML report, run: go tool cover -html=coverage.out"

# Run tests in verbose mode without cache
test-verbose: build
	@echo "Running tests in verbose mode..."
	go test -v -count=1 ./...

# Run a specific test by name
test-one:
	@echo "Running specific test: $(TEST)"
	@if [ -z "$(TEST)" ]; then \
		echo "Usage: make test-one TEST=TestName"; \
		exit 1; \
	fi
	go test -v -run $(TEST) ./...

# Format code
fmt:
	go fmt ./...

# Run linters (uses go run; no global install required; first run downloads the tool)
lint:
	go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run ./...

# Generate PDFs from markdown files
pdfs:
	@echo "Generating PDFs from markdown files..."
	@./scripts/generate-pdfs.sh

# Show help
help:
	@echo "Available targets:"
	@echo "  make build     - Build the application"
	@echo "  make run       - Build and run in continuous mode (--continuous)"
	@echo "  make run-once  - Build and run single query (default behavior)"
	@echo "  make install   - Download Go dependencies"
	@echo "  make setup     - Create config.yaml and credentials.yaml from templates"
	@echo "  make clean     - Remove built binaries"
	@echo "  make test      - Run tests"
	@echo "  make fmt       - Format Go code"
	@echo "  make lint      - Run golangci-lint (via go run; first run may download the tool)"
	@echo "  make pdfs      - Generate PDF files from markdown documentation"
	@echo "  make help      - Show this help message"
