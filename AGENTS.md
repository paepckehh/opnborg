# AGENTS.md

Reference guide for AI agents working in the `opnborg` repository.

> ## FIXED REQUIREMENT — EVERY CHANGE, NO EXCEPTIONS
>
> Before a task or change is considered done, all five steps below MUST be completed
> in this exact order. Skipping or reordering any step is a failure.
>
> 1. **Format source code** — run `gofmt -w .` (or `make check`) so the tree
>    stays gofmt-clean.
> 2. **Build** — `CGO_ENABLED=0 go build -o opnborg ./cmd/opnborg` must succeed.
>    **Always run with parallel package builds**: the Go toolchain compiles all
>    packages concurrently by default — never pass `-p 1` or any flag that
>    forces single-package builds. The machine has ample cores; serialise a
>    build only when a toolchain error specifically requires it.
> 3. **Test** — `go test -count=1 ./...` must be ok. **Always run with
>    parallel test execution**: let `go test` run every package's test binary
>    concurrently (the default). Never pass `-p 1` or otherwise force serial
>    package execution; only narrow the package list (e.g. `go test ./pkg/...`)
>    to iterate on a single failing package during a fix.
> 4. **Commit** — `git add . && git commit -m '<message>'`.
> 5. **Tag** — bump the patch segment only: the result is `v0.1.<N+1>`
>    (the current release series; the latest tag is `v0.1.185`). Never move,
>    delete, or reuse an existing tag. Also bump the `SemVer` constant in
>    `api.go` to match the new tag.
>
> These steps are non-negotiable for every single task regardless of size.


## Project Summary

`opnborg` is a single-binary Go daemon that backs up, monitors, and synchronizes configuration across a fleet of OPNsense firewalls and (optionally) Unifi controllers. Configuration is driven entirely through environment variables; the binary itself accepts only `-v` / `-h`. Output is emitted to stdout via an internal `displayChan` log engine, and an embedded HTTP server renders the hive status as HTML. Backups are stored as XML on disk, deduplicated by SHA-256, and (optionally) committed to a local git repo.

- Module path: `paepcke.de/opnborg`
- Go version: see `go.mod` (currently `1.26.5`); CI `golang.yml`, `release.yml`, and the Dockerfile all use `go-version-file: go.mod` / `golang:1.26`, so they track `go.mod` automatically (the earlier `1.23` pin in `golang.yml` was replaced by `go-version-file: go.mod`)
- Entry point: `cmd/opnborg/main.go` -> `opnborg.Start(config)` -> `srv(config)`
- License: BSD 3-Clause

## IMPORTANT FOR EVERY SINGLE TASK: NEVER SKIP THIS ACTIONS

- Test every change via build and unit tests; always run builds and tests with
  parallel package execution (`-p <cores>` or the Go toolchain default) — the
  machine has ample system resources, never serialise (`-p 1`) a build or test
  pass unless a toolchain error specifically demands it.
- commit each task into git repo when done
- **Every committed task must be tagged with a git semver tag**, bumping only
  the **patch** segment, the last segment (e.g. `v0.1.132` → `v0.1.133`).
  The other two first segments (major, minor) stay unmodified,
  increment only the last part by +1. Never move, delete, or rewrite an existing tag.

## Build, Run, Check

```sh
# Build the whole module (used by CI) — parallel package build
CGO_ENABLED=0 go build ./...

# Build just the binary (matches Dockerfile/goreleaser) — parallel package build
CGO_ENABLED=0 go build -ldflags="-w -s" ./cmd/opnborg

# Run locally from a checked-out repo (after sourcing env config)
go run cmd/opnborg/main.go

# Run via remote module
go run paepcke.de/opnborg/cmd/opnborg@main

# Format / lint / vet (top-level Makefile target)
make check        # runs gofmt -l ., go vet ./..., go mod tidy -diff

# Run the test suite (race detector enabled, parallel package execution)
make test         # runs go test -race -count=1 ./...

# Run the test suite with explicit parallel package execution (max cores)
go test -count=1 -p $(nproc) ./...

# Dependency refresh (DESTRUCTIVE - rewrites go.mod/go.sum)
make deps

# Install
go install paepcke.de/opnborg/cmd/opnborg@main
```

`make check` is the canonical pre-PR gate; it runs `gofmt -l .`, `go vet ./...`, and `go mod tidy -diff`. There is no `staticcheck` wiring in the Makefile or CI.

### Releasing

