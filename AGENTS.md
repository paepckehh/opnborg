# AGENTS.md

Reference guide for AI agents working in the `opnborg` repository.

> ## FIXED REQUIREMENT — EVERY CHANGE, NO EXCEPTIONS
>
> Before a task or change is considered done, all steps below MUST be completed
> in this exact order. Skipping or reordering any step is a failure.
>
> 1. **Format** — `gofmt -w .` (or `make check`)
> 2. **Build** — `CGO_ENABLED=0 go build -o opnborg ./cmd/opnborg` must succeed.
>    Always use parallel package builds (the Go toolchain default); never pass `-p 1`.
> 3. **Test** — `go test -count=1 ./...` must pass. Always use parallel test
>    execution (the default); never pass `-p 1`. To iterate on a single failing
>    package, narrow the package list (e.g. `go test ./pkg/...`).
> 4. **Commit** — `git add . && git commit -m '<message>'`
> 5. **Tag** — bump the patch segment only: `v0.1.<N+1>` (latest tag is `v0.1.207`).
>    Never move, delete, or reuse an existing tag. Also bump the `SemVer` constant
>    in `api.go` to match the new tag.
> 6. **Push** — always push commits and tags.


## Project Summary

`opnborg` is a single-binary Go daemon that backs up, monitors, and synchronizes configuration across a fleet of OPNsense firewalls and (optionally) Unifi controllers. Configuration is driven entirely through environment variables; the binary accepts only `-v` / `-h`. Output goes to stdout via an internal `displayChan` log engine, and an embedded HTTP server renders the hive status as HTML. Backups are stored as XML on disk, deduplicated by SHA-256, and (optionally) committed to a local git repo.

- Module path: `paepcke.de/opnborg`
- Go version: see `go.mod` (currently `1.26.5`); CI uses `go-version-file: go.mod`
- Entry point: `cmd/opnborg/main.go` -> `opnborg.Start(config)` -> `srv(config)`
- License: BSD 3-Clause

## Build, Run, Check

```sh
CGO_ENABLED=0 go build ./...                          # build the whole module (CI)
CGO_ENABLED=0 go build -ldflags="-w -s" ./cmd/opnborg  # build just the binary
go run cmd/opnborg/main.go                             # run locally (after sourcing env config)
go run paepcke.de/opnborg/cmd/opnborg@main             # run via remote module
make check        # gofmt -l ., go vet ./..., go mod tidy -diff
make test         # go test -race -count=1 ./...
make deps         # DESTRUCTIVE — rewrites go.mod/go.sum
go install paepcke.de/opnborg/cmd/opnborg@main
```

`make check` is the canonical pre-PR gate. No `staticcheck` wiring exists. `-race` requires CGO; on CGO-disabled toolchains use `go test -count=1 ./...`.

### Releasing

- Releases are tag-driven (`v*`). Pushing a `v*` tag triggers `.github/workflows/release.yml` (goreleaser cross-compile via `.goreleaser.yml`) and `.github/workflows/ghcr.yml` (build/push `ghcr.io/paepckehh/opnborg:latest`).
- goreleaser targets linux/freebsd/darwin/netbsd/openbsd/windows on amd64 + arm64, `CGO_ENABLED=0`.
- **Before tagging**, bump `SemVer` in `api.go` to match the new tag. `SemVer` is the single source of truth for the version string (CLI banner + WebUI footer). The Makefile's `-ldflags -X paepcke.de/opnborg/internal/version.Version=...` is a no-op (no `internal/version` package exists).

## Architecture & Control Flow

All package files live at the repository root (`/`) under `package opnborg`. `cmd/opnborg/main.go` is the only file in `package main`.

### Startup sequence

1. `main.go` parses only `-v` / `-h`; anything else is a fatal error. All other configuration is ENV-only.
2. `opnborg.Setup()` (`setup.go`) loads `.env` (via `godotenv`), reads every `OPN_*` env var, sanitizes, and returns a populated `*OPNCall` config struct (defined in `api.go`).
3. `opnborg.Start(config)` -> `srv(config)` (`srv.go`) is the orchestrator.

