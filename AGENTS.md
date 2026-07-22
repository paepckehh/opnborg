# AGENTS.md

Reference guide for AI agents working in the `opnborg` repository.

## Project Summary

`opnborg` is a single-binary Go daemon that backs up, monitors, and synchronizes configuration across a fleet of OPNsense firewalls and (optionally) Unifi controllers. Configuration is driven entirely through environment variables; the binary itself accepts only `-v` / `-h`. Output is emitted to stdout via an internal `displayChan` log engine, and an embedded HTTP server renders the hive status as HTML. Backups are stored as XML on disk, deduplicated by SHA-256, and (optionally) committed to a local git repo.

- Module path: `paepcke.de/opnborg`
- Go version: see `go.mod` (currently `1.26.4`); CI pins `1.23`, Dockerfile pins `1.24`
- Entry point: `cmd/opnborg/main.go` -> `opnborg.Start(config)` -> `srv(config)`
- License: BSD 3-Clause

## IMPORTANT FOR EVERY SINGLE TASK: NEVER SKIP THIS ACTIONS

- Test every change via build and unit tests
- commit each task into git repo when done
- **Every committed task must be tagged with a git semver tag**, bumping only
  the **patch** segment, the last segemnt (e.g. `v0.0.22` → `v0.0.23` → `v0.0.24`). 
  The other two first segments (major, minor) stays at any cost at zero, `v0.0.xx`
  increment only the xx part. Never move, delete, or rewrite an existing tag.

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

# Lint / format (top-level Makefile target)
make check        # runs gofmt -w -s, go vet, go fix (root + cmd/opnborg)

# Dependency refresh (DESTRUCTIVE - rewrites go.mod/go.sum)
make deps

# Install
go install paepcke.de/opnborg/cmd/opnborg@main
```

`make check` is the canonical pre-PR gate. `staticcheck` is intentionally commented out in both Makefiles; do not re-enable it without checking the existing code baseline (the project currently emits `writestring` and `unusedparams` diagnostics by design).

### Releasing

- Releases are tag-driven (`v*`). Pushing a `v*` tag triggers both `.github/workflows/release.yml` (goreleaser cross-compile via `.goreleaser.yml`) and `.github/workflows/ghcr.yml` (build/push `ghcr.io/paepckehh/opnborg:latest`).
- goreleaser builds target linux/freebsd/darwin/netbsd/openbsd/windows on amd64 + arm64, `CGO_ENABLED=0`.
- **Before tagging a release**, bump the `SemVer` constant in `api.go` AND the hardcoded API version reminder in the top-level `Makefile` (`make deps` prints a reminder to do this). `SemVer` is consumed both by the CLI startup banner and the WebUI footer.

## Architecture & Control Flow

All package files live at the repository root (`/`) under `package opnborg`. `cmd/opnborg/main.go` is the only file in `package main`.

### Startup sequence

1. `main.go` parses only `-v` / `-h`; anything else is a fatal error. All other configuration is ENV-only.
2. `opnborg.Setup()` (`setup.go`) loads `.env` (via `godotenv`), reads every `OPN_*` env var, sanitizes, and returns a populated `*OPNCall` config struct (defined in `api.go`).
3. `opnborg.Start(config)` -> `srv(config)` (`srv.go`) is the orchestrator.

### `srv()` main loop (`srv.go`)

- Spins up goroutines for: display/log engine (`startLog`), background timer, internal HTTP server (`startWeb`), RFC5424 syslog server (`startRSysLog`), Unifi backup (`srvUnifiBackup`), and Unifi asset export (`srvUnifiExport`), each guarded by its respective `config.*.Enable` flag.
- Builds the `servers` slice by splitting `config.Targets` on commas; each entry may carry an asset tag separated by `#` (e.g. `opn01.lan#edge-1`). The split-on-`#` switch is duplicated in `srv()` (status init) and the main loop (worker dispatch) -- keep them in sync.
- Main loop body per tick:
  1. `gitCheckIn(config)` to reset the worktree state and clear `config.dirty`.
  2. If `config.Sync.Enable`: `readMasterConf(config)` pulls the master OPNsense XML and derives the package list (`sync-master.go`).
  3. For each server: `go actionOPN(...)` with a `sync.WaitGroup`; `wg.Wait()` blocks until the whole hive finishes a pass.
  4. If `config.dirty.Load()` (set atomically by workers when they wrote new XML), run `gitCheckIn` again.
  5. In daemon mode, block on `<-updateOPN` (tick channel from the background timer, or poked by the `/force` HTTP handler). In one-shot mode (`OPN_NODAEMON`), close `displayChan`, wait, and return.

