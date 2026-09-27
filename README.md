# Cigno

Fast container image builder without Docker daemon, using OCI operations.

> **Positioning**: cigno is an *assembly builder*, not a general builder.
> It assembles images from prebuilt artifacts and component images
> (COPY / rebase / config edits). It intentionally does not execute `RUN` —
> see [docs/spec/20260926-assessment-and-direction.md](docs/spec/20260926-assessment-and-direction.md).

## Features

- **No Docker daemon required**: Builds images using OCI operations via `crane`
- **Fast**: No full blob pull for base images when rebase is possible
- **Artifact-friendly**: Designed for artifact image builds and CI pipelines
- **Dockerfile compatible**: Supports common Dockerfile instructions
- **Embedded registry**: Local registry v2 server for cache / push targets
- **Hermetic tests**: `go test ./...` needs no network, no docker

## Installation

```bash
go install github.com/feng-project/cigno@latest
```

## Quick Start

```bash
# Build an image from Dockerfile
cigno build -t myapp:latest -f Dockerfile

# Specify build context
cigno build -f Dockerfile -t myapp:latest -c /path/to/context

# Save to docker-loadable tar instead of pushing
cigno build -f Dockerfile -o output.tar -t myapp:latest

# Build args (override ARG defaults)
cigno build -f Dockerfile --build-arg VERSION=1.2.3 -t myapp:1.2.3

# Validate Dockerfile without building
cigno build -f Dockerfile --validate

# Disable the local base image disk cache
cigno build -f Dockerfile --no-cache -t myapp:latest
```

## Supported Dockerfile Instructions

| Instruction | Status | Notes |
|-------------|--------|-------|
| `FROM` | ✅ | Pulls base image with cache support; `scratch` supported |
| `COPY` | ✅ | Local files/dirs, wildcards (`.dockerignore` honored), `--from` image rebase / tarball / **stage name** (path-level), ARG expansion |
| `ADD` | ✅ | Like COPY + URL download + auto-extract |
| `ENV` | ✅ | With `$VAR` expansion |
| `ARG` | ✅ | Global and local scope; in-stage `ARG KEY` inherits global default |
| `WORKDIR` | ✅ | Config field |
| `USER` | ✅ | Config field |
| `CMD` | ✅ | Config field |
| `ENTRYPOINT` | ✅ | Config field |
| `LABEL` | ✅ | Metadata, ARG-expanded values |
| `EXPOSE` | ✅ | Config field |
| `VOLUME` | ✅ | Config field |
| `RUN` | ❌ | **By design** — see positioning note above |
| `REBASE` | ⏳ | Planned (annotation-based) |

## Local Registry

Start an embedded registry server for local caching / push targets
(plain HTTP works automatically for localhost refs):

```bash
# Start registry server (default: localhost:5000)
cigno registry start

# Custom address and storage
cigno registry start --addr localhost:6000 --storage /path/to/storage
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
cigno build -f Dockerfile -t myapp:latest  # First run: pulls from registry
cigno build -f Dockerfile -t myapp:latest  # Subsequent runs: uses cache

# Manage the cache
cigno cache list
cigno cache clear
```

## Testing

Tests are fully hermetic — they spin up the embedded registry in-process:

```bash
go test ./...
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
- [Design Roadmap (2026-01)](docs/spec/20260121-design-roadmap.md)
- [Value Assessment & Direction (2026-09)](docs/spec/20260926-assessment-and-direction.md)
- [Dockerfile Support](docs/dockerfile.md)
- [Rebase Guide](docs/rebase.md)

## Dependencies

- [go-containerregistry](https://github.com/google/go-containerregistry) - OCI operations
- [moby/buildkit](https://github.com/moby/buildkit) - Dockerfile parsing

The embedded registry server uses only the Go standard library.

## License

MPL 2.0
