# Cigno

Fast container image builder without Docker daemon, using OCI operations.

## Features

- **No Docker daemon required**: Builds images using OCI operations via `crane`
- **Fast**: No full blob pull for base images when rebase is possible
- **Artifact-friendly**: Designed for artifact image builds and CI pipelines
- **Dockerfile compatible**: Supports common Dockerfile instructions

## Installation

```bash
go install github.com/feng-project/cigno@latest
```

## Quick Start

```bash
# Build an image from Dockerfile
cigno build -t myapp:latest

# Specify Dockerfile
cigno build -f Dockerfile -t myapp:latest

# Build context
cigno build -f Dockerfile -t myapp:latest -c /path/to/context

# Save to tar file instead of pushing
cigno build -f Dockerfile -o output.tar -t myapp:latest

# Validate Dockerfile without building
cigno build -f Dockerfile --validate
```

## Supported Dockerfile Instructions

| Instruction | Status | Notes |
|-------------|--------|-------|
| `FROM` | ✅ | Pulls base image with cache support |
| `COPY` | ✅ | Local files, `--from` (image rebase), `--from` (tarball) |
| `ADD` | ✅ | Like COPY + URL download + auto-extract |
| `ENV` | ✅ | With `$VAR` expansion |
| `ARG` | ✅ | Global and local scope |
| `WORKDIR` | ✅ | Config field |
| `USER` | ✅ | Config field |
| `CMD` | ✅ | Config field |
| `ENTRYPOINT` | ✅ | Config field |
| `LABEL` | ✅ | Metadata |
| `EXPOSE` | ✅ | Config field |
| `VOLUME` | ✅ | Config field |
| `RUN` | ⏳ | Planned (via cigno-cli wrapper) |
| `REBASE` | ⏳ | Planned (annotation-based) |

## Local Registry

Start an embedded registry server for local caching:

```bash
# Start registry server (default: localhost:5000)
cigno registry start

# Custom address and storage
cigno registry start --addr localhost:6000 --storage /path/to/storage

# Check status
cigno registry status
```

## Advanced Usage

### Image Rebase with `COPY --from`

Copy layers from another image, supporting component assembly:

```dockerfile
FROM alpine:latest AS base
RUN apk add --no-cache ca-certificates

FROM scratch AS app
COPY --from=base / /
COPY app /usr/local/bin/app
```

### Tarball as Source

```dockerfile
FROM alpine:latest
COPY --from=tarball://./artifacts.tar.gz / /usr/local/
```

### Build Context Options

```bash
# Add base image mapping
cigno build --image-base mybase=ubuntu:22.04

# Add tarball source
cigno build --tarball artifacts=./artifacts.tar

# Add folder source
cigno build --folder data=/path/to/data
```

## Caching

Cigno uses local disk cache by default (`~/.cigno/cache/`):

```bash
# Images are cached automatically
cigno build -t myapp:latest  # First run: pulls from registry
cigno build -t myapp:latest  # Subsequent runs: uses cache
```

## Architecture

```
cigno/
├── cmd/              # CLI commands
│   ├── root.go       # Main entry point
│   ├── build.go      # Build command
│   └── registry.go   # Registry server command
├── pkg/
│   ├── build.go      # Main build logic
│   ├── engine.go     # Build engine
│   ├── config.go     # Config command handlers
│   ├── copy.go       # COPY/ADD implementation
│   ├── env.go        # ENV handling
│   ├── arg.go        # ARG handling
│   ├── tarball.go    # Tarball operations
│   ├── cache/        # Local cache
│   ├── registry/     # Embedded registry
│   └── validate.go   # Validation layer
└── docs/             # Documentation
```

## Design Philosophy

> "Build image quickly, without docker daemon, no full blob pull, for artifact and CI scenarios"

### Key Constraints

- **No Docker daemon**: Uses crane for OCI operations
- **No full blob pull**: Uses registry API + layer diffing
- **CI-friendly**: Stateless, containerizable
- **Artifact-focused**: Prioritizes COPY over RUN

## Documentation

- [Architecture](docs/arch/20260121-arch.md)
- [Design Roadmap](docs/spec/20260121-design-roadmap.md)
- [Dockerfile Support](docs/dockerfile.md)
- [Rebase Guide](docs/rebase.md)

## Dependencies

- [go-containerregistry](https://github.com/google/go-containerregistry) - OCI operations
- [moby/buildkit](https://github.com/moby/buildkit) - Dockerfile parsing
- [gorilla/mux](https://github.com/gorilla/mux) - HTTP routing for registry

## License

MIT