### `srv()` main loop (`srv.go`)

- Spins up goroutines for: display/log engine (`startLog`), background timer, internal HTTP server (`startWeb`), RFC5424 syslog server (`startRSysLog`), Unifi backup (`srvUnifiBackup`), Unifi asset export (`srvUnifiExport`), and Unifi autoBackup folder watch (`srvUnifiWatch`), each guarded by its respective `config.*.Enable` flag.
- When `config.Git.Enable`, `gitInit(config)` runs once at startup (before the first worker pass) to open-or-init the storage repo and write `.gitignore`.
- Builds the `servers` slice by splitting `config.Targets` on commas; each entry may carry an asset tag separated by `#` (e.g. `opn01.lan#edge-1`). The host/tag split is centralised in `parseServerTag` (`srv.go`), used by both the hive status-tile init and the worker dispatch.
- Main loop body per tick:
  1. Reset `config.dirty` to false.
  2. If `config.Sync.Enable`: `readMasterConf(config)` pulls the master OPNsense XML and derives the package list (`sync-master.go`).
  3. For each server: `go actionOPN(...)` with a `sync.WaitGroup`; `wg.Wait()` blocks until the whole hive finishes a pass.
  4. If `config.dirty.Load()` (set atomically by workers when they wrote new XML), run `gitCheckIn` (commit + optional push).
  5. In daemon mode, block on `<-updateOPN` (tick channel from the background timer, or poked by the `/force` HTTP handler via a non-blocking select). In one-shot mode (`OPN_NODAEMON`), close `displayChan`, wait, and return.

### Per-server backup (`actionOPN.go`)

1. If sync or syslog is enabled, `fetchOPN(server, config)` pulls + XML-unmarshals the live config into an `*Opnsense` (`struct-xml-opn.go`) and returns the raw XML payload, which `actionOPN` reuses for the check-in below (single download per pass).
2. `checkInstallPKG` (only on non-master hosts, `sync-pkg.go`) diffs the host's installed plugins against the master package list and calls `installPKG` per missing package.
3. `checkRSysLogConfig` (`rsyslog-clientconf.go`) ensures the remote-syslog client config matches.
4. `fetchXML` (`transport.go`) downloads the actual backup XML via `/api/core/backup/download/this` with HTTP basic auth.
5. SHA-256 the new XML; compare to the previous `CONFIG-CURRENT` (`store.go::lastSum`). If unchanged, mark status and skip storage.
6. On change: `checkIntoStore` writes the timestamped archive file, rotates the `current.<ext>` / `CONFIG-CURRENT` / `CONFIG-LAST` pointers, and sets `config.dirty.Store(true)` so the main loop will commit. `checkIntoStore` resolves all paths against `config.Path` and does **not** call `os.Chdir`.

### HTTP WebUI (`srvHttpd.go`, `httpd-handler.go`, `httpd-ui.go`, `httpd-transport.go`, `progress.go`, `auth.go`, `auth-http.go`)

