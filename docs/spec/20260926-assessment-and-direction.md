# Cigno Value Assessment & Direction Decision

**Date**: 2026-09-26
**Status**: Accepted

## 1. What Cigno Is

A small (≈2.5k LOC) Go CLI that assembles container images from a subset of
Dockerfile instructions **without a Docker daemon**, using pure OCI registry
operations via `crane`:

- `COPY` local artifacts / tarballs into an image
- `COPY --from=<image>` as a **layer-level rebase** (only diff layers are
  transferred — the base is never re-pulled in full)
- Config-only instructions (ENV/ARG/WORKDIR/USER/CMD/ENTRYPOINT/LABEL/EXPOSE/VOLUME)
- Local disk cache for base images; embedded registry for local push/pull
- Output: push to registry or docker-loadable tar

## 2. Is It Still Worth Keeping? (especially in the AI era)

### 2.1 Arguments FOR

1. **Agent-driven CI has no daemon.** AI coding agents and sandboxed CI
   runners increasingly build and ship containers in environments where
   there is no Docker socket and no privileged runtime (rootless, gVisor,
   Kata sandboxes). A single static binary that produces correct OCI images
   with zero daemon/privilege requirements fits exactly there. This is more
   relevant in 2026 than when the project started.
2. **The niche is real and narrow.** Component assembly + artifact packaging
   (drop prebuilt artifacts into a stable base, occasionally rebase to a new
   base) is a genuine workflow — see the success of narrow builders like
   `ko` and `apko`. Cigno is not competing with BuildKit on general builds.
3. **Rebase is cheap where others are not.** kaniko/buildah re-execute and
   re-push; `crane rebase` exists but has no Dockerfile UX. Cigno's
   annotation-based diff-layer rebase keeps multi-component images cheap to
   update.
4. **Cost of ownership is tiny.** Small codebase, no runtime/overlay/security
   surface (no RUN execution), pure stdlib + crane. An AI agent (or a human)
   can hold the entire codebase in context — cheap to maintain and to modify
   with AI assistance.

### 2.2 Arguments AGAINST (and responses)

1. *"kaniko/buildah/buildkit already do this"* — they do much more, at much
   higher cost: big images, slow cold starts, privileges or fuse overlays.
   For artifact assembly they are overkill. Cigno's honest positioning is
   "assembly builder, not a general builder".
2. *"AI can just generate Dockerfiles for docker build"* — generating the
   Dockerfile was never the bottleneck; **executing** it in a daemon-less
   sandbox is. Cigno is on the execution side of that problem.
3. *"RUN is missing, so most Dockerfiles fail"* — true, and it should stay
   that way (see decision below). The tool is for builds that are already
   artifact-shaped.

### 2.3 Verdict

**Keep the project**, with a sharply narrowed scope. It is not a kaniko
replacement and must not try to become one.

## 3. Decisions

| Decision | Rationale |
|---|---|
| **Keep**: assembly/rebase core (COPY, COPY --from, config cmds, cache, embedded registry) | This is the differentiated value; now covered by hermetic tests |
| **Abandon**: interactive `session` mode and `record.go` | Contradicts the CI/stateless design constraint; dead stub deleted |
| **Keep RUN unimplemented (by design)** | Safe RUN requires a real runtime (chroot/userns + package managers) — that is buildah's job. Whitelist-RUN would be fragile and surprising. Document this loudly instead |
| **Defer**: `cigno base` / `assemble` / `component` dedicated commands | The Dockerfile path already covers these; add only on real demand |
| **Invest**: correctness, hermetic tests, CI, docs, positioning | Done in this pass (see changelog below); CI + release packaging next |

## 4. What Was Done in This Pass (2026-09-26)

**Repaired**
- Repo did not compile at HEAD: `pkg/registry/server.go` was described in a
  commit message but never committed. Reimplemented the embedded registry
  (registry v2 API: blob upload init/chunked/monolithic/finish with sha256
  verification, manifest PUT/GET/DELETE, tags list) on stdlib `net/http`.