- Releases are tag-driven (`v*`). Pushing a `v*` tag triggers both `.github/workflows/release.yml` (goreleaser cross-compile via `.goreleaser.yml`) and `.github/workflows/ghcr.yml` (build/push `ghcr.io/paepckehh/opnborg:latest`).
- goreleaser builds target linux/freebsd/darwin/netbsd/openbsd/windows on amd64 + arm64, `CGO_ENABLED=0`.
- **Before tagging a release**, bump the `SemVer` constant in `api.go` to match the new tag (e.g. `v0.1.184` → `v0.1.185`). `SemVer` is the single source of truth for the version string and is consumed by the CLI startup banner (`cmd/opnborg/main.go`) and the WebUI footer. The top-level `Makefile` injects a build version via `-ldflags -X paepcke.de/opnborg/internal/version.Version=...` (sourced from `git describe --tags`), but no `internal/version` package exists, so that flag is currently a no-op and the displayed version comes from `SemVer`.

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

### HTTP WebUI (`srvHttpd.go`, `httpd-handler.go`, `httpd-ui.go`, `httpd-transport.go`, `progress.go`, `auth.go`, `auth-http.go`)

- `mux`: `/` (index), `/config` (config dashboard), `/audit` (BorgAUDIT commit-history page), `/progress` (forced-backup progress), `/files/` (static file server rooted at `config.Path`, admin session required), `/force` (manual trigger), `/approve` (single-commit approval toggle), `/approve-all` (bulk approve), `/auth/login` (admin login POST), `/auth/logout` (session revoke POST), `/auth/state` (mode + lock JSON polled by the nav-bar JS), `/auth-hash` (credential generator bootstrap page), `/favicon.ico`.
- `/force`, `/approve`, and `/approve-all` are registered **without** `addSecurityHeader` (they are mutating action endpoints, not page renders); all page-render routes (`/`, `/config`, `/audit`, `/progress`, `/files/`) are wrapped with it. `/files/` is additionally wrapped with `requireAdminFiles` (`auth-http.go`): monitoring-mode requests are redirected to the index with `?auth=locked` when credentials are armed, or straight to the config dashboard when login is impossible. `/force`, `/approve`, and `/approve-all` are wrapped with `requireAdmin` (`auth-http.go`): monitoring-mode POSTs are redirected to `audit?auth=locked` so an unauthenticated client cannot trigger a forced backup or approve commits by POSTing directly to the endpoint, bypassing the greyed-out UI buttons.
- Index handler renders HTML built from inlined SVG/HTML constants in `httpd-ui.go`. `_head`, `_forceRedirect` are assembled at `Setup()` time from `OPN_HTTPD_COLOR_FG` / `OPN_HTTPD_COLOR_BG`.
- Status strings (`_ok`, `_fail`, `_na`, `_degraded`, `_unifi`) are inline animated SVGs defined as `const` in `httpd-ui.go`. `status.go` mutates the `hive` / `unifiStatus` / `unifiWatchStatus` strings under `hiveMutex` / `unifiMutex` / `unifiWatchMutex`.
- The `/force` handler pokes the `updateOPN` / `updateUnifiBackup` / `updateUnifiExport` / `updateUnifiWatch` channels with non-blocking selects (buffer-1 channels): if a backup pass is already pending it drops the duplicate rather than blocking the HTTP client for a full backup cycle. It also bumps `forceSeq` (`progress.go`) so the animated progress dashboard knows a fresh forced pass is armed.
- **Forced-backup progress dashboard** (`progress.go`): every line the display engine writes to stdout is tee'd into a fixed-size in-memory ring buffer (`_progressCap`, 512 lines) guarded by `progressMu`. The `/progress` handler streams the captured lines back to the browser as JSON so the operator watches the backup happen in real time, and the page redirects back to the hive view once the forced pass ends. `forceSeq` / `passSeq` / `busy` are lock-free `atomic` counters.
- **AI review-in-progress banner** (`httpd-handler.go`): while `reviewPending` (`api.go`, an `atomic.Bool`) is set during an Ollama- or OpenAI-assisted commit generation (`git.go::gitCommit` sets it true around the model call and defers it false when either backend is enabled), the index page renders an animated "AI security review in progress" banner so the operator knows a commit is pending and not yet written.
- The httpd is armed only in daemon mode and only when `OPN_HTTPD_DISABLE` is unset. In one-shot mode (`OPN_NODAEMON`) `config.Httpd.Enable` stays false so `startWeb` is never called — previously it defaulted to true with an empty listen address.
- TLS is opt-in via `OPN_HTTPD_CACERT` + `OPN_HTTPD_CAKEY`; setting `OPN_HTTPD_CACLIENT` enables mTLS enforcement (`httpd-transport.go::getHTTPTLS`).

