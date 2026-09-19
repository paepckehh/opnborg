<div align="center">

<img src="resources/borg.png" width="180" alt="OPNBORG"/>

# ⚙️ OPNBORG

### Resistance is futile. Your OPNsense will be assimilated. 🖖

**One binary. Your whole firewall fleet. Backed up, monitored, audited.**

Self-hosted daemon for [OPNsense](https://opnsense.org/) config management — with an embedded WebUI,
AI-assisted security audit, git-tracked archive, and optional [Unifi](https://ui.com) integration.

[![Go Reference](https://pkg.go.dev/badge/paepcke.de/opnborg.svg)](https://pkg.go.dev/paepcke.de/opnborg)
[![Go Report Card](https://goreportcard.com/badge/paepcke.de/opnborg)](https://goreportcard.com/report/paepcke.de/opnborg)
[![Go Build](https://github.com/paepckehh/opnborg/actions/workflows/golang.yml/badge.svg)](https://github.com/paepckehh/opnborg/actions/workflows/golang.yml)
[![SemVer](https://img.shields.io/github/v/release/paepckehh/opnborg)](https://github.com/paepckehh/opnborg/releases/latest)
[![License](https://img.shields.io/github/license/paepckehh/opnborg)](https://github.com/paepckehh/opnborg/blob/master/LICENSE)
[![built with nix](https://builtwithnix.org/badge.svg)](https://search.nixos.org/packages?channel=unstable&from=0&size=50&sort=relevance&type=packages&query=opnborg)

[🚀 Quick Start](#-quick-start) · [✨ Features](#-features) · [📸 Screenshots](#-screenshots) · [🔧 Config](#-configuration) · [❓ FAQ](#-faq)

</div>

---

## TL;DR

```sh
OPN_TARGETS='opn01.lan:8443,opn02.lan:8443' \
OPN_APIKEY='...' OPN_APISECRET='...' \
go run paepcke.de/opnborg/cmd/opnborg@main
# ▸ WebUI live at http://localhost:6464
```

---

## ✨ Features

🖥️ **Hive dashboard** — live version, status & compliance per firewall, animated SVG, zero JS frameworks
💾 **Smart backups** — SHA-256 deduplicated, timestamped archive, symlink rotation, rapid restore
🔁 **Fleet sync** — replicate plugins & syslog config from a master host to every target
🤖 **AI security audit** — local LLM (Ollama / OpenAI-compatible) writes commit messages with a `tag: <severity>` line
🧟 **BorgAUDIT** — commit history review: full diffs, threat dashboard, change performer, approval ledger
🎛️ **Unifi** — `.unf` backups, autoBackup folder watch, inventory export (CSV/JSON)
📜 **Syslog collector** — built-in RFC5424 server, rotated retention
🔐 **Two-mode WebUI** — monitoring-only by default; Argon2id-gated admin sessions for sensitive actions
📦 **One static binary** — `CGO_ENABLED=0`, no runtime deps · Linux · BSD · Windows · amd64/arm64/armv7
☁️ **Git native** — go-git repo management, aggressive repack, SSH upstream push with host-key pinning

---

## 📸 Screenshots

![OPNBORG Sample Screenshot 01](resources/sc01.png)
![OPNBORG Sample Screenshot 02](resources/sc02.png)

---

## 🚀 Quick Start

Run the latest release straight from source — no build step:

```sh
OPN_PATH='/tmp/opn' \
OPN_TARGETS='opn01.lan:8443,opn02.lan:8443' \
OPN_APIKEY='...' \
OPN_APISECRET='...' \
go run paepcke.de/opnborg/cmd/opnborg@main
```

Prebuilt binaries: [Releases](https://github.com/paepckehh/opnborg/releases) · `go install paepcke.de/opnborg/cmd/opnborg@main`
Add `OPN_NODAEMON=1` for a single pass instead of the daemon.

<details>
<summary><b>🐳 Docker / Docker Compose</b> — distroless, <code>linux/amd64</code> + <code>linux/arm64</code></summary>

Published from every `v*` tag to [ghcr.io/paepckehh/opnborg](https://github.com/paepckehh/opnborg/pkgs/container/opnborg) — tags: `latest`, `edge`, `v0`, `v0.1`, exact release.

```yaml
services:
  opnborg:
    image: ghcr.io/paepckehh/opnborg:latest
    restart: unless-stopped
    ports: ["6464:6464"]
    volumes: [opnborg-data:/var/opnborg]
    environment:
      OPN_TARGETS: "opn01.lan:443,opn02.lan:443"
      OPN_APIKEY: "+RIb6YWNdcDWMMM7W5ZY..."
      OPN_APISECRET: "8VbjM3HKKqQW2ozO..."

volumes:
  opnborg-data:
```

> 🔐 Expose `6464` to trusted networks only — and arm `OPN_AUTH_HASH` / `OPN_AUTH_SALT` for admin mode.

</details>

<details>
<summary><b>❄️ NixOS / Nix</b></summary>

Available as a [Nix package](https://search.nixos.org/packages?channel=unstable&from=0&size=50&sort=relevance&type=packages&query=opnborg). Ready-made modules in this repo:

`opnborg-docker.nix` · `opnborg-docker-complex.nix` · `opnborg-prometheus-grafana.nix` (Prometheus + Grafana; Wazuh/Influx/Graylog WIP)

</details>

---

## 🔧 Configuration

Everything is **environment variables** — the binary accepts only `-v` / `-h`. A local `.env` file is auto-loaded.
Ready-to-use examples: `example.sh`, `example-env-config-simple.sh`, `example-env-config-complex.sh`, `example-env-config-unifi.sh`, `example-env-config-dev.sh`.

> ⚠️ **Booleans are presence-based.** `OPN_DEBUG=0` and `OPN_DEBUG=false` are both **true** — unset the var to disable. `OPN_GIT_ENABLE` is opt-in.

<details open>
<summary><b>🎯 Basics</b></summary>

| Variable | Default | Description |
| --- | --- | --- |
| `OPN_APIKEY` / `OPN_APISECRET` | — | OPNsense backup user API credentials *(required)* |
| `OPN_TARGETS` | — | Comma-separated targets, optional `#<asset-tag>` (`opn01.lan:8443#RACK-PROD01`) |
| `OPN_TARGETS_<GROUP>` | — | Named groups: `OPN_TARGETS_INTRANET='opn01.lan:8443,...'` |
| `OPN_TARGETS_DESC_<GROUP>` | — | WebUI group description |
| `OPN_TARGETS_IMGURL_<GROUP>` | — | WebUI group image URL (description becomes the tooltip) |
| `OPN_PATH` | `.` | Backup store path |
| `OPN_TLSKEYPIN` | — | SPKI SHA-256 keypin — MitM-proofing for the OPNsense API |
| `OPN_SLEEP` | `3600` | Poll interval in seconds (min `10`) |
| `OPN_MASTER` | — | Master host — replicate its config across the hive |
| `OPN_SYNC_PKG` | — | Sync installed plugins from master to all targets |
| `OPN_EMAIL` | `git@opnborg` | Git commit author email |
| `OPN_NODAEMON` | — | One-shot mode: single pass, then exit |
| `OPN_DEBUG` | — | Verbose debug logging |

</details>

<details>
<summary><b>☁️ Git store</b></summary>

`OPN_PATH` is managed as a git repo (native go-git, no external git binary): auto-init, `.gitignore` upkeep, commit per backup pass, aggressive repack (delta window=250) after each commit.

| Variable | Description |
| --- | --- |
| `OPN_GIT_ENABLE` | Auto commit each backup pass — opt-in |
| `OPN_GIT_UPSTREAM` | SSH remote to push to; unset = local-only |
| `OPN_GIT_SSH_KEY` | PEM private key for upstream auth (required with upstream) |
| `OPN_GIT_SSH_HOSTKEY` | `SHA256:<base64>` host-key pin; unset skips verification |

</details>

<details>
<summary><b>🤖 AI security audit</b> — Ollama first, OpenAI-compatible fallback</summary>

The LLM reads an enriched diff and writes the commit message — headline, security summary, trailing `tag: low|medium|high|critical[, needs-review]` — surfaced on the BorgAUDIT page. Needs `OPN_GIT_ENABLE`.

```sh
export OLLAMA_DESC_URL='http://localhost:11434'
export OLLAMA_DESC_MODEL='llama3'
# or/and (automatic fallback):
export OPENAPI_DESC_URL='http://192.168.6.222:11434/v1'   # model: gpt-4o-mini (default)
# export OPENAPI_DESC_MODEL='...' OPENAPI_DESC_TOKEN='...'
```

</details>

<details>
<summary><b>🔐 WebUI auth</b> — monitoring by default, admin opt-in</summary>

Monitoring mode is always open (green badge): dashboards, audit page, progress. Sensitive actions — config downloads, audit diffs, forced backup, approvals — need an admin session (red/yellow badge): Argon2id (time=8, 64 MiB) credentials, HttpOnly SameSite=Strict cookies, 12 h sliding TTL, exponential global lockout on failed logins.

Generate `OPN_AUTH_HASH` + `OPN_AUTH_SALT` from a password on the `/auth-hash` page — nothing is ever logged. Without valid credentials, login is impossible and sensitive paths return 403.

</details>

<details>
<summary><b>🖥️ WebUI routes & HTTP options</b></summary>

| Route | Access | What |
| --- | --- | --- |
| `/` | open | Hive dashboard |
| `/config` | open | Auth, AI, git & store tiles |
| `/audit` | open | BorgAUDIT — commit history, threat dashboard |
| `/progress` | open | Live forced-backup progress |
| `/force` | open | Trigger a backup pass now |
| `/files/` | admin | Config downloads |
| `/approve`, `/approve-all` | admin | Approval ledger actions (POST) |
| `/auth/*`, `/auth-hash` | open | Login/logout/state, credential generator |

| Variable | Default | Description |
| --- | --- | --- |
| `OPN_HTTPD_SERVER` | `127.0.0.1:6464` | Listen address |
| `OPN_HTTPD_DISABLE` | — | Turn the WebUI off |
| `OPN_HTTPD_CACERT` / `OPN_HTTPD_CAKEY` | — | Enable HTTPS |
| `OPN_HTTPD_CACLIENT` | — | Enforce mTLS |
| `OPN_HTTPD_COLOR_FG` / `OPN_HTTPD_COLOR_BG` | `white` / `#333333` | Theme colors |
| `OPN_RSYSLOG_ENABLE` + `OPN_RSYSLOG_SERVER` | — | Built-in RFC5424 collector (e.g. `192.168.0.1:5140`) |

</details>

<details>
<summary><b>🎛️ Unifi controller</b> — backups, watch, inventory</summary>

```sh
export OPN_UNIFI_WEBUI='https://192.168.1.10:8443#RACK-PROD03'
export OPN_UNIFI_BACKUP_USER='backup'
export OPN_UNIFI_BACKUP_SECRET='start'
export OPN_UNIFI_VERSION='8.5.6'              # required
# optional:
export OPN_UNIFI_WATCH_PATH='/var/lib/unifi/data/backup/autobackup'  # watch-only deploy is valid
export OPN_UNIFI_EXPORT='1'; export OPN_UNIFI_FORMAT='csv'           # or 'json'
export OPN_UNIFI_MONGODB_URI='mongodb://127.0.0.1:27117'
export OPN_UNIFI_BACKUP_DESC='Network controller'   # (+_IMGURL) like target groups
```

</details>

<details>
<summary><b>📈 Observability links</b> — Prometheus, Grafana, Wazuh</summary>

`OPN_PROMETHEUS_WEBUI` · `OPN_WAZUH_WEBUI` · `OPN_GRAFANA_WEBUI` · `OPN_GRAFANA_DASHBOARD_FREEBSD` · `OPN_GRAFANA_DASHBOARD_HAPROXY` · `OPN_GRAFANA_DASHBOARD_UNIFI` — deep links into your existing monitoring stack, rendered on the dashboard.

</details>

<details>
<summary><b>🗄️ Storage layout</b></summary>

```
<OPN_PATH>/<server>/
  current.xml          # latest backup
  .archive/<Y>/<M>/<ts>-<server>.xml
  CONFIG-CURRENT/LAST # symlink rotation
  sha256.db           # dedup log
approval.db            # security-approval ledger (SQLite, always gitignored)
.git/                  # when OPN_GIT_ENABLE is set
```

</details>

---

## ❓ FAQ

<details>
<summary><b>Create an OPNsense API key?</b></summary>

System → Access → Users → add a `backup` user (scrambled password) → grant *Diagnostics: Configuration History* (+ *System: Firmware* for plugin sync) → API Keys → Add.

</details>

<details>
<summary><b>Generate <code>OPN_TLSKEYPIN</code>?</b></summary>

```sh
go run paepcke.de/tlsinfo/cmd/tlsinfo@latest <your-opn-server-name>
# X509 Cert KeyPin [base64] : [FezOCC3qZFzBmD5xRKtDoLgK445Kr0DeJBj2TWVvR9M=]
export OPN_TLSKEYPIN='FezOCC3qZFzBmD5xRKtDoLgK445Kr0DeJBj2TWVvR9M='
```

</details>

<details>
<summary><b>Enable the AI audit?</b></summary>

Install [Ollama](https://ollama.com), `ollama pull llama3`, set `OLLAMA_DESC_URL` + `OLLAMA_DESC_MODEL`, enable `OPN_GIT_ENABLE`, restart.

</details>

<details>
<summary><b>Arm admin mode?</b></summary>

`/config` → *Create Authentication Env Vars* → copy both `OPN_AUTH_*` lines into your env → restart.

</details>

<details>
<summary><b>Operational notes</b></summary>

- Targets need an explicit port unless `:443`; plain HTTP is unsupported.
- OS trust-store verification is intentionally **off** — use `OPN_TLSKEYPIN`.
- `approval.db` (+ WAL sidecars) is always gitignored, never committed.

</details>

---

<div align="center">

## 🤝 Contributing

PRs welcome — see [`AGENTS.md`](AGENTS.md) for the workflow. BSD 3-Clause, see [LICENSE](LICENSE).

**💖 Sponsors:** [pvz.digital](https://pvz.digital) · [debitor.de](https://debitor.de)
**🧑‍🎨 UX Borg design:** [@Codebase-Torben](https://github.com/Codebase-Torben) & [@Jones71190](https://github.com/Jones71190)

<details>
<summary>📄 Citation</summary>

```bibtex
@misc{opnborg,
  author = {Michael Paepcke},
  title = {Selfhost-able OPNSense Appliance Configuration Management & Backup Portal},
  year = {2024},
  publisher = {GitHub},
  journal = {GitHub repository},
  howpublished = {\url{https://paepcke.de/opnborg}}
}
```

</details>

Made with ⚙️ by [Michael Paepcke](https://paepcke.de) — *the hive thanks you for your compliance.*

</div>