### Per-server backup (`actionOPN.go`)

1. If sync or syslog is enabled, `fetchOPN(server, config)` pulls + XML-unmarshals the live config into an `*Opnsense` (`struct-xml-opn.go`).
2. `checkInstallPKG` (only on non-master hosts, `sync-pkg.go`) diffs the host's installed plugins against the master package list and calls `installPKG` per missing package.
3. `checkRSysLogConfig` (`rsyslog-clientconf.go`) ensures the remote-syslog client config matches.
4. `fetchXML` (`transport.go`) downloads the actual backup XML via `/api/core/backup/download/this` with HTTP basic auth.
5. SHA-256 the new XML; compare to the previous `CONFIG-CURRENT` (`store.go::lastSum`). If unchanged, mark status and skip storage.
6. On change: `checkIntoStore` writes the timestamped archive file, rotates the `current.xml` / `last.xml` symlinks, and sets `config.dirty.Store(true)` so the main loop will commit.

### HTTP WebUI (`srvHttpd.go`, `httpd-handler.go`, `httpd-ui.go`, `httpd-transport.go`)

- `mux`: `/` (index), `/files/` (static file server rooted at `config.Path`), `/force` (manual trigger), `/favicon.ico`, optional `/git` (go-git-http smart HTTP server, enabled by `OPN_GITSRV`).
- Index handler renders HTML built from inlined SVG/HTML constants in `httpd-ui.go`. `_head`, `_forceRedirect` are assembled at `Setup()` time from `OPN_HTTPD_COLOR_FG` / `OPN_HTTPD_COLOR_BG`.
- Status strings (`_ok`, `_fail`, `_na`, `_degraded`, `_unifi`) are inline animated SVGs defined as `const` in `httpd-ui.go`. `status.go` mutates the `hive` / `unifiStatus` strings under `hiveMutex` / `unifiMutex`.
- A `addSecurityHeader` middleware wraps index and `/files/`.
- TLS is opt-in via `OPN_HTTPD_CACERT` + `OPN_HTTPD_CAKEY`; setting `OPN_HTTPD_CACLIENT` enables mTLS enforcement (`httpd-transport.go::getHTTPTLS`).

### OPNsense API endpoints (`transport.go`)

Hardcoded under the `_api*` consts:
- `/api/core/backup/download/this` - XML backup fetch (no legacy endpoint support)
- `/api/core/firmware/status/` - firmware version JSON (`struct-json-firmwareStatus.go`)
- `/api/core/firmware/install/<pkg>` - plugin install (POST)

HTTPS is mandatory; the client intentionally skips OS trust store verification and relies on `OPN_TLSKEYPIN` (SHA-256 base64 of the SPKI) for MitM-proofing. See `getTlsConf` / `getTransport` in `httpd-transport.go`.

## Configuration Conventions (ENV)

- **Boolean env vars are presence-based, not value-based**: setting `OPN_DEBUG=0`, `OPN_DEBUG=false`, or `OPN_DEBUG=1` all evaluate to `true` via `isEnv()` (`littlehelper.go`) as long as the value is non-empty. To disable, unset the var. The only exception is the empty string, which `isEnv` treats as false.
- `OPN_NODAEMON` and `OPN_NOGIT` invert the default (both default to `true` when unset): `Daemon = !isEnv("OPN_NODAEMON")`, `Git = !isEnv("OPN_NOGIT")`.
- Either OPN backup (`OPN_APIKEY` + `OPN_APISECRET`) or Unifi backup (`OPN_UNIFI_BACKUP_USER` + `OPN_UNIFI_BACKUP_SECRET` + `OPN_UNIFI_VERSION`) must be configured or `Setup()` returns a fatal error.
- `OPN_TARGETS` is comma-separated. Each host may append `#<asset-tag>`. Custom groups use `OPN_TARGETS_<GROUPNAME>` with the same syntax; `OPN_TARGETS_DESC_<GROUPNAME>` supplies the group's WebUI text description.
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
    current.xml                      # symlink to the latest archive file
    CONFIG-LAST                      # previous checksum source
    CONFIG-CURRENT                   # current checksum source (read by lastSum)
    .archive/<YYYY>/<MM>/<RFC3339TS>-<server>.xml
    sha256.db
  Logs/current.log                   # rotated by lumberjack (256MB, 256 backups, 180d, gzipped)
