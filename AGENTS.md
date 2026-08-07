# AGENTS.md

Reference guide for AI agents working in the `opnborg` repository.

> ## FIXED REQUIREMENT — EVERY CHANGE, NO EXCEPTIONS
>
> Before a task or change is considered done, all five steps below MUST be completed
> in this exact order. Skipping or reordering any step is a failure.
>
> 1. **Format source code** — run `gofmt -w .` (or `make check`) so the tree
>    stays gofmt-clean.
> 2. **Build** — `go build -o opnborg ./cmd/opnborg` must succeed.
> 3. **Test** — `go test -count=1 ./...` must be ok
> 4. **Commit** — `git add . && git commit -m '<message>'`.
> 5. **Tag** — bump the patch segment only: the result is `v0.1.<N+1>`
>    (the current release series; the latest tag is `v0.1.137`). Never move,
>    delete, or reuse an existing tag. Also bump the `SemVer` constant in
>    `api.go` to match the new tag.
>
> These steps are non-negotiable for every single task regardless of size.


## Project Summary

`opnborg` is a single-binary Go daemon that backs up, monitors, and synchronizes configuration across a fleet of OPNsense firewalls and (optionally) Unifi controllers. Configuration is driven entirely through environment variables; the binary itself accepts only `-v` / `-h`. Output is emitted to stdout via an internal `displayChan` log engine, and an embedded HTTP server renders the hive status as HTML. Backups are stored as XML on disk, deduplicated by SHA-256, and (optionally) committed to a local git repo.

- Module path: `paepcke.de/opnborg`
- Go version: see `go.mod` (currently `1.26.5`); CI `golang.yml` pins `1.23`, Dockerfile + `release.yml` pin `1.26`
- Entry point: `cmd/opnborg/main.go` -> `opnborg.Start(config)` -> `srv(config)`
- License: BSD 3-Clause

## IMPORTANT FOR EVERY SINGLE TASK: NEVER SKIP THIS ACTIONS

- Test every change via build and unit tests
- commit each task into git repo when done
- **Every committed task must be tagged with a git semver tag**, bumping only
  the **patch** segment, the last segment (e.g. `v0.1.132` → `v0.1.133`).
  The other two first segments (major, minor) stay unmodified,
  increment only the last part by +1. Never move, delete, or rewrite an existing tag.

## Build, Run, Check

```sh
# Build the whole module (used by CI)
CGO_ENABLED=0 go build ./...

# Build just the binary (matches Dockerfile/goreleaser)
CGO_ENABLED=0 go build -ldflags="-w -s" ./cmd/opnborg

# Run locally from a checked-out repo (after sourcing env config)
go run cmd/opnborg/main.go

# Run via remote module
go run paepcke.de/opnborg/cmd/opnborg@main

# Format / lint / vet (top-level Makefile target)
make check        # runs gofmt -l ., go vet ./..., go mod tidy -diff

# Run the test suite (race detector enabled)
make test         # runs go test -race -count=1 ./...

# Dependency refresh (DESTRUCTIVE - rewrites go.mod/go.sum)
make deps

# Install
go install paepcke.de/opnborg/cmd/opnborg@main
```

`make check` is the canonical pre-PR gate; it runs `gofmt -l .`, `go vet ./...`, and `go mod tidy -diff`. There is no `staticcheck` wiring in the Makefile or CI.

### Releasing

- Releases are tag-driven (`v*`). Pushing a `v*` tag triggers both `.github/workflows/release.yml` (goreleaser cross-compile via `.goreleaser.yml`) and `.github/workflows/ghcr.yml` (build/push `ghcr.io/paepckehh/opnborg:latest`).
- goreleaser builds target linux/freebsd/darwin/netbsd/openbsd/windows on amd64 + arm64, `CGO_ENABLED=0`.
- **Before tagging a release**, bump the `SemVer` constant in `api.go` to match the new tag (e.g. `v0.1.132` → `v0.1.133`). `SemVer` is the single source of truth for the version string and is consumed by the CLI startup banner (`cmd/opnborg/main.go`) and the WebUI footer. The top-level `Makefile` injects a build version via `-ldflags -X paepcke.de/opnborg/internal/version.Version=...` (sourced from `git describe --tags`), but no `internal/version` package exists, so that flag is currently a no-op and the displayed version comes from `SemVer`.

## Architecture & Control Flow

All package files live at the repository root (`/`) under `package opnborg`. `cmd/opnborg/main.go` is the only file in `package main`.

### Startup sequence