- **Routes**: `/` (index, open), `/config` (config dashboard, open), `/audit` (BorgAUDIT commit-history page, open), `/progress` (forced-backup progress, open), `/files/` (static file server rooted at `config.Path`, admin-gated via `requireAdminFiles` — never a pass-through, returns 403 when no credentials configured), `/force` (manual trigger, open by design — it only arms an already-scheduled backup pass), `/approve` (single-commit approval toggle, admin-gated via `requireAdmin`), `/approve-all` (bulk approve, admin-gated via `requireAdmin`), `/auth/login` (admin login POST), `/auth/logout` (session revoke POST), `/auth/state` (mode + lock JSON polled by nav-bar JS), `/auth-hash` (credential generator bootstrap page), `/favicon.ico`.
- **Middleware**: page-render routes (`/`, `/config`, `/audit`, `/progress`, `/files/`) are wrapped with `addSecurityHeader`. `/files/`, `/approve`, and `/approve-all` are additionally wrapped with `requireAdmin` (or `requireAdminFiles` for the static file server). Page-render routes (`/`, `/config`, `/audit`, `/progress`) are always open — sensitive sub-features (diffs, download buttons, approve controls) are locked in the render path when no admin session is present, regardless of whether credentials are configured. When credentials are armed and no live admin session exists, `requireAdmin` redirects to `./?auth=locked` and `requireAdminFiles` redirects to `config?auth=locked`. When no credentials are configured, `requireAdminFiles` returns 403 Forbidden — it is never a pass-through.
- Index handler renders HTML built from inlined SVG/HTML constants in `httpd-ui.go`. `_head`, `_forceRedirect` are assembled at `Setup()` time from `OPN_HTTPD_COLOR_FG` / `OPN_HTTPD_COLOR_BG`. Status strings (`_ok`, `_fail`, `_na`, `_degraded`, `_unifi`) are inline animated SVGs; `status.go` mutates `hive` / `unifiStatus` / `unifiWatchStatus` under their respective mutexes.
- The `/force` handler pokes `updateOPN` / `updateUnifiBackup` / `updateUnifiExport` / `updateUnifiWatch` channels with non-blocking selects (buffer-1 channels): if a pass is already pending it drops the duplicate. It also bumps `forceSeq` (`progress.go`) so the animated progress dashboard knows a fresh forced pass is armed.
- **Forced-backup progress dashboard** (`progress.go`): every display-engine line is tee'd into a fixed-size ring buffer (`_progressCap`, 512 lines) guarded by `progressMu`. The `/progress` handler streams captured lines as JSON; the page redirects back to the hive view once the forced pass ends. `forceSeq` / `passSeq` / `busy` are lock-free `atomic` counters.
- **AI review-in-progress banner** (`httpd-handler.go`): while `reviewPending` (`api.go`, an `atomic.Bool`) is set during model-assisted commit generation (`git.go::gitCommit`), the index page renders an animated banner so the operator knows a commit is pending.
- The httpd is armed only in daemon mode and only when `OPN_HTTPD_DISABLE` is unset.
- TLS is opt-in via `OPN_HTTPD_CACERT` + `OPN_HTTPD_CAKEY`; setting `OPN_HTTPD_CACLIENT` enables mTLS enforcement (`httpd-transport.go::getHTTPTLS`).

### WebUI authentication — two-mode access model (`auth.go`, `auth-http.go`)

> ## SECURITY REQUIREMENT — NO EXCEPTIONS
>
> **Any new user access session starts unauthenticated in monitoring view-only mode.**
> In unauthenticated monitoring mode, `current.xml`, `current.unf`, and the config
> archive (`.archive/`) are **never** accessible — not via the `/files/` HTTP route,
> not via download buttons on the index page, not via any render path. Access to
> these files and data is granted to **authenticated admin sessions only**.
> This applies under all conditions:
>
> - When credentials are armed but no admin session is live: `requireAdminFiles`
>   redirects to `config?auth=locked`; `renderDownloadButton` renders locked
>   controls; audit diffs and approval controls are locked.
> - When no credentials are configured at all: `requireAdminFiles` returns **403
>   Forbidden** (no login path to redirect to); `renderDownloadButton` renders
>   locked controls with setup instructions; audit diffs and approval controls
>   are locked.
>
> **Never** make `requireAdminFiles`, `renderDownloadButton`, or the audit
> approval/diff render path a pass-through for unauthenticated sessions. A
> regression test (`TestConfigFilesNeverAccessibleWithoutAdminSession`) guards
> this invariant.

The WebUI page-render routes (`/`, `/config`, `/audit`, `/progress`) are always viewable without authentication. The `[ Authenticate ]` nav-bar button unlocks admin mode per browser session, which enables sensitive sub-features (config-file downloads, audit diff details, approve actions, forced backup trigger) that are greyed out in monitoring mode.

