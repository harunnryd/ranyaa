.PHONY: test test-all test-memory build clean

# Build tags for SQLite FTS5 support
BUILD_TAGS := sqlite_fts5

# Test all packages with FTS5 support
test:
	go test -tags $(BUILD_TAGS) ./... -v

# Test all packages (short version)
test-all:
	go test -tags $(BUILD_TAGS) ./...

# Test memory package specifically
test-memory:
	go test -tags $(BUILD_TAGS) ./pkg/memory/... -v

# Build with FTS5 support
build:
	go build -tags $(BUILD_TAGS) -o bin/ranya ./cmd/ranya

# Clean build artifacts
clean:
	rm -rf bin/
	go clean

# Run tests with coverage
test-coverage:
	go test -tags $(BUILD_TAGS) ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

# Install dependencies
deps:
	go mod download
	go mod tidy

# Lint code
lint:
	golangci-lint run

# Format code
fmt:
	go fmt ./...
