# Build Instructions

## Prerequisites

This project uses SQLite with FTS5 (Full-Text Search) support. The Go SQLite driver (`mattn/go-sqlite3`) requires CGO and specific build tags to enable FTS5.

### Required Tools

1. **Go 1.21 or later**
   ```bash
   go version
   ```

2. **C Compiler** (required for CGO)
   - **macOS**: Xcode Command Line Tools
     ```bash
     xcode-select --install
     ```
   - **Linux**: GCC and build tools
     ```bash
     # Debian/Ubuntu
     sudo apt-get install build-essential
     
     # RHEL/CentOS/Fedora
     sudo yum groupinstall "Development Tools"
     ```
   - **Windows**: MinGW-w64 or TDM-GCC
     - Download from: https://www.mingw-w64.org/

## Build Tags

The following build tag is **REQUIRED** for this project:

- `sqlite_fts5` - Enables SQLite FTS5 full-text search engine

Without this tag, tests will fail with error: `"no such module: fts5"`

## Building the Project

### Using Make (Recommended)

The project includes a Makefile with all necessary build tags configured:

```bash
# Build the binary
make build

# Output: bin/ranya
```

### Using Go Commands Directly

If you prefer to use Go commands directly, you **MUST** include the build tag:

```bash
# Build
go build -tags sqlite_fts5 -o bin/ranya ./cmd/ranya

# Build with optimizations
go build -tags sqlite_fts5 -ldflags="-s -w" -o bin/ranya ./cmd/ranya
```

## Running Tests

### Using Make (Recommended)

```bash
# Run all tests
make test

# Run tests for specific package
make test-memory

# Run tests with coverage report
make test-coverage
```

### Using Go Commands Directly

**IMPORTANT**: Always include the `sqlite_fts5` build tag when running tests:

```bash
# Run all tests
go test -tags sqlite_fts5 ./...

# Run tests with verbose output
go test -tags sqlite_fts5 ./... -v

# Run specific package tests
go test -tags sqlite_fts5 ./pkg/memory/... -v

# Run specific test
go test -tags sqlite_fts5 ./pkg/memory/... -run TestMemorySearch -v

# Run tests with coverage
go test -tags sqlite_fts5 ./... -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
```

## IDE Configuration

### Visual Studio Code

The project includes `.vscode/settings.json` with the correct configuration:

```json
{
  "go.buildTags": "sqlite_fts5",
  "go.testTags": "sqlite_fts5",
  "go.toolsEnvVars": {
    "CGO_ENABLED": "1"
  }
}
```

This ensures that:
- IntelliSense works correctly
- Tests can be run from the IDE
- Build commands use the correct tags

### GoLand / IntelliJ IDEA

1. Open **Settings** (Cmd+, on macOS, Ctrl+Alt+S on Windows/Linux)
2. Navigate to **Go** → **Build Tags & Vendoring**
3. In the **Custom tags** field, add: `sqlite_fts5`
4. Click **OK**

### Vim / Neovim with vim-go

Add to your `.vimrc` or `init.vim`:

```vim
let g:go_build_tags = 'sqlite_fts5'
```

## CI/CD Configuration

### GitHub Actions

```yaml
name: Test

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      
      - name: Install dependencies
        run: |
          sudo apt-get update
          sudo apt-get install -y build-essential
      
      - name: Run tests
        run: go test -tags sqlite_fts5 ./...
```

### GitLab CI

```yaml
test:
  image: golang:1.21
  before_script:
    - apt-get update && apt-get install -y build-essential
  script:
    - go test -tags sqlite_fts5 ./...
```

## Troubleshooting

### Error: "no such module: fts5"

**Cause**: Tests or builds are running without the `sqlite_fts5` build tag.

**Solution**: Always use `-tags sqlite_fts5` when building or testing:

```bash
# Wrong ❌
go test ./pkg/memory/...

# Correct ✅
go test -tags sqlite_fts5 ./pkg/memory/...
```

Or use the Makefile:

```bash
make test
```

### Error: "C compiler not found"

**Cause**: CGO requires a C compiler, but none is installed.

**Solution**: Install a C compiler for your platform:

- **macOS**: `xcode-select --install`
- **Linux**: `sudo apt-get install build-essential`
- **Windows**: Install MinGW-w64

### Error: "undefined: sqlite3_fts5_*"

**Cause**: The SQLite library doesn't have FTS5 compiled in.

**Solution**: The `sqlite_fts5` build tag should handle this automatically. If the error persists:

1. Clean the build cache:
   ```bash
   go clean -cache
   ```

2. Rebuild:
   ```bash
   make build
   ```

### Tests Pass Locally but Fail in CI

**Cause**: CI environment might not have the build tags configured.

**Solution**: Ensure your CI configuration includes `-tags sqlite_fts5` in all test commands.

### Slow Build Times

**Cause**: CGO compilation can be slow, especially for SQLite.

**Solution**: 
1. Use build cache (enabled by default in Go 1.11+)
2. Consider using `-ldflags="-s -w"` to reduce binary size
3. For development, use `go build` without optimizations

## Cross-Compilation

When cross-compiling, you need a C cross-compiler for the target platform:

### Linux to Windows

```bash
# Install MinGW cross-compiler
sudo apt-get install gcc-mingw-w64

# Build for Windows
CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc \
  go build -tags sqlite_fts5 -o bin/ranya.exe ./cmd/ranya
```

### macOS to Linux

```bash
# Install cross-compiler (using Homebrew)
brew install FiloSottile/musl-cross/musl-cross

# Build for Linux
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 CC=x86_64-linux-musl-gcc \
  go build -tags sqlite_fts5 -o bin/ranya-linux ./cmd/ranya
```

## Docker Build

If you're building in Docker, ensure the base image has a C compiler:

```dockerfile
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache gcc musl-dev sqlite-dev

WORKDIR /app
COPY . .

# Build with FTS5 support
RUN go build -tags sqlite_fts5 -o /ranya ./cmd/ranya

FROM alpine:latest
RUN apk add --no-cache ca-certificates sqlite-libs
COPY --from=builder /ranya /usr/local/bin/ranya

ENTRYPOINT ["/usr/local/bin/ranya"]
```

## Performance Considerations

### Build Performance

- **First build**: ~30-60 seconds (compiles SQLite with FTS5)
- **Incremental builds**: ~5-10 seconds (uses cached objects)
- **Clean builds**: Use `go clean -cache` to clear cache

### Runtime Performance

FTS5 provides excellent full-text search performance:
- Indexing: ~1000-5000 documents/second
- Search: <100ms for most queries
- Memory usage: ~10-50MB for typical workloads

## References

- [mattn/go-sqlite3 Documentation](https://github.com/mattn/go-sqlite3)
- [SQLite FTS5 Extension](https://www.sqlite.org/fts5.html)
- [Go Build Tags](https://pkg.go.dev/cmd/go#hdr-Build_constraints)
- [CGO Documentation](https://pkg.go.dev/cmd/cgo)

## Quick Reference

```bash
# Build
make build

# Test
make test

# Test specific package
go test -tags sqlite_fts5 ./pkg/memory/... -v

# Clean
make clean

# Coverage
make test-coverage
```

**Remember**: Always use `-tags sqlite_fts5` when building or testing! 🔑