- **Monitoring mode** (green `MONITORING` badge): all pages (`/`, `/config`, `/audit`, `/progress`) are readable. Config-file downloads (`/files/`), forced backup trigger (`/force`), and approval actions (`/approve`, `/approve-all`) are locked — `requireAdmin`/`requireAdminFiles` redirect unauthenticated requests to `./?auth=locked` (or `config?auth=locked` for `/files/`), or return 403 Forbidden when no credentials are configured. On the audit page, diff details are hidden behind a greyed-out placeholder; approve buttons render as locked hints. Download buttons on the index page render as greyed-out locked controls that open the auth info dialog on click.
- **Admin mode** (red/yellow `ADMIN` badge): per-browser-session state unlocked by submitting the admin password via the nav-bar dialog; enables config downloads, audit page with full diffs, forced backup, and approval actions.
- **Startup validation** (`authInit`, called from `Setup()`): login is armed only when `OPN_AUTH_HASH` + `OPN_AUTH_SALT` are both present, non-empty, non-whitespace, and valid — the hash must be a base64-encoded key decoding to exactly 64 bytes (keylen); the legacy `<base64-key>$<base64-key>` format (key encoded twice) is also accepted for backward compatibility. The salt must base64-decode to >= 8 raw bytes. Invalid content keeps auth fully disabled (`[AUTH][DISABLED]`), the login dialog is never rendered, and `authCheckPassword` refuses every attempt. Even in this state `requireAdminFiles` returns 403 for any non-admin request — config files are never served without authentication.
- **Middleware** (`auth-http.go`): `requireAdmin(next)` checks `authCredentialsEnabled()` and `authIsAdmin(r)` — if credentials are armed but no live admin session exists, the request is redirected to `./?auth=locked` (HTTP 303). `requireAdminFiles(next)` checks `authIsAdmin(r)` first — if the request carries a live admin session, the file is served; otherwise, when credentials are armed it redirects to `config?auth=locked`, and when no credentials are configured it returns **403 Forbidden**. `requireAdminFiles` is **never** a pass-through: sensitive config files (`current.xml`, `current.unf`, archive) must never be served to an unauthenticated session under any condition. Page-render routes are NOT wrapped with these middleware — they are always open.
- **`adminEnabled` atomic** (`api.go`/`auth.go`): mirrors the live admin-session state for the render path. `authCheckPassword` sets it `true` on successful login; `authLogout` sets it `false` when the last session is revoked. `renderDownloadButton` (`status.go`) reads it to decide whether to render an active download link or a greyed-out locked control.
- **Sessions**: successful login mints a 32-byte random token as an HttpOnly SameSite=Strict cookie (`opnborg_auth`, TTL 12 h sliding). `authIsAdmin(q)` is the render-path gate used by `requireAdmin` and `renderDownloadButton`.
- **Global lockout**: failed logins bump a process-global counter shared across ALL sessions — 10 s after the 1st failure, doubling with each further failure (10s, 20s, 40s, ...). Resets on successful login, daemon restart, or 6 h of inactivity; countdown shown live in nav bar (polled from `/auth/state`).
- **Credential generator** (`/auth-hash`): enter a password; opnborg derives `OPN_AUTH_HASH` (a single base64-encoded 64-byte Argon2id key; the legacy `<base64-key>$<base64-key>` format is also accepted for backward compatibility) and `OPN_AUTH_SALT` (16 raw random bytes) with Argon2id (time=8, memory=64 MiB, threads=1, keylen=64). The password is never stored or logged. The generator is always available — even when credentials are already armed — and the page makes it absolutely clear that the displayed values are display-only: the operator must manually copy both env vars into the opnborg environment and restart the daemon for them to take effect. The Authentication tile on the config dashboard (`renderAuthPanel`) always shows the `[ Create Authentication Env Vars ]` button linking to the generator, regardless of whether credentials are currently armed.
- Verification is constant-time (`subtle.ConstantTimeCompare`); all state transitions logged to `displayChan` with `[AUTH]` tags.

### BorgAUDIT — git commit history review (`audit.go`, `approval.go`, `approval-http.go`)

