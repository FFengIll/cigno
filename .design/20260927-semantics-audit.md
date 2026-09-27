# Cigno × Dockerfile Semantics Audit

**Date**: 2026-09-27
**Baseline**: docs.docker.com Dockerfile reference (fetched 2026-09-27) + buildkit v0.8.3 parser source (the pinned parse layer)
**Scope**: every instruction cigno accepts, plus the explicit not-supported list

## Verdict

After the fixes listed in §3, every instruction cigno accepts implements the
documented docker semantics, and every accepted behavior has a harness test
(`go test ./...`, hermetic). Everything NOT in the tables below is rejected
or warned-and-skipped — never silently mis-executed.

## 1. Instructions implemented (semantics-aligned, tested)

| Instruction | Docker semantics honored | cigno notes | Test |
|---|---|---|---|
| `FROM` | image ref, tag/digest, `scratch`, ARG expansion before/after FROM, stage names | local-disk cache; localhost registries over plain HTTP | `TestEngine_Build_*` (scratch, rebase, multistage, arg) |
| `RUN` | n/a — **rejected by design** with guidance (assembly builder, no command execution) | hard error, not warn-skip | `TestEngine_Build_RunRejected` |
| `CMD` | exec form verbatim; shell form wrapped: `CMD echo hi` → `["/bin/sh","-c","echo hi"]` | last-wins | `TestConfig_CmdShellForm`, `TestConfig_CmdExecForm` |
| `ENTRYPOINT` | same shell/exec forms as CMD | — | `TestConfig_EntrypointShellAndExec` |
| `ENV` | chain expansion (`B=$A`); ENV overrides same-named ARG; undefined refs → `""` | order preserved in config | `TestConfig_EnvChainAndArgPrecedence`, `TestConfig_UndefinedVarExpandsEmpty` |
| `ARG` | global scope before FROM; in-stage `ARG KEY` redecl + inherits global default; no image persistence | — | `TestEngine_Build_ArgExpansion`, `TestConfig_EnvChainAndArgPrecedence` |
| `WORKDIR` | absolute as-is; relative chains onto previous WORKDIR (inherited from base, default `/`); `$ENV` expansion | — | `TestConfig_WorkdirAbsoluteRelativeAndEnv` |
| `USER` | verbatim, env-expanded | — | `TestConfig_UserExposeVolumeLabel` |
| `LABEL` | multi key=value, inheritance from base, var expansion | — | `TestConfig_UserExposeVolumeLabel` |
| `EXPOSE` | port[/proto], var expansion | — | `TestConfig_UserExposeVolumeLabel` |
| `VOLUME` | list, var expansion | — | `TestConfig_UserExposeVolumeLabel` |
| `COPY` | file→dest rename; dir→contents under dest; multi-source → dest is dir; wildcards (filepath.Match); `../`-stripping of sources; `.dockerignore`; `--chown/--chmod` (numeric + root, verbatim mode); `--from=<stage>` (path-level) / `--from=<image>` (layer rebase, cigno-specific assembly semantics documented separately) | env replacement in sources/dest | `TestEngine_Build_Copy*`, `TestConfig_CopyPathUsesEnv`, `TestConfig_CopyStripsParentNavigation`, `TestEngine_Build_CopyChownChmod`, `TestEngine_Build_DockerIgnore` |
| `ADD` | local tar extraction with content-based compression detection (gzip/bzip2/xz/identity); compressed non-tar added verbatim; **remote URLs never decompressed**; URL naming (trailing slash → basename; else dest is filename); multi-source: each local tar extracted, dest is dir, single layer; `--chown/--chmod`; whiteout markers dropped; `..` traversal rejected | git-repo sources unsupported (errors as unknown local path) | `TestExtractTar_*`, `TestAddURL_NeverExtracted`, `TestAdd_MultiSourceSingleLayer` |
| `.dockerignore` | moby/patternmatcher semantics (negation, parent-dir pruning) | context root only | `TestEngine_Build_DockerIgnore` |

## 2. Instructions NOT supported (explicit)

| Instruction | Behavior in cigno | Rationale |
|---|---|---|
| `RUN` | build error | no command execution, by design |
| `SHELL`, `HEALTHCHECK`, `STOPSIGNAL`, `ONBUILD`, `MAINTAINER` | warn + skip | low value for assembly builds; never mis-executed |
| `ADD <git-url>` | error (treated as local path) | no git support |
| `COPY --link/--parents/--exclude`, `ADD --unpack/--checksum/--keep-git-dir/--link/--exclude` | parse error (pinned buildkit parser predates them) | backlog; `--unpack` is the most likely future flag |
| `--chmod` symbolic notation (`+x`, `u=rwX`) | parse error (parser accepts flag; only octal values implemented) | backlog |

## 3. Deviations found by this audit (all fixed 2026-09-27)

| # | Deviation | Fix |
|---|---|---|
| 1 | shell-form CMD/ENTRYPOINT stored unwrapped (`["echo","hi"]`) | wrapped in `["/bin/sh","-c",...]` (buildkit `PrependShell`) |
| 2 | relative WORKDIR did not chain onto previous WORKDIR | chains; base-inherited WorkingDir honored; `//` avoided |
| 3 | undefined `$VAR` expanded to literal `$VAR` | expands to `""` (docker) |
| 4 | ENV values invisible to expansions (COPY/WORKDIR/etc. only saw ARGs); no ENV>ARG precedence | unified `expandEnvIn`: ENV first, then local ARG, then global ARG |
| 5 | `COPY ../x` errored on the context-root check | `../` navigation stripped, per docs |
| 6 | WORKDIR ignored `$ENV` | env-expanded |

## 4. Known gaps (documented, not semantic violations)

- `ENV`/`ARG` values are not used to override `FROM` platform; no `--platform` support.
- ADD URL details: response `Last-Modified` → mtime is not applied; URL files keep temp-file mode 0600 (docker uses 600 too — aligned by construction).
- History `created_by` strings are the raw instruction text, not docker's exact formatting.
- COPY/ADD cache invalidation differs from docker (no content-hash caching yet; base-image disk cache only).
