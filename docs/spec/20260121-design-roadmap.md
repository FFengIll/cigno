# Cigno Design Spec: Current State and Future Roadmap

**Date**: 2026-01-21
**Status**: Draft

## 1. Current State Analysis

### 1.1 Implemented Features

| Feature | File | Status |
|---------|------|--------|
| Dockerfile parsing | `pkg/dockerfile.go` | ✅ |
| FROM (base image pull) | `pkg/build.go:62-71` | ✅ |
| COPY (local files) | `pkg/copy.go:20-40` | ✅ |
| COPY --from (image rebase) | `pkg/copy.go:42-122` | ✅ |
| COPY --from (tarball) | `pkg/copy.go:96-119` | ✅ |
| ENV (with $VAR expansion) | `pkg/env.go:13-34` | ✅ |
| ARG (global + local) | `pkg/arg.go:9-29` | ✅ |
| Tarball creation/editing | `pkg/tarball.go` | ✅ |
| Push to registry | `pkg/registry.go:11-27` | ✅ |
| Save to tar file | `pkg/option.go:28-36` | ✅ |
| Layer diffing (rebase) | `pkg/copy.go:124-182` | ✅ |
| OCI annotations | `pkg/build.go:158-164` | ✅ |

### 1.2 Skeleton/Placeholder Features

| Feature | File | Notes |
|---------|------|-------|
| `run.go` | Empty file | RUN command placeholder |
| `record.go` | Stub only | Record management (no impl) |
| `registry/v2/` | Empty dirs | Local registry server |

### 1.3 Known TODOs/FIXMEs

```go
// pkg/arg.go:14
// TODO: add a layer for arg command into image

// pkg/copy.go:27
// FIXME: for now, we do not extract special files

// pkg/copy.go:48, 129
// TODO: validate first

// pkg/build.go:61
// TODO: validate base and tag here

// pkg/build.go:122
// FIXME: no we use a `bash -c` command to help eval env

// pkg/build.go:145
// FIXME: we can not add history for current API

// pkg/build.go:219
// FIXME: merge the image config field to confirm the output work

// pkg/env.go:32
// FIXME: do env mutate into image config later
```

---

## 2. Design Principles

### 2.1 Core Philosophy

> "Build image quickly, without docker daemon, no full blob pull, for artifact and CI scenarios"

### 2.2 Design Constraints

| Constraint | Implication |
|------------|-------------|
| No Docker daemon | Use crane for OCI operations |
| No full blob pull | Use registry API + layer diffing |
| CI-friendly | Stateless, containerizable |
| Artifact-focused | Prioritize COPY over RUN |

### 2.3 Target Use Cases

1. **Component Assembly**: Combine multiple service components into one image
2. **Artifact Packaging**: Wrap build artifacts as container images
3. **Simple Edits**: wget, tee, sed (limited RUN)
4. **Base Updates**: Rebase to new base image without full rebuild

---

## 3. Short-term Design (Now)

### 3.1 Fix Existing TODOs

#### A. History Tracking (`pkg/build.go:145`)
```go
// Current: History not added due to API limitations
// Design: Use mutate.Append with History field
for _, ins := range stage.Commands {
    add := mutate.Addendum{
        History: v1.History{
            CreatedBy: ins.Name(),
            // Comment, Author, Created, EmptyLayer
        },
    }
    img, _ = mutate.Append(img, add)
}
```

#### B. Env Mutation Fix (`pkg/env.go:32`)
```go
// Current: ENV stored in engine.Env, not mutated to image
// Design: Apply immediately via mutate.Config
cfg, _ := img.ConfigFile()
cfg.Config.Env = flattenEnv(engine.Env[stage])
img, _ = mutate.Config(img, cfg.Config)
```

#### C. Config Merge (`pkg/build.go:219`)
```go
// Merge: User, WorkDir, Entrypoint, Cmd, Labels, ExposedPorts
func mergeConfig(baseCfg, newCfg *v1.Config) *v1.Config {
    return &v1.Config{
        User:        coalesce(newCfg.User, baseCfg.User),
        WorkingDir:  coalesce(newCfg.WorkingDir, baseCfg.WorkingDir),
        // ... other fields
    }
}
```

