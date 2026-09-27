# Cigno

**A daemonless container image *assembly* builder.**

Cigno assembles container images from prebuilt artifacts and component
images using plain OCI registry operations — no Docker daemon, no
privileged runtime, no overlay filesystem. One static binary, built for
sandboxed CI and AI-agent environments.

```bash
cigno build -f Dockerfile -o myapp.tar
```

> **What it is / is not**
>
> ✅ Drop prebuilt artifacts into an image (`COPY` / `ADD`)
> ✅ Assemble component images onto a shared base (layer-level rebase)
> ✅ Config edits: `ENV` / `WORKDIR` / `CMD` / `LABEL` / …
> ❌ Runs nothing. `RUN` is rejected by design — for command execution use
>    [BuildKit](https://github.com/moby/buildkit) / [kaniko](https://github.com/GoogleContainerTools/kaniko),
>    then feed their artifacts back into cigno.

---

## Why cigno

| | Docker/BuildKit | kaniko | cigno |
|---|---|---|---|
| Daemon / privilege | needs daemon | rootless but heavy | **none, single binary** |
| Work model | executes commands | executes commands | **assembles files & layers** |
| Rebase to new base | full rebuild/re-push | full rebuild | **diff layers only** |
| Cold start / footprint | large | large | tiny |

Typical flow: build/test artifacts anywhere → `cigno build` wraps them into
an OCI image → push (or save as docker-loadable tar). Rebuilding against a
new base image only ships the layers that changed.

## Install

```bash
go install github.com/feng-project/cigno@latest
```

Prebuilt binaries for linux/darwin (amd64/arm64) are attached to releases
(goreleaser). Check the installed version with `cigno --version`.

## Quick Start

```bash
cigno build -f Dockerfile -t myapp:latest          # build & push
cigno build -f Dockerfile -o myapp.tar             # …or save a docker-loadable tar
cigno build -f Dockerfile --validate               # parse-only check
cigno build -f Dockerfile --build-arg VERSION=1.2.3 ...
cigno build -f Dockerfile -c /path/to/context ...  # custom build context
cigno build -f Dockerfile --no-cache ...           # skip the base-image disk cache
```

## Dockerfile support

Cigno implements a **strict, verified subset** of the Dockerfile spec —
semantics are checked line-by-line against the official reference and locked
by tests (see the [semantics audit](.design/20260927-semantics-audit.md)).

| Instruction | Status | Semantics |
|---|---|---|
| `FROM` | ✅ | image / tag / digest / `scratch`; ARG-expanded; multi-stage with named stages |
| `COPY` | ✅ | files & directories, wildcards, `.dockerignore`, `../`-stripping, `--chown`/`--chmod`, `--from=<stage>` (path-level) and `--from=<image>` (layer rebase, see below); env/arg expansion in paths |
| `ADD` | ✅ | like COPY, plus: **local** tar/tar.gz/tgz/tar.bz2/tar.xz auto-extract (detected by file content, not extension); **remote URLs are placed verbatim, never decompressed** — docker semantics |
| `ENV` | ✅ | chained expansion; overrides same-named ARG; undefined vars → empty |
| `ARG` | ✅ | global + stage scope; in-stage `ARG KEY` inherits the global default |
| `WORKDIR` | ✅ | absolute / relative chaining / env expansion |
| `USER` `EXPOSE` `VOLUME` `LABEL` | ✅ | config fields with env/arg expansion |
| `CMD` `ENTRYPOINT` | ✅ | exec form verbatim; shell form wrapped in `/bin/sh -c` |
| `RUN` | ❌ | **rejected with guidance** — cigno never executes commands |
| `SHELL` `HEALTHCHECK` `STOPSIGNAL` `ONBUILD` `MAINTAINER` | ⚠️ | warned and skipped |
| `REBASE` (cigno extension) | ⏳ | planned |

Full per-instruction mapping with test references:
[.design/20260927-semantics-audit.md](.design/20260927-semantics-audit.md).

## Component assembly & rebase

The differentiating feature: `COPY --from=<image>` copies **only the diff
layers** of that image onto the current one (the base is never re-pulled in
full). Update a component, rebuild the composite, push one layer.

```dockerfile
FROM registry.internal/base:2.1          # shared, stable base
COPY --from=registry.internal/comp-a:1.4 / /   # verbatim layer rebase
COPY --from=comp-build /out/bin /usr/local/bin/ # path-level from a build stage
COPY conf/app.yaml /etc/app/
```

## Embedded registry

A built-in registry v2 server (stdlib HTTP, filesystem storage) serves as a
local cache / push target — localhost refs automatically use plain HTTP:

```bash
cigno registry start [--addr 127.0.0.1:5000] [--storage ~/.cigno/registry]
```

## Caching

Base images are cached on disk (`~/.cigno/cache/`) and reused across builds:

```bash
cigno cache list
cigno cache clear
```

## Development

```bash
task build   # go build
task test    # hermetic: spins up the embedded registry in-process; no network, no docker
task vet     # vet + gofmt
task --list  # see all tasks
```

Repository layout: `cmd/` (CLI: build, registry, cache, version) ·
`pkg/` (engine, copy/add, archive detection, tar tooling, cache,
`registry/` embedded server) · `.design/` (architecture, specs, audits) ·
`.github/` (CI + release workflows).

### Design constraints

- No Docker daemon — pure OCI operations via
  [go-containerregistry](https://github.com/google/go-containerregistry)
- No full blob pull — registry API + layer diffing for rebase
- Stateless and CI-friendly; containerizable
- Dockerfile semantics: strict subset, never guess — when in doubt, follow
  the reference and add a test

## Documentation

| Doc | Content |
|---|---|
| [Semantics Audit (2026-09)](.design/20260927-semantics-audit.md) | per-instruction spec alignment + tests |
| [Value Assessment & Direction (2026-09)](.design/20260926-assessment-and-direction.md) | positioning, decisions, changelog |
| [Design Roadmap (2026-01)](.design/20260121-design-roadmap.md) | historical spec (partially superseded) |
| [Architecture](.design/20260121-arch.md) | internals |

## License

[MPL 2.0](LICENSE)