- `tar()`: single-file `COPY` sources were silently dropped (only dirs were
  walked); relative sources resolved against process CWD instead of the
  build context (so `--context` only worked by accident).
- `createBlob`: discarded `ReplacePrefixPath` option results (broken
  multi-file dest semantics); panic-based error handling → returned errors.
- `createAddendums` (rebase): images whose history is shorter than their
  layer list — i.e. **every cigno-built image** — copied zero layers on
  `COPY --from`; fallback loop had an off-by-one and wrong index.
- `subBaseImage`: ignored errors, index-out-of-range on short images,
  cryptic mismatch errors.
- `Build`: `FROM scratch` tried to pull from a registry (fatal); unknown
  instructions silently ignored; `mutate.Config` result discarded; base
  digest annotation recorded the wrong digest; empty arch/os in scratch
  builds (now defaulted to amd64/linux).

**Added**
- `--build-arg` wired to the engine (previously parsed and dropped);
  stage-level `ARG KEY` inherits the global default (docker behavior)
- ARG/ENV expansion in `COPY` sources/dest and `LABEL` values
- Base-image disk cache actually wired (`--no-cache` to disable);
  `cigno cache list/clear` commands
- `--context`, `--output-file`, `--image-base/--tarball/--folder` flags now
  actually take effect; validate-only mode (`-v`) parses without building
- Local-host registry refs automatically use plain HTTP (embedded registry
  works end-to-end without TLS setup)

**Tests** (all hermetic — no network, no docker; `go test ./...`)
- Embedded registry: crane push/pull round-trip, tags list, digest mismatch
- Builds: local copy, single-file copy, `FROM scratch`, `COPY --from` rebase
  (layer-count + content assertions), ARG expansion, invalid-ref rejection
- Units: `createAddendums`, `setEnvVars`, arg expansion, nil-map safety,
  tarball path transforms, cache put/get/list/clear

**Known remaining gaps** (acceptable for now)
- No wildcard (`COPY *.txt`) matching
- No multi-stage `COPY --from=<stage-name>` (works with image refs, tarballs)
- History `created` timestamps are zero values
- No CI pipeline / release packaging yet

## 5. Gap Closure Pass (2026-09-27)

All four gaps above were closed the next day:

- **Multi-stage `COPY --from=<stage>`**: built stages are recorded on the
  engine; a stage reference now copies paths out of the stage's built
  filesystem (docker semantics), while image-ref rebase keeps verbatim
  layer assembly semantics.
- **Wildcard COPY**: glob patterns (`* ? [..]`) expand against the build
  context (sorted); zero matches fail the build like docker.
- **`.dockerignore`**: honored for local COPY/ADD via moby/patternmatcher
  (negation + parent-dir pruning); also fixed `COPY . /data` dest remap.
- **Timestamps**: image `created` and history entries now carry real
  build times instead of zero values.
- **Release packaging**: goreleaser config (linux/darwin, amd64/arm64)
  plus a tag-triggered release workflow; `cigno --version` added.

## 6. COPY/ADD Completion Pass (2026-09-27)

The "limited RUN" whitelist from the 2026-01 roadmap is formally retired:
its real needs are all COPY/ADD features, now implemented:

- `COPY/ADD --chown/--chmod` wired through (numeric ids + root; unresolvable
  names error with a hint; verbatim mode like docker; applied on all
  path-level paths, ignored-with-warning on verbatim image rebase)
- `RUN` now fails the build with guidance instead of warn-and-skip
- ADD extraction rewritten to the docker/moby-archive standard: magic-byte
  compression detection (gzip/bzip2/xz), tar-only extraction, `..` traversal
  rejected, metadata preserved, whiteouts dropped, URLs treated like local
  files (the old code never extracted compressed archives at all)