#### D. Validation (`pkg/build.go:61`, `pkg/copy.go:48,129`)
```go
// Validate base image exists
// Validate COPY --from sources have proper annotations
// Validate tag format
```

---

## 4. Medium-term Design

### 4.1 Limited RUN Command

**Scope**: Only "simple, scope-in-control" commands (Design.md:31-43)

| Allowed | Not Allowed |
|---------|-------------|
| wget, curl | yum, apt-get |
| tee, sed, cat >> | useradd, groupadd |
| tar, unzip | systemctl, service |
| chmod, chown | package managers |

**Implementation Strategy**:
```go
// pkg/run.go - new implementation
type RunCommand struct {
    Cmd    string
    Scope  RunScope // LOCAL, TEMP, CONTAINER
}

func (engine *Engine) doRun(cmd *instructions.RunCommand, img v1.Image) (v1.Image, error) {
    // 1. Parse and validate command
    parsed := parseRunCommand(cmd)

    // 2. Check if allowed
    if !isAllowedCommand(parsed) {
        return nil, fmt.Errorf("RUN command not supported: %s", cmd)
    }

    // 3. Execute in temp directory
    tempDir := createTempRootfs()
    defer os.RemoveAll(tempDir)

    // 4. Extract current image to tempDir
    extractImage(img, tempDir)

    // 5. Execute command with chroot
    err := execInChroot(tempDir, parsed.CmdString)

    // 6. Archive changed files as layer
    layer := tarball.ChangesAsLayer(tempDir, baseline)

    return mutate.AppendLayers(img, layer)
}
```

### 4.2 Additional Dockerfile Commands

| Command | Priority | Complexity | Notes |
|---------|----------|------------|-------|
| `WORKDIR` | High | Low | Config field only |
| `USER` | High | Low | Config field only |
| `ADD` | Medium | Medium | Like COPY + auto-extract |
| `EXPOSE` | Low | Low | Config field only |
| `ENTRYPOINT` | Medium | Low | Config field only |
| `CMD` | Medium | Low | Config field only |
| `VOLUME` | Low | Low | Config field only |
| `LABEL` | Low | Low | Already in labels map |

### 4.3 Record Management (`pkg/record.go`)

**Purpose**: Track build steps for interactive mode (Design.md:212-234)

```go
type Record struct {
    UUID      string
    Date      time.Time
    Cursor    string    // Current step UUID
    Steps     []Step
}

type Step struct {
    UUID     string
    Parent   string
    Command  string
    Cleanup  bool
    Pushed   bool
}

// Implementation
type RecordManager struct {
    Path   string
    Record *Record
}

func (r *RecordManager) Load(path string) error
func (r *RecordManager) Save() error
func (r *RecordManager) AddStep(cmd string) (*Step, error)
func (r *RecordManager) GetStep(uuid string) (*Step, error)
```

---

## 5. Long-term Design

### 5.1 New CLI Commands (from Design.md)

#### A. `cigno base` - Update base annotations
```sh
cigno base <image> -b <new-base> -t <new-tag>
```

**Implementation**:
```go
// cmd/base.go
var baseCmd = &cobra.Command{
    Use: "base <image>",
    Run: func(cmd *cobra.Command, args []string) {
        img := crane.Pull(args[0])
        newBase := flag("base")

        // Update annotations
        annotations := map[string]string{
            AnnotationBaseImageName: newBase,
        }
        img = mutate.Annotations(img, annotations)

        crane.Push(img, flag("tag"))
    },
}
```

#### B. `cigno assemble` - Merge component images
```sh
cigno assemble -b <base> -a <comp1> -a <comp2> -t <tag>
```

**Implementation**:
```go
// cmd/assemble.go
func assemble(base string, components []string, tag string) error {
    // Start with base
    img := crane.Pull(base)

    // Append each component's diff layers
    for _, comp := range components {
        compImg := crane.Pull(comp)
        baseImg := getBaseImage(compImg) // from annotations

        adds, _ := subBaseImage(compImg, baseImg)
        img, _ = mutate.Append(img, adds...)
    }

    return crane.Push(img, tag)
}
```

#### C. `cigno component` - Create component with metadata
```sh
cigno component -b <base> -f <dockerfile> -t <tag>
```

### 5.2 Interactive Mode (Design.md:186-210)