1. `main.go` parses only `-v` / `-h`; anything else is a fatal error. All other configuration is ENV-only.
2. `opnborg.Setup()` (`setup.go`) loads `.env` (via `godotenv`), reads every `OPN_*` env var, sanitizes, and returns a populated `*OPNCall` config struct (defined in `api.go`).
3. `opnborg.Start(config)` -> `srv(config)` (`srv.go`) is the orchestrator.

### `srv()` main loop (`srv.go`)

- Spins up goroutines for: display/log engine (`startLog`), background timer, internal HTTP server (`startWeb`), RFC5424 syslog server (`startRSysLog`), Unifi backup (`srvUnifiBackup`), Unifi asset export (`srvUnifiExport`), and Unifi autoBackup folder watch (`srvUnifiWatch`), each guarded by its respective `config.*.Enable` flag.
- When `config.Git.Enable`, `gitInit(config)` runs once at startup (before the first worker pass) to open-or-init the storage repo and write `.gitignore`.
- Builds the `servers` slice by splitting `config.Targets` on commas; each entry may carry an asset tag separated by `#` (e.g. `opn01.lan#edge-1`). The split-on-`#` switch is duplicated in `srv()` (status init) and the main loop (worker dispatch) -- keep them in sync.
- Main loop body per tick:
  1. Reset `config.dirty` to false.
  2. If `config.Sync.Enable`: `readMasterConf(config)` pulls the master OPNsense XML and derives the package list (`sync-master.go`).
  3. For each server: `go actionOPN(...)` with a `sync.WaitGroup`; `wg.Wait()` blocks until the whole hive finishes a pass.
  4. If `config.dirty.Load()` (set atomically by workers when they wrote new XML), run `gitCheckIn` (commit + optional push).
  5. In daemon mode, block on `<-updateOPN` (tick channel from the background timer, or poked by the `/force` HTTP handler via a non-blocking select). In one-shot mode (`OPN_NODAEMON`), close `displayChan`, wait, and return.

### Per-server backup (`actionOPN.go`)

1. If sync or syslog is enabled, `fetchOPN(server, config)` pulls + XML-unmarshals the live config into an `*Opnsense` (`struct-xml-opn.go`).
2. `checkInstallPKG` (only on non-master hosts, `sync-pkg.go`) diffs the host's installed plugins against the master package list and calls `installPKG` per missing package.
3. `checkRSysLogConfig` (`rsyslog-clientconf.go`) ensures the remote-syslog client config matches.
4. `fetchXML` (`transport.go`) downloads the actual backup XML via `/api/core/backup/download/this` with HTTP basic auth.
5. SHA-256 the new XML; compare to the previous `CONFIG-CURRENT` (`store.go::lastSum`). If unchanged, mark status and skip storage.
6. On change: `checkIntoStore` writes the timestamped archive file, rotates the `current.<ext>` / `CONFIG-CURRENT` / `CONFIG-LAST` pointers, and sets `config.dirty.Store(true)` so the main loop will commit. `checkIntoStore` resolves all paths against `config.Path` and does **not** call `os.Chdir`.

### HTTP WebUI (`srvHttpd.go`, `httpd-handler.go`, `httpd-ui.go`, `httpd-transport.go`)

- `mux`: `/` (index), `/files/` (static file server rooted at `config.Path`), `/force` (manual trigger), `/favicon.ico`.
- Index handler renders HTML built from inlined SVG/HTML constants in `httpd-ui.go`. `_head`, `_forceRedirect` are assembled at `Setup()` time from `OPN_HTTPD_COLOR_FG` / `OPN_HTTPD_COLOR_BG`.
- Status strings (`_ok`, `_fail`, `_na`, `_degraded`, `_unifi`) are inline animated SVGs defined as `const` in `httpd-ui.go`. `status.go` mutates the `hive` / `unifiStatus` / `unifiWatchStatus` strings under `hiveMutex` / `unifiMutex` / `unifiWatchMutex`.
- A `addSecurityHeader` middleware wraps index and `/files/`.
- The `/force` handler pokes the `updateOPN` / `updateUnifiBackup` / `updateUnifiExport` / `updateUnifiWatch` channels with non-blocking selects (buffer-1 channels): if a backup pass is already pending it drops the duplicate rather than blocking the HTTP client for a full backup cycle.
- The httpd is armed only in daemon mode and only when `OPN_HTTPD_DISABLE` is unset. In one-shot mode (`OPN_NODAEMON`) `config.Httpd.Enable` stays false so `startWeb` is never called — previously it defaulted to true with an empty listen address.
- TLS is opt-in via `OPN_HTTPD_CACERT` + `OPN_HTTPD_CAKEY`; setting `OPN_HTTPD_CACLIENT` enables mTLS enforcement (`httpd-transport.go::getHTTPTLS`).