The `BorgAUDIT` tile and `/audit?range=` page expose the storage repo git history for operator review — the human-facing half of the AI security-audit workflow (the AI half is the `tag:` severity line in model-authored commit messages). The tile is only emitted when `OPN_GIT_ENABLE` is set. Each entry is a collapsible card with commit hash, author, date, file-change stats, full commit message (including `tag: <severity>[, needs-review]` line when model-authored), the full unified diff against the first parent (capped at `_auditDiffCap`, 512 KB), and a `change-performed-by:` line surfacing the admin account that authored the config change (extracted via `extractPerformerFromDiff`). `gatherAuditCommits` walks the log bounded by `_auditCap` (250 commits), opening the repo directly against `config.Path` (no `os.Chdir`). Five windows via `?range=` (default `24h`): `24h`, `7d`, `1m`, `3m`, `6m`. A `renderAuditThreatDashboard` summary renders an interactive threat-level categorisation map at the top of the page.

### Security-approval ledger (`approval.go`, `approval-http.go`)

A single on-disk SQLite database (`approval.db`, co-located with `config.Path`) tracks every opnborg-authored git commit whose security-impact tag is above `low`/`none`/`backup` (i.e. `medium` / `high` / `critical`). For each tracked commit the ledger records the full git hash, severity, headline, timestamp, and an approval state toggleable from the BorgAUDIT page. Toggling to approved records the wall-clock timestamp, source IP, `X-Forwarded-For` chain, and `Remote-User` identity of the operator.

- Uses `modernc.org/sqlite` (pure-Go, CGO-free) so the binary stays `CGO_ENABLED=0`.
- `approvalDB` is a package-global `*sql.DB` opened from `gitInit` (`git.go`) after repo init, and lazily from the httpd. `approvalBackfillFromHistory` walks the existing commit log on a fresh store.
- `approvalTrackCommit` is called from `gitCommit` after every commit (idempotent insert). `syncAuditCommitsToLedger` reconciles the ledger with the audit page.
- The `.gitignore` carries three explicit entries — `approval.db`, `approval.db-wal`, `approval.db-shm` — so the ledger database and its SQLite WAL sidecars are never tracked. `gitEnsureIgnore` reconciles this on every startup (and every `gitCheckIn`), and `gitCommit` skips any path containing `approval.db` at staging time (`isApprovalDBPath`) so a ledger update can never enter a commit even when a stale or hand-edited `.gitignore` failed to ignore the file. A regression test (`TestApprovalDBNeverCommitted`) guards this invariant.
- `POST /approve?hash=<hash>&range=<range>` and `POST /approve-all?range=<range>` toggle approval state and redirect to the audit page. Both are POST-only and wrapped with `requireAdmin` so an unauthenticated client cannot approve commits.

### OPNsense API endpoints (`transport.go`)

Hardcoded under the `_api*` consts:
- `/api/core/backup/download/this` — XML backup fetch (no legacy endpoint support)
- `/api/core/firmware/status/` — firmware version JSON (`struct-json-firmwareStatus.go`)
- `/api/core/firmware/install/<pkg>` — plugin install (POST)

HTTPS is mandatory; the client intentionally skips OS trust store verification and relies on `OPN_TLSKEYPIN` (SHA-256 base64 of the SPKI) for MitM-proofing. See `getTlsConf` / `getTransport` / `opnClient` in `transport.go` (not `httpd-transport.go`, which only holds the WebUI listener `getHTTPTLS`).

## Configuration Conventions (ENV)