### WebUI authentication — two-mode access model (`auth.go`, `auth-http.go`)

The WebUI implements a two-mode access model and **always starts in monitoring-only view mode** — a login is never required to use the dashboard, and the `[ Authenticate ]` nav-bar button is purely an optional entry point.

- **Monitoring mode** (green `MONITORING` badge, right side of the nav bar, directly left of the version pill): full dashboard readable, config-file download buttons (`current.*` / `archive`) and BorgAUDIT approval actions locked (greyed out).
- **Admin mode** (red/yellow `ADMIN` badge): per-browser-session state unlocked by submitting the admin password via the nav-bar dialog; enables config downloads and the approve / approve-all actions.
- **Startup validation** (`authInit`, called from `Setup()`): the login feature is armed only when `OPN_AUTH_HASH` + `OPN_AUTH_SALT` are both present, non-empty, non-whitespace, and valid — the hash must carry the `$` separator with both key halves decoding to exactly 64 bytes (keylen), and the salt must be base64 decoding to >= 8 raw bytes. Any invalid content keeps authentication fully disabled (log line `[AUTH][DISABLED]`), the login dialog is never rendered, and `authCheckPassword` refuses every attempt.
- **Locked-button behavior**: in monitoring mode, greyed-out download buttons (rendered per render path via `renderDownloadButton` in `status.go`) and locked approve controls either open the login dialog (credentials armed) or navigate to the config dashboard's Authentication setup section (login impossible). The dialog markup is only emitted when the login flow is reachable.
- **Sessions**: successful login mints a 32-byte random token returned as an HttpOnly SameSite=Strict cookie (`opnborg_auth`, TTL 12 h sliding). `authIsAdmin(q)` is the single render-path gate; the `/files/` gate enforces it server-side.
- **Global lockout**: failed logins bump a process-global counter shared across ALL sessions — 10 s after the 1st failure, doubling with each further failure (10s, 20s, 40s, ...). The counter resets on a successful login, on daemon restart, and after 6 h of inactivity; the countdown is shown live in the nav bar and login dialog (polled from `/auth/state`).
- **Credential generator** (`/auth-hash`): enter a password, opnborg derives `OPN_AUTH_HASH` (format `<base64-key>$<base64-key>`) and `OPN_AUTH_SALT` (16 raw random bytes) with Argon2id (time=8, memory=64 MiB, threads=4, keylen=64). The password is never stored or logged; the endpoint refuses with 403 once valid credentials are armed. The Authentication tile on the config dashboard (`renderAuthPanel`) shows the current mode, masked credential state, the KDF parameters, and links to the generator when unarmed.
- Verification is constant-time (`subtle.ConstantTimeCompare`) over the Argon2id-derived keys; all state transitions are logged to `displayChan` with `[AUTH]` tags.

### BorgAUDIT — git commit history review (`audit.go`, `approval.go`, `approval-http.go`)

The `BorgAUDIT` tile on the index page and its dedicated `/audit?range=` page expose the storage repo git history so an operator can review recent backup changes at a glance — the human-facing half of the Ollama security-audit workflow (the AI half is the `tag:` security-impact line every Ollama-authored commit message ends with; see the *Ollama-assisted commit messages* bullet below). The tile is only emitted when `OPN_GIT_ENABLE` is set: without a git repo there is no commit history to audit. Each entry is a collapsible card carrying the commit hash, author, date, file-change stats, the full commit message (including the `tag: <severity>[, needs-review]` line when the commit was Ollama-authored), the full unified diff against the commit's first parent with syntax highlighting, and a `change-performed-by:` line surfacing the admin account that authored the configuration change (extracted from the diff via `extractPerformerFromDiff`, which reconstructs the new-version file content and scans for OPNsense admin-account XML fields). `gatherAuditCommits` walks the log filtered by the requested window and bounded by `_auditCap` (250 commits); the per-commit diff is capped at `_auditDiffCap` (512 KB) with a visible truncation marker. The walker opens the repo directly against `config.Path` (no `os.Chdir`) so it is safe to run from the httpd goroutine concurrently with the backup workers. Three windows are offered via the `?range=` query param (validated by `auditRangeSlug`, default `24h`): `24h`, `7d`, `1m`. A `renderAuditThreatDashboard` summary renders an interactive threat-level categorisation map (emoji + label + count) at the top of the page so an operator can click a threat class to filter the timeline.