```sh
# Interactive session
cigno session start
ID=$(cigno session new)
cigno session $ID from alpine:latest
cigno session $ID copy ./app /usr/local/bin/app
cigno session $ID env PATH=/usr/local/bin:$PATH
cigno session $ID commit -t myapp:latest
cigno session $ID cleanup
```

**Implementation**:
```go
// cmd/session.go
type SessionManager struct {
    Sessions map[string]*Session
    Record   *RecordManager
}

type Session struct {
    ID        string
    Image     v1.Image
    BuildDir  string
    CreatedAt time.Time
}
```

### 5.3 Local Registry Server (`registry/v2/`)

**Purpose**: Cache layer blobs locally, serve as build cache

```go
// registry/server.go
type LocalRegistry struct {
    Addr     string
    Storage  StorageBackend  // disk, s3, memory
    Blobs    map[string]Blob
    Manifests map[string]Manifest
}

// Serve registry v2 API on localhost:5000
func (r *LocalRegistry) Start() error
```

---

## 6. Architecture Improvements

### 6.1 Plugin System for Commands

```go
// pkg/plugin.go
type CommandHandler interface {
    Name() string
    CanHandle(*instructions.Command) bool
    Execute(*Engine, *instructions.Command, v1.Image) (v1.Image, error)
}

// Register handlers
engine.RegisterCommandHandler(&CopyHandler{})
engine.RegisterCommandHandler(&EnvHandler{})
engine.RegisterCommandHandler(&RunHandler{})
```

### 6.2 Validation Layer

```go
// pkg/validate.go
type Validator interface {
    Validate(*Engine, *instructions.Stage) error
}

type BaseImageValidator struct{}
type CopySourceValidator struct{}
type RunCommandValidator struct{}
```

### 6.3 Caching Strategy

```go
// pkg/cache.go
type Cache interface {
    Get(key string) (v1.Image, bool)
    Put(key string, img v1.Image) error
    Invalidate(key string) error
}

// Implementations:
// - MemoryCache: in-process LRU
// - RegistryCache: push/pull from local registry
// - DiskCache: tar files on disk
```

---

## 7. Testing Strategy

### 7.1 Unit Tests Needed

| File | Test Coverage Goal |
|------|-------------------|
| `pkg/copy.go` | Local COPY, --from image, --from tarball |
| `pkg/env.go` | $VAR expansion, nested expansion |
| `pkg/arg.go` | Global vs local scope |
| `pkg/tarball.go` | Path transformations, permissions |
| `pkg/run.go` | Whitelisted commands only |

### 7.2 Integration Tests

```go
// tests/integration_test.go
func TestBuildSimpleDockerfile(t *testing.T)
func TestBuildMultiStage(t *testing.T)
func TestBuildWithRebase(t *testing.T)
func TestBuildWithRun(t *testing.T)
func TestPushAndPull(t *testing.T)
```

### 7.3 Golden File Tests

Use `test/data/*.Dockerfile` as fixtures, compare output manifests.

---

## 8. Implementation Priority

| Phase | Tasks | Estimate |
|-------|-------|----------|
| **P0** | Fix history, env, config merge | 1-2 days |
| **P0** | Add validation | 1 day |
| **P1** | WORKDIR, USER, CMD, ENTRYPOINT | 1-2 days |
| **P1** | ADD command | 1 day |
| **P2** | Limited RUN (wget, tee, sed) | 3-5 days |
| **P2** | Record management | 2 days |
| **P3** | cigno base, assemble, component | 3-5 days |
| **P3** | Interactive session mode | 5-7 days |
| **P4** | Local registry server | 5-7 days |

---

## 9. Open Questions

1. **RUN command scope**: How to validate "safe" commands? Whitelist or sandbox?
2. **Cache invalidation**: When to invalidate cached intermediate images?
3. **Cross-platform**: Current code assumes Linux containers. Windows support?
4. **Security**: chroot execution for RUN needs privilege considerations.
5. **Registry auth**: Need credential helpers for private registries?

---

## 10. References

- [OCI Image Spec](https://github.com/opencontainers/image-spec)
- [go-containerregistry](https://github.com/google/go-containerregistry)
- [crane docs](https://github.com/google/go-containerregistry/blob/main/cmd/crane/README.md)
- Design.md - Original design document (Chinese)