- **Boolean env vars are presence-based, not value-based**: setting `OPN_DEBUG=0`, `OPN_DEBUG=false`, or `OPN_DEBUG=1` all evaluate to `true` via `isEnv()` (`littlehelper.go`). To disable, unset the var.
- `OPN_NODAEMON` inverts the daemon default (`Daemon = !isEnv("OPN_NODAEMON")`). In one-shot mode the httpd, rsyslog server, and Unifi goroutines are not armed.
- **Git repo** is opt-in via `OPN_GIT_ENABLE` (presence-based). `OPN_GIT_UPSTREAM` sets an upstream SSH git URL; `OPN_GIT_SSH_KEY` points at the PEM private key; `OPN_GIT_SSH_HOSTKEY` sets the upstream's SHA-256 fingerprint (`SHA256:base64...`) for host key verification — when unset, host key verification is skipped (`InsecureIgnoreHostKey`). All validated in `validateGitConfig` (`git.go`). (`OPN_NOGIT` is no longer honored.)
- **Ollama-assisted commit messages** (opt-in via `OLLAMA_DESC_URL` + `OLLAMA_DESC_MODEL`). Before each non-`.unf` commit, an enriched diff (commit summary + per-file metadata + widened hunks + detected OPNsense XML sections + full small-file content, capped at 256 KB) is POSTed to `<OLLAMA_DESC_URL>/api/generate` (`stream=false`, 240 s timeout, 5 retries, 2 s backoff). The model's verbatim response becomes the commit message: a short headline, a brief security summary, and a trailing `tag: <severity>[, needs-review]` line. `.unf`-only commits and any model failure fall back to the default message (`opnborg auto update`). Implemented in `ollama.go`, wired into `gitCommit` (`git.go`).
- **OpenAI-compatible fallback** (opt-in via `OPENAPI_DESC_URL`, required; `OPENAPI_DESC_MODEL`, optional, defaults to `gpt-4o-mini`; `OPENAPI_DESC_TOKEN`, optional). When Ollama is unset or fails, the same enriched diff and prompt go to `<OPENAPI_DESC_URL>/chat/completions` (two-message chat, `stream=false`). Token sent as `Authorization: Bearer <token>` when set. Same retry/fallback contract via shared `generateWithRetry`. The `reviewPending` atomic banner is armed when either backend is enabled.
- **AI security-audit `tag:` line**: the system prompt instructs the model to classify each commit's security impact as `low` (routine), `medium` (bounded hardening/exposure change), `high` (broadens attack surface), or `critical` (removes a key control), optionally appending `, needs-review` for high/critical severity or any auth/cert/IPsec/firewall-defaults change. opnborg writes this verbatim into the commit message and surfaces it on the BorgAUDIT page — it does not parse or act on the tag.
- The internal httpd is armed only in daemon mode and only when `OPN_HTTPD_DISABLE` is unset.
- **WebUI authentication** (`OPN_AUTH_HASH` + `OPN_AUTH_SALT`, both required together): when set and valid, admin mode unlocks per browser session. When missing/invalid, auth is fully disabled — the WebUI stays in monitoring-only mode. See the *WebUI authentication* subsection above.
- Either OPN backup (`OPN_APIKEY` + `OPN_APISECRET`) or Unifi backup (`OPN_UNIFI_BACKUP_USER` + `OPN_UNIFI_BACKUP_SECRET` + `OPN_VERSION`) must be configured or `Setup()` returns a fatal error.
- `OPN_TARGETS` is comma-separated. Each host may append `#<asset-tag>`. Custom groups use `OPN_TARGETS_<GROUPNAME>`; `OPN_TARGETS_DESC_<GROUPNAME>` supplies the group's WebUI text description, `OPN_TARGETS_IMGURL_<GROUPNAME>` a custom image URL (description becomes the tooltip).
- `OPN_TARGETS` / `OPN_MASTER` entries must include a port suffix if not `:443`. Clear-text HTTP is unsupported.
- `.env` is auto-loaded by `godotenv.Load()` if present at the working directory.
- Example env templates: `example.sh`, `example-env-config-simple.sh`, `example-env-config-complex.sh`, `example-env-config-unifi.sh`, `example-env-config-dev.sh`.

See `README.md` for the exhaustive env var reference.

## On-disk Storage Layout (`store.go`, `git.go`)

For a configured `OPN_PATH` (default `.`):