### Security-approval ledger (`approval.go`, `approval-http.go`)

A single on-disk SQLite database (`approval.db`, co-located with the backup store at `config.Path`) tracks every opnborg-authored git commit whose Ollama security-impact tag is above `low`/`none`/`backup` (i.e. `medium` / `high` / `critical`). For each tracked commit the ledger records the full git hash, the security severity, the commit headline, the commit timestamp, and an approval state an operator can toggle from the BorgAUDIT WebUI page. Toggling a commit to approved records the wall-clock timestamp together with the source IP address, the `X-Forwarded-For` reverse-proxy chain, and the `Remote-User` authenticated identity of the operator, so every approval carries a full audit trail of who acted and from where.

- The ledger uses `modernc.org/sqlite` (pure-Go, CGO-free SQLite), so the binary stays `CGO_ENABLED=0`.
- `approvalDB` is a package-global `*sql.DB` opened once from `gitInit` (`git.go`) after the repo is initialized, and lazily from the httpd. On a fresh store, `approvalBackfillFromHistory` walks the existing commit log and backfills any security-relevant commits that predate the ledger.
- `approvalTrackCommit` is called from `gitCommit` after every commit to insert (idempotently) any security-relevant commit into the ledger.
- `syncAuditCommitsToLedger` reconciles the ledger with the audit page's commit list so the approval state shown on the page matches the database.
- The `.gitignore` carries an `approval.db*` glob (plus explicit `-wal` / `-shm` entries) so the ledger database and its SQLite WAL sidecars are never committed. `gitEnsureIgnore` reconciles this line on every startup, and `gitCommit` skips any path containing `approval.db`.
- `POST /approve?hash=<full-git-hash>&range=<range>` and `POST /approve-all?range=<range>` toggle approval state and redirect back to the audit page. Both handlers capture the operator identity from the request (`approvalSourceFromRequest`). Both actions additionally require a live admin-mode session (see *WebUI authentication* above): the handlers are wrapped with `requireAdmin` (`auth-http.go`), so an unauthenticated POST is redirected to `audit?auth=locked` before the inner handler runs. In monitoring mode the per-commit approve button and the approve-all button render as locked 🔒 hints (tooltip pointing at the config dashboard's authentication setup section). `renderAuditApprovalControl` / `renderAuditApproveAllButton` take the admin flag threaded from `renderAuditPage` (`authIsAdmin(q)`).

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
- **Ollama-assisted commit messages** (opt-in via `OLLAMA_DESC_URL` + `OLLAMA_DESC_MODEL`, both must be non-empty). When enabled, before each non-Unifi commit the HEAD-vs-worktree diff is POSTed to the Ollama REST API (`<OLLAMA_DESC_URL>/api/generate`, `stream=false`) with a prompt that casts the model as an infrastructure / Unix firewall expert and asks for a short headline plus a brief one-to-three line summary of the change and its key security implication, ending with a trailing `tag:` security-impact line. The model's response is used verbatim (trimmed) as the commit message in a single REST call: the model owns the full message (headline + brief summary + `tag:` line). Commits whose changed files are all `.unf` Unifi backups keep the static default message (`opnborg auto update`) without consulting the model, since the `.unf` format is an opaque binary archive. Any model error, empty response, or timeout falls back to the default message so a model outage never blocks a backup from being committed. Implemented in `ollama.go`; wired into `gitCommit` (`git.go`). The diff is computed in-process with `go-difflib` against HEAD blobs and worktree files (capped at `_ollamaMaxDiffBytes`, currently 256 KB). It is an **enriched** diff, not raw hunks: `gitDiffText` emits a `=== COMMIT SUMMARY ===` header (files changed, total +insertions/-deletions, per-file change kind and +/- counts), then per-file `=== FILE ===` blocks carrying change kind, before/after byte and line sizes, detected OPNsense XML top-level sections (via `xmlTopLevelSections`, which walks the `<opnsense>` root children such as `filter`, `aliases`, `interfaces`, `gateways`, `nat`, `ipsec`, `vpn`, `cert`), the unified hunks with widened context (`_ollamaDiffContext`, currently 8 lines), and for small files (`_ollamaSmallFileBytes`, currently 16 KB) the full resulting content so the model can describe the complete new state. The system prompt tells the model how to read this structure so it grounds its summary in concrete change geometry and the affected configuration subtrees. The affected server name(s) are injected as explicit input via `extractServersFromStatus` (deduplicated, sorted first path segment of every changed file, e.g. `fw01.lan/current.xml` → `fw01.lan`) so the model anchors its summary to the changed appliance rather than inferring it from diff paths. The API timeout is 240 s (4 minutes) per attempt (`_ollamaTimeout`), with up to `_ollamaMaxRetries` (5) retries and a `_ollamaRetryBackoff` (2 s) pause between attempts. Every step of the flow (diff build, prompt assembly, each attempt, send, wait, receive, success, failure, fallback) is logged to `displayChan` with `[OLLAMA]` prefixed tags so an operator can follow the progress in the daemon log.
- **OpenAI-compatible fallback commit messages** (opt-in via `OPENAI_DESC_URL`, required; `OPENAI_DESC_MODEL`, optional, defaults to `_openaiDefaultModel` `gpt-4o-mini`; `OPENAI_DESC_TOKEN`, optional). When the Ollama backend is not configured (`OLLAMA_DESC_*` unset) or the Ollama endpoint fails on every retry, the OpenAI-compatible REST API is used as a fallback with the same system prompt, the same enriched diff, and the same retry/fallback contract. The OpenAI-compatible endpoint is `<OPENAI_DESC_URL>/chat/completions` (the standard OpenAI chat completions API), sent a two-message chat (system persona + user diff payload) with `stream=false` so the whole response arrives in one shot. When `OPENAI_DESC_TOKEN` is set it is sent as `Authorization: Bearer <token>`; some OpenAI-compatible servers (e.g. a local vLLM or Ollama `/v1` shim) do not require a token, so it is optional. The model is also optional and defaults to `gpt-4o-mini`; operators who need a different model set `OPENAI_DESC_MODEL` explicitly. The first choice's content from the `choices` array is used verbatim (trimmed) as the commit message. Any model error, empty response, or timeout falls back to the default message so a model outage never blocks a backup from being committed. Implemented in `ollama.go` alongside the Ollama backend; the shared retry loop (`generateWithRetry`) is used by both backends so the retry count, backoff, and log tags (`[OPENAI]` vs `[OLLAMA]`) are consistent. The `reviewPending` atomic banner is armed when either backend is enabled. The config dashboard shows a dedicated `OpenAI Commit Messages` panel (`renderOpenAIPanel`) with the parsed env vars and a live `/models` probe (`openaiHealthCheck`) mirroring the Ollama `/api/tags` probe. The `OPENAI_DESC_TOKEN` is never logged or rendered in the clear; the dashboard shows it as a set/not-set pill.
- **AI security-audit review via the `tag:` severity line.** The Ollama system prompt (`_ollamaSystemPrompt`) instructs the model to produce a brief commit message: a short headline, a one-to-three line summary of the change and its key security implication in plain language, followed by a single `tag: <severity>[, needs-review]` line that classifies the security impact of the change so every backup commit can be triaged by risk. The model is told to keep it short — no deep analysis, no structured sections, no bullet lists — just a quick, factual summary. `<severity>` is one of `low` (routine, no security impact), `medium` (bounded hardening/exposure change), `high` (broadens attack surface or weakens hardening), or `critical` (removes a key control or broadly exposes a sensitive service); `, needs-review` is appended when a human should inspect the change before it ships (high/critical severity, ambiguous intent, any auth/cert/IPsec/firewall-defaults change). opnborg does not parse or act on the tag — it is written verbatim into the commit message and surfaced on the `BorgAUDIT` history page for an operator to review. This is the AI half of the security-audit workflow; the human half is the `BorgAUDIT` commit-history page (see the *BorgAUDIT* subsection under *HTTP WebUI* above).
- The internal httpd is armed only in daemon mode and only when `OPN_HTTPD_DISABLE` is unset (daemon mode + flag unset → enable; everything else → disable).
- **WebUI authentication env vars** (`OPN_AUTH_HASH` + `OPN_AUTH_SALT`, both required together): when both are set and valid, the nav-bar `[ Authenticate ]` button opens the login dialog and admin mode unlocks per browser session. When they are missing, empty, whitespace-only, or invalid (`$`-separator missing, key halves not decoding to exactly 64 bytes, malformed base64, salt < 8 raw bytes), authentication is fully disabled: the WebUI stays in monitoring-only mode, the login dialog is never rendered, and locked controls point at the config dashboard's Authentication setup section. See the *WebUI authentication* subsection under *HTTP WebUI*.
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

`git.go` manages the storage folder as a git repo (opt-in via `OPN_GIT_ENABLE`). `gitInit` (`git.PlainOpen` / `git.PlainInit` on first run) plus `gitEnsureIgnore` run once at startup; `gitCheckIn` (`os.Chdir(config.Path)`, open repo, `wtree.Status()` fast-path, `wtree.Add(".")`, commit) runs per tick when `config.dirty` is set. The commit author is the static `OPNBORG-AUTO-COMMIT` handle (`_authorName`) unless Ollama or the OpenAI-compatible fallback authored the message, in which case `authorFromCommitMessage` (`ollama.go`) replaces it with a short, sanitised version of the commit headline so the commit log surfaces the change at a glance; it falls back to `_authorName` for the default message, `.unf` bypass, or any failure. When Ollama-assisted generation is enabled (`OLLAMA_DESC_URL` + `OLLAMA_DESC_MODEL`, see `ollama.go`), `gitCommit` calls `generateCommitMessage` to route the diff to the model in a single REST call and use its response verbatim — a short headline, a structured security and impact review, and the trailing `tag: <severity>[, needs-review]` security-impact classification line the model is prompted to emit (see the *Ollama-assisted commit messages* bullet under *Configuration Conventions*); when the Ollama backend fails and the OpenAI-compatible fallback is enabled (`OPENAI_DESC_URL`, see the *OpenAI-compatible fallback commit messages* bullet), the same diff is routed to the OpenAI-compatible `/chat/completions` endpoint with the same prompt; `.unf`-only commits and any model failure (both backends exhausted) keep the default message. After every commit, `approvalTrackCommit` inserts any security-relevant commit into the approval ledger (see *Security-approval ledger* above). When `OPN_GIT_UPSTREAM` is set, `gitPush` recreates the `origin` remote if its URL drifted and pushes via `ssh.NewPublicKeysFromFile` from `OPN_GIT_SSH_KEY`; host key verification uses go-git's default `~/.ssh/known_hosts` callback. `gitGC` runs a best-effort per-tick repack equivalent to `git gc --aggressive` (`gitEnsureAggressiveWindow` raises `pack.deltaWindow` to `_aggressivePackWindow`, 250) and prunes unreachable loose objects; failures are logged and never block a commit. All git operations use the native `go-git` library — no external `git` binary is invoked.

`checkIntoStore` (`store.go`) resolves every path against `config.Path` and does **not** call `os.Chdir`, so it is safe to invoke from the concurrent per-server worker goroutines. The `CONFIG-CURRENT` / `CONFIG-LAST` symlinks use a relative target (the `.archive/...` path) so the store tree stays portable when copied or moved. The only remaining `os.Chdir` call sites are `gitInit` / `gitCheckIn` (both Chdir to `config.Path`, the same directory) and `startWeb` / `startRSysLog` (startup only, before workers run).

### Config dashboard (`config-dashboard.go`, `dashboard.go`)

- `renderOllamaPanel` (`config-dashboard.go`) shows the parsed `OLLAMA_DESC_URL` / `OLLAMA_DESC_MODEL` values plus a live probe of the daemon (`ollama.go::ollamaHealthCheck`) that GETs `<OLLAMA_DESC_URL>/api/tags` with a short timeout (`_ollamaHealthTimeout`, 3 s) on every dashboard render and reports three layered signals: server reachable, REST API ready (parseable `/api/tags` JSON), and model ready (the configured model is present in the tags list, matched by exact name or `<model>:<tag>` prefix via `ollamaModelMatch`).
- `renderOpenAIPanel` (`config-dashboard.go`) shows the parsed `OPENAI_DESC_URL` / `OPENAI_DESC_MODEL` / `OPENAI_DESC_TOKEN` values plus a live probe of the OpenAI-compatible server (`ollama.go::openaiHealthCheck`) that GETs `<OPENAI_DESC_URL>/models` with a short timeout on every dashboard render and reports the same three layered signals (server reachable, REST API ready, model ready). The token is shown as a set/not-set pill (`secretPill`), never the raw value.
- `gatherDashboard` (`dashboard.go`) describes the state of the on-disk backup store, the local git repository, and the upstream sync health. It is gathered on every WebUI dashboard render and covers backup folder stats, git repo state, and upstream divergence (`countDivergence` walks reachable hashes to compute ahead/behind counts).

## Testing

The test suite lives in `littlehelper_test.go` (package `opnborg`) and covers env parsing (`isEnv`, `Setup`), URL helpers, the OPN/Unifi group builders, `splitPlugins`/`checkInstallPKG`, the syslog config comparison, the git init/checkin round-trip plus aggressive-window and GC behavior, the Unifi autoBackup watch setup/sync, `checkIntoStore` rotation, the compression helpers, the httpd enable gating, the non-blocking `/force` handler, the forced-backup progress ring buffer and lifecycle, the approval ledger round-trip / backfill / approve-all / commit-tracking / source capture, the Ollama prompt / retry / fallback / model-match / health-check, the OpenAI-compatible fallback direct-use / fallback-from-Ollama / retry / fallback-on-error / health-check / dashboard panel / review-pending banner, the audit page rendering / threat dashboard / tag-line highlighting / performer extraction / diff direction, the dashboard gather/render, and the WebUI authentication two-mode model (credential generation + format validation, login success / failure lockout doubling, session round-trip, nav-bar mode badge and dialog gating, greyed-out download button states, /files admin gate, audit locked approve hints, login handler flow, and the auth-hash generator flow). Run it via `make test` (`go test -race -count=1 ./...`) — note that `-race` requires CGO + a C compiler, so on minimal/CGO-disabled toolchains use `go test -count=1 ./...` instead. CI (`.github/workflows/golang.yml`) runs `go build ./...`, `go vet ./...`, and `go test -count=1 ./...` on ubuntu/macos/windows using `go-version-file: go.mod`. When adding code, prefer extending the existing table-driven tests and keeping the `make check` gate green.

## Coding Conventions

- One package (`opnborg`) at the repo root; `package main` only under `cmd/opnborg/`. Do not introduce subpackages without strong reason.
- File naming: kebab-case `topic.go`. Struct types are PascalCase; exported fields are PascalCase with inline `// comment` docs (see `OPNCall` in `api.go`).
- Constants and unexported globals are grouped at the top of the relevant file (often with `_` prefix for package-private consts like `_app`, `_lf`, `_archive`).
- Logging: never use `log`/`fmt.Println` directly inside the daemon hot paths -- send `[]byte` to the `displayChan` channel so the background `startLog` goroutine serializes output. `fmt.Println` is acceptable in `srvHttpd.go` startup error paths only because it runs before the display engine is ready.
- HTTP handlers return `http.Handler` constructed via `http.HandlerFunc` closure; apply `addSecurityHeader` middleware at `mux.Handle` registration time, not inside the handler.
- `config.dirty` is an `atomic.Bool` -- always `.Store()`/`.Load()`, never `=`.
- `hive`, `unifiStatus`, and `unifiWatchStatus` are package globals guarded by `hiveMutex`/`unifiMutex`/`unifiWatchMutex`. Mutate only under the matching lock.
- `gofmt -s` (simplify) is enforced; run `make check` before committing.
- **Parallel build & test execution**: every `go build` and `go test` invocation
  must use parallel package execution. The Go toolchain runs package builds in
  parallel by default (one goroutine per core) and runs each package's test
  binary concurrently — never pass `-p 1` or any serialising flag. When
  iterating on a single failing package, narrow the package list
  (e.g. `go test ./pkg/...`) instead of serialising the whole run. Use
  `-p $(nproc)` (or let the toolchain default) to pin the parallelism explicitly
  when the environment provides more cores than Go detects.
- The WebUI HTML/SVG is hand-rolled and inlined as Go `const` strings in `httpd-ui.go`. The favicon is embedded via `//go:embed resources/borg.png`.

## Gotchas

- **Duplicate `#` split logic** in `srv.go` (lines ~88 and ~136) parses `server#tag` in two places. Changes to the host/tag format must update both, plus `actionOPN`'s signature.
- **`os.Chdir` is called from a few startup paths** (`gitInit`, `gitCheckIn`, `startWeb`, `startRSysLog`). `checkIntoStore` no longer Chdirs — it resolves paths against `config.Path` — so the per-server worker goroutines do not race on the process-wide CWD. The remaining Chdir sites either run at startup (before workers) or Chdir to the same `config.Path`, so they do not observably race; still, prefer absolute paths when adding new code.
- **`make deps` deletes `go.mod` and `go.sum` and re-runs `go mod init`** -- only run it when you intend to fully refresh the module graph.
- **`SemVer` in `api.go` is the single source of truth** for the version string (CLI banner via `cmd/opnborg/main.go` + WebUI footer). goreleaser consumes git tags, not this constant. Bump it in lockstep with each release tag (see *Releasing* above).
- **CI Go version tracks `go.mod` via `go-version-file: go.mod`** in both `golang.yml` and `release.yml`; the Dockerfile pins `golang:1.26`. The earlier `1.23` pin in `golang.yml` was replaced, so all build paths now follow `go.mod` (currently `1.26.5`).
- **OPNsense API does not support legacy backup endpoints** -- only `/api/core/backup/download/this` is wired in (`_apiBackupXML`). Don't fall back to alternatives without explicit requirement.
- **HTTPS chain verification via the OS trust store is disabled by default**; security relies on `OPN_TLSKEYPIN`. Documenting "just use system CAs" would be wrong.
- The `resources/` directory contains the embedded favicon (`borg.png`) and sample screenshots referenced from `README.md`; the `resources/opnborg/index.html` is the legacy static demo page, not the live UI.
- **`OPN_NOGIT` is no longer honored** — the git feature is opt-in via `OPN_GIT_ENABLE`. Stale references in older docs to `OPN_NOGIT` inverting the default are obsolete; the only env-inverted flag is `OPN_NODAEMON`.
- **The approval ledger (`approval.db`) is never committed**: the `.gitignore` carries `approval.db*` (plus explicit `-wal` / `-shm` entries), `gitEnsureIgnore` reconciles it on every startup, and `gitCommit` skips any path containing `approval.db`. Do not remove these guards — the ledger is a local-only runtime database.
- **`reviewPending` (`api.go`) is an `atomic.Bool`** set true around the Ollama or OpenAI model call in `gitCommit` (when either backend is enabled) and deferred false; the index page reads it to show an "AI security review in progress" banner. Always use `.Store()`/`.Load()`, never `=`.
- **`modernc.org/sqlite` keeps the binary CGO-free**: the approval ledger uses a pure-Go SQLite driver so `CGO_ENABLED=0` builds (Dockerfile, goreleaser, CI) still work. Do not swap it for a CGO-based SQLite driver without also revisiting the build flags.
- **The WebUI always starts in monitoring-only view mode**; a login is never required. The `[ Authenticate ]` nav-bar button is an optional entry point. Login is armed only when `OPN_AUTH_HASH` + `OPN_AUTH_SALT` are both set AND pass the strict startup validation (see `authInit`); any missing/empty/invalid value disables authentication entirely — do not render the login dialog or call `openAuthDialog` from locked controls in that state; point operators at the config dashboard's Authentication setup section instead.
- **`adminEnabled` (`api.go`/`auth.go`) is an `atomic.Bool`** mirroring the live admin-session state for the render path (greyed-out download buttons read it without a request handle). Always use `.Load()`/`.Store()`, never `=`.
- **Auth session tokens live only in process memory** (`auth.sessions`): a daemon restart revokes every admin session and resets the global login-failure counter. There is no persistent session store; do not add one without revisiting the security model.
- **Never log or render the admin password** or the raw `OPN_AUTH_*` values; the credential generator displays the derived env lines once and the config dashboard shows only a masked set/not-set pill.

## External Dependencies of Note

- `github.com/go-git/go-git/v5` -- pure-Go git for local repo management, commits, SSH push to upstream, and the `git gc --aggressive`-equivalent per-tick repack (`gitGC` / `gitEnsureAggressiveWindow`).
- `github.com/cnaude/go-syslog/syslog/v3` -- RFC5424 syslog server.
- `github.com/natefinch/lumberjack` + `github.com/sirupsen/logrus` -- log rotation/formatting for the syslog sink.
- `github.com/fsnotify/fsnotify` -- filesystem watcher for the Unifi autoBackup folder sync (`srvUnifiWatch.go`).
- `github.com/pmezard/go-difflib` -- unified-diff generation for the Ollama-assisted commit message feature; `ollama.go` builds an enriched diff (commit summary + per-file metadata + widened hunks + detected OPNsense XML sections + full small-file content) on top of `go-difflib` hunks.
- `modernc.org/sqlite` -- pure-Go, CGO-free SQLite driver backing the security-approval ledger (`approval.go`); keeps the binary `CGO_ENABLED=0`.
- `golang.org/x/crypto/argon2` -- Argon2id KDF backing the WebUI admin authentication (`auth.go`): credential generation (`/auth-hash`) and constant-time login verification with the fixed parameter set time=8, memory=64 MiB, threads=4, keylen=64.
- `paepcke.de/uniex` -- Unifi inventory export logic (delegated to that module; opnborg only wires the env config).

Dependabot keeps these current via `.github/dependabot.yml`.
