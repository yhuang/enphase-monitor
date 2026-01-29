.PHONY: build run run-once install clean help

# Binary name
BINARY_NAME=enphase-monitor

# Build the application
build:
	@echo "Building $(BINARY_NAME)..."
	go build -o $(BINARY_NAME)
	@echo "Build complete: ./$(BINARY_NAME)"

# Run in continuous monitoring mode
run: build
	@echo "Starting continuous monitoring..."
	./$(BINARY_NAME)

# Run once and exit
run-once: build
	@echo "Running single query..."
	./$(BINARY_NAME) --once

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
		echo "Please edit config.yaml with your API token and system IDs"; \
	else \
		echo "config.yaml already exists"; \
	fi

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f $(BINARY_NAME)
	go clean
	@echo "Clean complete"

# Run tests (validation tests require test data - see README.md Testing section)
test:
	@echo "Note: Unit tests not yet implemented."
	@echo "For validation tests, use: ./enphase-monitor --test --date YYYY-MM-DD"
	@echo "Or run: ./run-tests.sh"

# Format code
fmt:
	go fmt ./...

# Generate PDFs from markdown files
pdfs:
	@echo "Generating PDFs from markdown files..."
	@./generate-pdfs.sh

# Show help
help:
	@echo "Available targets:"
	@echo "  make build     - Build the application"
	@echo "  make run       - Build and run in continuous mode"
	@echo "  make run-once  - Build and run single query"
	@echo "  make install   - Download Go dependencies"
	@echo "  make setup     - Create config.yaml from template"
	@echo "  make clean     - Remove built binaries"
	@echo "  make test      - Run tests"
	@echo "  make fmt       - Format Go code"
	@echo "  make pdfs      - Generate PDF files from markdown documentation"
	@echo "  make help      - Show this help message"