```
<OPN_PATH>/
  .gitignore                         # auto-created: ignores .archive, CONFIG*, Logs, approval.db, approval.db-wal, approval.db-shm
  <server>/
    current.xml                      # regular file holding the latest backup XML (served by the WebUI)
    CONFIG-CURRENT                   # symlink to the latest .archive entry (read by lastSum for SHA-256 compare)
    CONFIG-LAST                      # previous CONFIG-CURRENT symlink (renamed on each rotation)
    .archive/<YYYY>/<MM>/<YYYYMMDDTHHMMSSZ>-<server>.xml
    sha256.db                        # append-only log: <archive-name>\t<base64-sha256>
  Logs/current.log                   # rotated by lumberjack (256 MB, 256 backups, 180 d, gzipped)
```

`git.go` manages the storage folder as a git repo (opt-in via `OPN_GIT_ENABLE`). `gitInit` opens-or-inits the repo and reconciles `.gitignore` at startup. `gitCheckIn` runs per tick when `config.dirty` is set: `os.Chdir(config.Path)`, status fast-path, path-by-path staging (skipping approval-ledger files via `isApprovalDBPath`), commit. The commit author is `_authorName` (`OPNBORG-AUTO-COMMIT`) unless a model authored the message, in which case `authorFromCommitMessage` (`ollama.go`) uses a sanitised version of the headline. `gitPush` pushes via `ssh.NewPublicKeysFromFile` from `OPN_GIT_SSH_KEY`, with host key verification via `OPN_GIT_SSH_HOSTKEY` fingerprint matching (or `InsecureIgnoreHostKey` when unset). `gitGC` runs a best-effort per-tick aggressive repack (`pack.deltaWindow` raised to `_aggressivePackWindow`, 250) consolidating loose objects into a fresh packfile — the native `go-git` equivalent of `git gc --aggressive`. Pruning of unreachable loose objects is deliberately omitted: go-git's `Prune` implementation is prone to errors on repos with rotated packfiles, and the repack alone already consolidates every reachable loose object. All git operations use `go-git` — no external `git` binary.

`checkIntoStore` (`store.go`) resolves every path against `config.Path` and does **not** call `os.Chdir`, so it is safe from the concurrent per-server worker goroutines. The `CONFIG-CURRENT` / `CONFIG-LAST` symlinks use a relative target so the store tree stays portable. The remaining `os.Chdir` call sites are `gitInit` / `gitCheckIn` (both Chdir to `config.Path`) and `startWeb` / `startRSysLog` (startup only, before workers run).

### Config dashboard (`config-dashboard.go`, `dashboard.go`)

- `renderOllamaPanel` shows parsed `OLLAMA_DESC_URL` / `OLLAMA_DESC_MODEL` plus a live probe (`ollamaHealthCheck`) that GETs `<OLLAMA_DESC_URL>/api/tags` (3 s timeout) and reports server reachable, REST API ready, and model ready (matched by exact name or `<model>:<tag>` prefix).
- `renderOpenAIPanel` shows parsed `OPENAPI_DESC_URL` / `OPENAPI_DESC_MODEL` / `OPENAPI_DESC_TOKEN` plus a live probe (`openaiHealthCheck`) that GETs `<OPENAPI_DESC_URL>/models`. Token shown as a set/not-set pill.
- `renderAuthPanel` shows the current auth mode, masked credential state, KDF parameters, and links to the generator when unarmed.
- `gatherDashboard` (`dashboard.go`) describes the on-disk backup store, local git repo, and upstream sync health (gathered on every dashboard render; `countDivergence` walks reachable hashes for ahead/behind counts).

## Testing

The test suite lives in `littlehelper_test.go` (package `opnborg`) and covers env parsing, URL helpers, OPN/Unifi group builders, `splitPlugins`/`checkInstallPKG`, syslog config comparison, git init/checkin round-trip + aggressive-window + GC (repack-only, no prune), Unifi autoBackup watch, `checkIntoStore` rotation, compression helpers, httpd enable gating, non-blocking `/force` handler, forced-backup progress ring buffer, approval ledger round-trip / backfill / approve-all / commit-tracking / source capture, approval.db never-committed invariant (`TestApprovalDBNeverCommitted`), Ollama prompt / retry / fallback / model-match / health-check, OpenAI-compatible fallback / retry / health-check / dashboard panel / review-pending banner, audit page rendering / threat dashboard / tag-line highlighting / performer extraction / diff direction, dashboard gather/render, and the WebUI authentication two-mode model (credential generation, login success/failure lockout, session round-trip, nav-bar badge, greyed-out download buttons, `requireAdmin`/`requireAdminFiles` middleware blocking and pass-through, `/files` admin gate, audit locked hints, logout clearing `adminEnabled`, login handler, auth-hash generator). CI (`.github/workflows/golang.yml`) runs `go build ./...`, `go vet ./...`, and `go test -count=1 ./...` on ubuntu/macos/windows. Prefer extending the existing table-driven tests and keeping `make check` green.