### OPNsense API endpoints (`transport.go`)

Hardcoded under the `_api*` consts:
- `/api/core/backup/download/this` - XML backup fetch (no legacy endpoint support)
- `/api/core/firmware/status/` - firmware version JSON (`struct-json-firmwareStatus.go`)
- `/api/core/firmware/install/<pkg>` - plugin install (POST)

HTTPS is mandatory; the client intentionally skips OS trust store verification and relies on `OPN_TLSKEYPIN` (SHA-256 base64 of the SPKI) for MitM-proofing. See `getTlsConf` / `getTransport` / `opnClient` in `transport.go` (not `httpd-transport.go`, which only holds the WebUI listener `getHTTPTLS`).

## Configuration Conventions (ENV)

- **Boolean env vars are presence-based, not value-based**: setting `OPN_DEBUG=0`, `OPN_DEBUG=false`, or `OPN_DEBUG=1` all evaluate to `true` via `isEnv()` (`littlehelper.go`) as long as the value is non-empty. To disable, unset the var. The only exception is the empty string, which `isEnv` treats as false.
- `OPN_NODAEMON` inverts the default (daemon defaults to `true` when unset): `Daemon = !isEnv("OPN_NODAEMON")`. In one-shot mode the httpd, rsyslog server, and Unifi goroutines are not armed.
- The backup storage git repo feature is opt-in via `OPN_GIT_ENABLE` (presence-based). `OPN_GIT_UPSTREAM` sets an upstream SSH git URL to push to, and `OPN_GIT_SSH_KEY` points at the PEM-encoded private key used for upstream auth; both are validated together in `validateGitConfig` (`git.go`). Host key verification relies on go-git's default `~/.ssh/known_hosts` callback. (`OPN_NOGIT` is no longer honored — the feature is opt-in, not opt-out.)
- **Ollama-assisted commit messages** (opt-in via `OLLAMA_DESC_URL` + `OLLAMA_DESC_MODEL`, both must be non-empty). When enabled, before each non-Unifi commit the HEAD-vs-worktree diff is POSTed to the Ollama REST API (`<OLLAMA_DESC_URL>/api/generate`, `stream=false`) with a prompt that casts the model as an infrastructure / Unix firewall expert and asks for a short headline plus an extensive explanation grounded in OPNsense XML firewall configuration semantics. The returned text becomes the commit message. Commits whose changed files are all `.unf` Unifi backups keep the static default message (`opnborg auto update`) without consulting the model, since the `.unf` format is an opaque binary archive. Any model error, empty response, or timeout falls back to the default message so a model outage never blocks a backup from being committed. Implemented in `ollama.go`; wired into `gitCommit` (`git.go`). The diff is computed in-process with `go-difflib` against HEAD blobs and worktree files (capped at `_ollamaMaxDiffBytes`, currently 256 KB). It is an **enriched** diff, not raw hunks: `gitDiffText` emits a `=== COMMIT SUMMARY ===` header (files changed, total +insertions/-deletions, per-file change kind and +/- counts), then per-file `=== FILE ===` blocks carrying change kind, before/after byte and line sizes, detected OPNsense XML top-level sections (via `xmlTopLevelSections`, which walks the `<opnsense>` root children such as `filter`, `aliases`, `interfaces`, `gateways`, `nat`, `ipsec`, `vpn`, `cert`), the unified hunks with widened context (`_ollamaDiffContext`, currently 8 lines), and for small files (`_ollamaSmallFileBytes`, currently 16 KB) the full resulting content so the model can describe the complete new state. The system prompt tells the model how to read this structure so it grounds its description in concrete change geometry and the affected configuration subtrees. The config dashboard (`config-dashboard.go::renderOllamaPanel`) shows the parsed `OLLAMA_DESC_URL` / `OLLAMA_DESC_MODEL` values plus a live probe of the daemon (`ollama.go::ollamaHealthCheck`) that GETs `<OLLAMA_DESC_URL>/api/tags` with a short timeout (`_ollamaHealthTimeout`, 3 s) on every dashboard render and reports three layered signals: server reachable, REST API ready (parseable `/api/tags` JSON), and model ready (the configured model is present in the tags list, matched by exact name or `<model>:<tag>` prefix via `ollamaModelMatch`).
- The internal httpd is armed only in daemon mode and only when `OPN_HTTPD_DISABLE` is unset (daemon mode + flag unset → enable; everything else → disable).
- Either OPN backup (`OPN_APIKEY` + `OPN_APISECRET`) or Unifi backup (`OPN_UNIFI_BACKUP_USER` + `OPN_UNIFI_BACKUP_SECRET` + `OPN_UNIFI_VERSION`) must be configured or `Setup()` returns a fatal error.
- `OPN_TARGETS` is comma-separated. Each host may append `#<asset-tag>`. Custom groups use `OPN_TARGETS_<GROUPNAME>` with the same syntax; `OPN_TARGETS_DESC_<GROUPNAME>` supplies the group's WebUI text description, and `OPN_TARGETS_IMGURL_<GROUPNAME>` supplies a custom image URL that replaces the text headline (the description then becomes the image's tooltip).
- `OPN_TARGETS` / `OPN_MASTER` entries must include a port suffix if not `:443` (e.g. `192.168.0.1:8443`). Clear-text HTTP is unsupported.
- `.env` is auto-loaded by `godotenv.Load()` if present at the working directory.
- Example env templates live at the repo root: `example.sh`, `example-env-config-simple.sh`, `example-env-config-complex.sh`, `example-env-config-unifi.sh`, `example-env-config-dev.sh`.