```

`gitCheckIn` does `os.Chdir(config.Path)` then `git.PlainOpen`/`git.PlainInit` (SHA256 object format on new repos), `wtree.Add(".")`, and commits as `OPNBORG-AUTO-COMMIT <git@opnborg>`. `OPN_GITPUSH` enables pushing to a configured upstream. Multiple call sites `os.Chdir` into subdirectories -- be careful when adding new code that assumes a stable CWD; restore or re-`Chdir` as needed.

## Testing

There is currently **no test suite** in the repository (no `*_test.go` files). CI runs only `go build ./...` and `go vet ./...` (see `.github/workflows/golang.yml`). When adding code, prefer keeping the `make check` gate green over introducing a new test framework unless asked.

## Coding Conventions

- One package (`opnborg`) at the repo root; `package main` only under `cmd/opnborg/`. Do not introduce subpackages without strong reason.
- File naming: kebab-case `topic.go`. Struct types are PascalCase; exported fields are PascalCase with inline `// comment` docs (see `OPNCall` in `api.go`).
- Constants and unexported globals are grouped at the top of the relevant file (often with `_` prefix for package-private consts like `_app`, `_lf`, `_archive`).
- Logging: never use `log`/`fmt.Println` directly inside the daemon hot paths -- send `[]byte` to the `displayChan` channel so the background `startLog` goroutine serializes output. `fmt.Println` is acceptable in `srvHttpd.go` startup error paths only because it runs before the display engine is ready.
- HTTP handlers return `http.Handler` constructed via `http.HandlerFunc` closure; apply `addSecurityHeader` middleware at `mux.Handle` registration time, not inside the handler.
- `config.dirty` is an `atomic.Bool` -- always `.Store()`/`.Load()`, never `=`.
- `hive` and `unifiStatus` are package globals guarded by `hiveMutex`/`unifiMutex`. Mutate only under the lock.
- `gofmt -s` (simplify) is enforced; run `make check` before committing.
- The WebUI HTML/SVG is hand-rolled and inlined as Go `const` strings in `httpd-ui.go`. The favicon is embedded via `//go:embed resources/borg.png`.

## Gotchas

- **Duplicate `#` split logic** in `srv.go` (lines ~88 and ~136) parses `server#tag` in two places. Changes to the host/tag format must update both, plus `actionOPN`'s signature.
- **`os.Chdir` is called from multiple goroutines** (`gitCheckIn`, `checkIntoStore`, `startWeb`, `startRSysLog`). The process-wide working directory is a shared resource. Workers run concurrently per server; rely on `config.Path` for absolute pathing and minimize `Chdir`.
- **`make deps` deletes `go.mod` and `go.sum` and re-runs `go mod init`** -- only run it when you intend to fully refresh the module graph.
- **`status.go::setUnifiStatus` has an unused `notice` parameter** (gopls `unusedparams` diagnostic). This is known and currently intentional; do not "fix" it without checking call sites.
- **`SemVer` in `api.go` is the single source of truth** for the version string (CLI banner + WebUI footer + goreleaser consumes git tags, not this constant). Bump it in lockstep with the release tag.
- **Dockerfile Go version (`1.24`) and CI Go version (`1.23`) lag behind `go.mod` (`1.26.4`)**. If you adopt newer language features, verify the Docker/CI builds still pass or bump these in the same change.
- **OPNsense API does not support legacy backup endpoints** -- only `/api/core/backup/download/this` is wired in (`_apiBackupXML`). Don't fall back to alternatives without explicit requirement.
- **HTTPS chain verification via the OS trust store is disabled by default**; security relies on `OPN_TLSKEYPIN`. Documenting "just use system CAs" would be wrong.
- The `resources/` directory contains the embedded favicon (`borg.png`) and sample screenshots referenced from `README.md`; the `resources/opnborg/index.html` is the legacy static demo page, not the live UI.

## External Dependencies of Note

- `github.com/go-git/go-git/v5` -- pure-Go git for local repo management and commits.
- `github.com/AaronO/go-git-http` -- smart HTTP git server mounted at `/git` when `OPN_GITSRV` is set.
- `github.com/alecthomas/chroma/v2` -- syntax highlighting for the WebUI (referenced from `git.go`).
- `github.com/cnaude/go-syslog/syslog/v3` -- RFC5424 syslog server.
- `github.com/natefinch/lumberjack` + `github.com/sirupsen/logrus` -- log rotation/formatting for the syslog sink.
- `paepcke.de/uniex` -- Unifi inventory export logic (delegated to that module; opnborg only wires the env config).

Dependabot keeps these current via `.github/dependabot.yml`.