## Coding Conventions

- One package (`opnborg`) at the repo root; `package main` only under `cmd/opnborg/`.
- File naming: kebab-case `topic.go`. Struct types are PascalCase; exported fields are PascalCase with inline `// comment` docs (see `OPNCall` in `api.go`).
- Constants and unexported globals are grouped at the top of the relevant file (often with `_` prefix like `_app`, `_lf`, `_archive`).
- Logging: never use `log`/`fmt.Println` in daemon hot paths — send `[]byte` to the `displayChan` channel. `fmt.Println` is acceptable in `srvHttpd.go` startup error paths only.
- HTTP handlers return `http.Handler` via `http.HandlerFunc` closure; apply `addSecurityHeader` at `mux.Handle` registration time.
- `config.dirty` is an `atomic.Bool` — always `.Store()`/`.Load()`, never `=`.
- `hive`, `unifiStatus`, and `unifiWatchStatus` are package globals guarded by `hiveMutex`/`unifiMutex`/`unifiWatchMutex`. Mutate only under the matching lock.
- `gofmt -s` (simplify) is enforced; run `make check` before committing.
- The WebUI HTML/SVG is hand-rolled and inlined as Go `const` strings in `httpd-ui.go`. The favicon is embedded via `//go:embed resources/borg.png`.

## Gotchas

- **Host/tag format** is parsed by `parseServerTag` (`srv.go`), the single source of truth for both the hive status-tile init and the worker dispatch; changes to the `server#tag` format only need to update that helper plus `actionOPN`'s signature.
- **`make deps` deletes `go.mod` and `go.sum` and re-runs `go mod init`** — only run when fully refreshing the module graph.
- **OPNsense API does not support legacy backup endpoints** — only `/api/core/backup/download/this` is wired in. Don't fall back to alternatives without explicit requirement.
- **HTTPS chain verification via the OS trust store is disabled by default**; security relies on `OPN_TLSKEYPIN`. Documenting "just use system CAs" would be wrong.
- **Auth session tokens live only in process memory** (`auth.sessions`): a daemon restart revokes every admin session and resets the global login-failure counter. There is no persistent session store; do not add one without revisiting the security model.
- **Never log or render the admin password** or raw `OPN_AUTH_*` values; the credential generator displays derived env lines once and the dashboard shows only a masked set/not-set pill.

## External Dependencies

- `github.com/go-git/go-git/v5` — pure-Go git for local repo management, commits, SSH push, and `git gc --aggressive`-equivalent per-tick repack (`gitGC` / `gitEnsureAggressiveWindow`).
- `github.com/cnaude/go-syslog/syslog/v3` — RFC5424 syslog server.
- `github.com/natefinch/lumberjack` + `github.com/sirupsen/logrus` — log rotation/formatting for the syslog sink.
- `github.com/fsnotify/fsnotify` — filesystem watcher for the Unifi autoBackup folder sync (`srvUnifiWatch.go`).
- `github.com/pmezard/go-difflib` — unified-diff generation for the model-assisted commit message feature.
- `modernc.org/sqlite` — pure-Go, CGO-free SQLite driver backing the security-approval ledger (`approval.go`).
- `golang.org/x/crypto/argon2` — Argon2id KDF backing WebUI admin authentication (`auth.go`).
- `paepcke.de/uniex` — Unifi inventory export logic.

Dependabot keeps these current via `.github/dependabot.yml`.