See `README.md` for the canonical, exhaustive env var reference.

## On-disk Storage Layout (`store.go`, `git.go`)

For a configured `OPN_PATH` (default `.`):

```
<OPN_PATH>/
  .gitignore                         # auto-created: ignores ".archive", "CONFIG*", "Logs"
  <server>/
    current.xml                      # regular file holding the latest backup XML (served by the WebUI)
    CONFIG-CURRENT                   # symlink to the latest .archive entry (read by lastSum for SHA-256 compare)
    CONFIG-LAST                      # previous CONFIG-CURRENT symlink (renamed on each rotation)
    .archive/<YYYY>/<MM>/<YYYYMMDDTHHMMSSZ>-<server>.xml
    sha256.db                        # append-only log: <archive-name>\t<base64-sha256>
  Logs/current.log                   # rotated by lumberjack (256MB, 256 backups, 180d, gzipped)
```

`git.go` manages the storage folder as a git repo (opt-in via `OPN_GIT_ENABLE`). `gitInit` (`git.PlainOpen` / `git.PlainInit` on first run) plus `gitEnsureIgnore` run once at startup; `gitCheckIn` (`os.Chdir(config.Path)`, open repo, `wtree.Status()` fast-path, `wtree.Add(".")`, commit authored as `OPNBORG-AUTO-COMMIT <config.Email>`) runs per tick when `config.dirty` is set. The commit message is the static `opnborg auto update` unless Ollama-assisted generation is enabled (`OLLAMA_DESC_URL` + `OLLAMA_DESC_MODEL`, see `ollama.go`), in which case `gitCommit` calls `generateCommitMessage` to route the diff to the model and use its response; `.unf`-only commits and any model failure keep the default message. When `OPN_GIT_UPSTREAM` is set, `gitPush` recreates the `origin` remote if its URL drifted and pushes via `ssh.NewPublicKeysFromFile` from `OPN_GIT_SSH_KEY`; host key verification uses go-git's default `~/.ssh/known_hosts` callback. All git operations use the native `go-git` library — no external `git` binary is invoked.

`checkIntoStore` (`store.go`) resolves every path against `config.Path` and does **not** call `os.Chdir`, so it is safe to invoke from the concurrent per-server worker goroutines. The `CONFIG-CURRENT` / `CONFIG-LAST` symlinks use a relative target (the `.archive/...` path) so the store tree stays portable when copied or moved. The only remaining `os.Chdir` call sites are `gitInit` / `gitCheckIn` (both Chdir to `config.Path`, the same directory) and `startWeb` / `startRSysLog` (startup only, before workers run).

## Testing

The test suite lives in `littlehelper_test.go` (package `opnborg`) and covers env parsing (`isEnv`, `Setup`), URL helpers, the OPN/Unifi group builders, `splitPlugins`/`checkInstallPKG`, the syslog config comparison, the git init/checkin round-trip, the Unifi autoBackup watch setup/sync, `checkIntoStore` rotation, the compression helpers, the httpd enable gating, and the non-blocking `/force` handler. Run it via `make test` (`go test -race -count=1 ./...`) — note that `-race` requires CGO + a C compiler, so on minimal/CGO-disabled toolchains use `go test -count=1 ./...` instead. CI (`.github/workflows/golang.yml`) still runs only `go build ./...` and `go vet ./...` on go 1.23 across ubuntu/macos/windows; tests are not yet wired into CI. When adding code, prefer extending the existing table-driven tests and keeping the `make check` gate green.

## Coding Conventions

- One package (`opnborg`) at the repo root; `package main` only under `cmd/opnborg/`. Do not introduce subpackages without strong reason.
- File naming: kebab-case `topic.go`. Struct types are PascalCase; exported fields are PascalCase with inline `// comment` docs (see `OPNCall` in `api.go`).
- Constants and unexported globals are grouped at the top of the relevant file (often with `_` prefix for package-private consts like `_app`, `_lf`, `_archive`).
- Logging: never use `log`/`fmt.Println` directly inside the daemon hot paths -- send `[]byte` to the `displayChan` channel so the background `startLog` goroutine serializes output. `fmt.Println` is acceptable in `srvHttpd.go` startup error paths only because it runs before the display engine is ready.
- HTTP handlers return `http.Handler` constructed via `http.HandlerFunc` closure; apply `addSecurityHeader` middleware at `mux.Handle` registration time, not inside the handler.
- `config.dirty` is an `atomic.Bool` -- always `.Store()`/`.Load()`, never `=`.
- `hive`, `unifiStatus`, and `unifiWatchStatus` are package globals guarded by `hiveMutex`/`unifiMutex`/`unifiWatchMutex`. Mutate only under the matching lock.
- `gofmt -s` (simplify) is enforced; run `make check` before committing.
- The WebUI HTML/SVG is hand-rolled and inlined as Go `const` strings in `httpd-ui.go`. The favicon is embedded via `//go:embed resources/borg.png`.

## Gotchas

- **Duplicate `#` split logic** in `srv.go` (lines ~88 and ~136) parses `server#tag` in two places. Changes to the host/tag format must update both, plus `actionOPN`'s signature.
- **`os.Chdir` is called from a few startup paths** (`gitInit`, `gitCheckIn`, `startWeb`, `startRSysLog`). `checkIntoStore` no longer Chdirs — it resolves paths against `config.Path` — so the per-server worker goroutines do not race on the process-wide CWD. The remaining Chdir sites either run at startup (before workers) or Chdir to the same `config.Path`, so they do not observably race; still, prefer absolute paths when adding new code.
- **`make deps` deletes `go.mod` and `go.sum` and re-runs `go mod init`** -- only run it when you intend to fully refresh the module graph.
- **`SemVer` in `api.go` is the single source of truth** for the version string (CLI banner via `cmd/opnborg/main.go` + WebUI footer). goreleaser consumes git tags, not this constant. Bump it in lockstep with each release tag (see *Releasing* above).
- **CI Go version (`1.23` in `.github/workflows/golang.yml`) lags behind `go.mod` (`1.26.5`) and the Dockerfile / `release.yml` (`1.26`)**. If you adopt newer language features, verify the Docker/CI builds still pass or bump these in the same change.
- **OPNsense API does not support legacy backup endpoints** -- only `/api/core/backup/download/this` is wired in (`_apiBackupXML`). Don't fall back to alternatives without explicit requirement.
- **HTTPS chain verification via the OS trust store is disabled by default**; security relies on `OPN_TLSKEYPIN`. Documenting "just use system CAs" would be wrong.
- The `resources/` directory contains the embedded favicon (`borg.png`) and sample screenshots referenced from `README.md`; the `resources/opnborg/index.html` is the legacy static demo page, not the live UI.
- **`OPN_NOGIT` is no longer honored** — the git feature is opt-in via `OPN_GIT_ENABLE`. Stale references in older docs to `OPN_NOGIT` inverting the default are obsolete; the only env-inverted flag is `OPN_NODAEMON`.

## External Dependencies of Note

- `github.com/go-git/go-git/v5` -- pure-Go git for local repo management, commits, and SSH push to upstream.
- `github.com/cnaude/go-syslog/syslog/v3` -- RFC5424 syslog server.
- `github.com/natefinch/lumberjack` + `github.com/sirupsen/logrus` -- log rotation/formatting for the syslog sink.
- `github.com/fsnotify/fsnotify` -- filesystem watcher for the Unifi autoBackup folder sync (`srvUnifiWatch.go`).
- `github.com/pmezard/go-difflib` -- unified-diff generation for the Ollama-assisted commit message feature; `ollama.go` builds an enriched diff (commit summary + per-file metadata + widened hunks + detected OPNsense XML sections + full small-file content) on top of `go-difflib` hunks.
- `paepcke.de/uniex` -- Unifi inventory export logic (delegated to that module; opnborg only wires the env config).

Dependabot keeps these current via `.github/dependabot.yml`.
