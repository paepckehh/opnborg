package opnborg

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/pmezard/go-difflib/difflib"
)

const (
	// _ollamaTimeout caps how long a single diff summarisation may block the
	// main loop before opnborg gives up and falls back to the default commit
	// message. A local Ollama daemon answers in a few seconds; the ceiling just
	// guards against a wedged model server stalling the backup cadence.
	_ollamaTimeout = 2 * time.Minute
	// _ollamaMaxDiffBytes caps the enriched diff payload sent to the model so a
	// multi-megabyte full-config XML rotation does not overrun the model context
	// window. The cap is generous because the enriched diff carries per-file
	// metadata, change stats, and detected OPNsense XML section names alongside
	// the unified hunks, giving the model structured context to ground its
	// commit message in. When a diff exceeds the cap the tail is dropped and a
	// truncation marker is appended so the model knows the input was clipped.
	_ollamaMaxDiffBytes = 256 * 1024
	// _ollamaDiffContext is the number of unchanged context lines emitted around
	// each unified-diff hunk. A wider context window than the default 3 lines
	// lets the model see surrounding OPNsense XML structure (parent elements,
	// sibling rules, aliases, interface names) so it can describe not just what
	// changed but where in the configuration tree the change lives.
	_ollamaDiffContext = 8
	// _ollamaSmallFileBytes is the threshold below which the full new file
	// content is appended to its diff block (in addition to the unified hunks).
	// For small config fragments this lets the model reason about the complete
	// resulting state rather than just the patch, yielding richer descriptions.
	_ollamaSmallFileBytes = 16 * 1024
	// _ollamaGeneratePath is the REST endpoint appended to OLLAMA_DESC_URL.
	_ollamaGeneratePath = "/api/generate"
	// _ollamaTagsPath is the REST endpoint that lists the models currently
	// available on the Ollama daemon. The config dashboard probes it to report
	// server reachability, REST API readiness, and whether the configured
	// model is loaded. /api/tags is the canonical lightweight GET: it does not
	// load or run a model, so it is safe to call on every dashboard render.
	_ollamaTagsPath = "/api/tags"
	// _ollamaHealthTimeout caps a dashboard health probe so a wedged or
	// unreachable Ollama daemon never stalls the page render. The probe is run
	// synchronously on each config dashboard GET.
	_ollamaHealthTimeout = 3 * time.Second
	// _unifiBackupExt is the file extension of Unifi autoBackup archives.
	// Commits whose changed files are all .unf are committed with the default
	// message without consulting the model: the binary Unifi backup format is
	// not human-readable, so an LLM summary adds no value.
	_unifiBackupExt = ".unf"
	// _unifiAutobackupSubject is the commit subject used when a changeset
	// touches any file under the Unifi autoBackup store (a path containing
	// the "unifi-autobackup" segment). Such changes are synced from the
	// Unifi controller's autoBackup folder as opaque archives opnborg cannot
	// meaningfully describe, so commit-message generation is skipped and the
	// subject is set to this constant verbatim.
	_unifiAutobackupSubject = "unifi-autobackup"
	// _unifiAutobackupTag is the security-impact tag line appended to every
	// unifi-autobackup commit message. It classifies the change as "low"
	// severity (a routine backup rotation with no security impact) and
	// carries the "backup" flag so the BorgAUDIT page can distinguish
	// appliance config commits from plain Unifi backup rotations.
	_unifiAutobackupTag = "tag: low, backup"
)

// _ollamaSystemPrompt is the persona and output contract sent to the model. It
// instructs the model to act as an infrastructure / Unix firewall expert and
// return a strict multi-part commit message: a single short headline line,
// a blank line, and a structured, on-point security and impact review
// grounded in OPNsense XML firewall configuration semantics, ending with a
// trailing "tag:" line carrying an automated security-impact classification
// (low / medium / high / critical, plus an optional "needs-review" flag) so
// commits can be triaged by risk. The body uses labelled one-liners and
// tight bullet lists rather than prose paragraphs, covering, in order: the
// affected appliance(s), the scope of the change per configuration section,
// the previous-vs-new state, the functional impact on firewall behaviour, a
// security impact analysis (attack surface, hardening posture, auth/credential
// exposure, certificate/PKI implications, confidentiality/data-path exposure,
// blast radius, and named risk patterns such as shadowed rules, asymmetric
// routing, NAT exposure, WAN admin enablement, default-deny erosion, and
// orphaned objects), compliance/operational implications (change management,
// audit trail, HA-sync propagation, rollback), and risks/caveats/ambiguities
// distinguishing confirmed facts from inferences. The model is told to be
// concise and factual: no padding, no restating the diff, only what a senior
// reviewer needs to triage the change. The diff payload is enriched with a
// commit-level summary, per-file metadata (change kind, byte/line sizes, +/-
// counts, detected OPNsense XML top-level sections), widened context, and for
// small files the full resulting content; the prompt tells the model how to
// read that structure so it can ground its description in concrete change
// geometry rather than only the raw hunks. The prompt also carries the server
// name(s) the changeset applies to (extracted from the first path segment of
// each changed file) as explicit input, so the model can anchor its
// description to the affected appliance rather than having to infer it from
// the diff paths.
const _ollamaSystemPrompt = `You are a senior infrastructure and Unix firewall engineer with deep expertise in OPNsense and Unifi network appliances. You are reviewing an automated git commit produced by opnborg, a daemon that backs up OPNsense firewall configuration as XML and Unifi controller backups as .unf files.

Your task: read the enriched diff below and author the git commit message for it.

The backup store layout puts each appliance's files under a top-level folder named after the server (e.g. "fw01.lan/current.xml" belongs to server fw01.lan). The "Affected server(s):" line below lists the server name(s) derived from the first path segment of every changed file. Use it as explicit input.

The diff is structured. Use every section to ground your description:
- "=== COMMIT SUMMARY ===": changed files with change kind (added/modified/deleted/renamed/copied), per-file +insertions/-deletions, commit-wide totals.
- "=== FILE: <path> ===": a "change:" line, byte/line sizes before/after, per-file insertions/deletions, and when present an "opnsense-xml-sections:" line naming the OPNsense <opnsense> top-level child elements touched (e.g. filter, aliases, interfaces, gateways, nat, ipsec, vpn, cert, users, group, service, package).
- "--- unified diff ---": the actual hunks with widened context. Reason about the surrounding OPNsense XML structure (parent elements, sibling rules, aliases, interface names) the context reveals.
- "--- full new content ---": when present, the complete resulting file (small files only). Use it to describe the full new state, not just the patch.

Output contract (obey exactly, no preamble, no markdown fences):
- Line 1: a short, concise commit headline (imperative mood, <= 72 characters, no trailing period).
- Line 2: empty.
- Lines 3+: a structured, on-point security and impact review. Be concise and factual: no padding, no restating the diff, no paragraphs of prose. Use the labelled sections below with tight bullet lists; each bullet one line, naming concrete identifiers (rules, interfaces, ports, protocols, networks, aliases) as they appear in the diff. Skip a section only when the diff does not touch anything it would cover. Cover, in this order, with these exact labels:

  Appliance: <server(s)> — one line framing the change on that appliance.

  Scope:
  - <section>: <added/modified/removed> <what> — one bullet per changed section, citing opnsense-xml-sections and the hunks.

  Previous -> New:
  - <section>: <prior behaviour> -> <new behaviour> — concrete before/after per affected section.

  Functional impact:
  - <traffic/reachability/HA/sync/management-plane/VPN/IPSec/identity> — one bullet per affected concern.

  Security impact:
  - Attack surface: <exposed ports/services, widened scopes, any-to-any/any-to-self rules> or "none".
  - Hardening: <relaxed/tightened, removed safeguards, allow-bypass> or "none".
  - Auth/credentials: <admin/API/key/cert/user/SSH-key changes> or "none".
  - Cert/PKI: <issued/rotated/revoked/expired/weakened, IPSec/mTLS posture> or "none".
  - Confidentiality/data-path: <new interception/logging/capture surfaces> or "none".
  - Blast radius: <local to one interface/zone vs whole perimeter/all VS> — one line.
  - Named risks: <shadowed rules, asymmetric routing, NAT exposure, WAN admin enablement, default-deny erosion, cleartext, over-broad CIDRs, unused/duplicate aliases, orphaned objects> — list only those present, or "none".

  Compliance/ops:
  - <change management / audit trail / retention / rollback / HA-sync propagation> — one bullet per applicable concern, or "none".

  Risks/caveats:
  - <what an operator must verify that the diff alone cannot confirm> — distinguish "confirmed (diff)" from "inferred".

  Ground every claim in the diff. Do not invent facts; mark inferences as inference. When the diff is an OPNsense config.xml rotation, reason about the <opnsense>/<filter>, <aliases>, <interfaces>, <gateways>, <nat>, <ipsec>, <vpn>, <cert>, <users>, <group>, <service>, <package>, <dhcpd>, <dnsmasq>, <unbound>, <cron>, <syslog>, <snmpd> and related subtrees you can infer from the diff and the opnsense-xml-sections metadata. Prefer depth over volume: cover everything that matters, nothing that does not.

- Final line: a single "tag:" line that classifies the security impact of the change so commits can be triaged by risk. Format it exactly as:

  tag: <severity>[, needs-review]

  where <severity> is one of: low, medium, high, critical.
  - low: routine or no security impact (e.g. alias description edit, logging tweak, cosmetic rename).
  - medium: changes hardening or exposure in a bounded way (e.g. tightened a rule, rotated a certificate, adjusted an interface).
  - high: broadens attack surface or weakens hardening (e.g. new any-to-any allow rule, opened a WAN port, disabled a safeguard).
  - critical: removes a key control or broadly exposes a sensitive service (e.g. deleted a drop rule, enabled WAN admin, removed IPsec, weakened mTLS).
  The tag severity must be the synthesis of the security impact section above: weigh every confirmed and inferred risk, the blast radius, and whether the change is reversible. Append ", needs-review" when a human should inspect the change before it ships (e.g. high/critical severity, ambiguous intent, any auth/cert/IPsec/firewall-defaults change, or an inference in the analysis that could not be confirmed from the diff). Keep the tag line to a single line, no prose after it.

If the diff is empty or unintelligible, return exactly: opnborg auto update`

// ollamaGenerateRequest is the JSON body POSTed to the Ollama /api/generate
// REST endpoint with stream=false so the whole response arrives in one shot.
type ollamaGenerateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

// ollamaGenerateResponse is the JSON body returned by Ollama /api/generate
// when stream=false. Only the Response field carries the generated text.
type ollamaGenerateResponse struct {
	Response string `json:"response"`
}

// ollamaPrompt assembles the full prompt sent to the model: the system persona,
// the affected server name(s) derived from the first path segment of each
// changed file, and the unified diff payload. The server list is injected as
// explicit input so the model can anchor its description to the affected
// appliance(s) rather than having to infer them from the diff paths.
func ollamaPrompt(servers []string, diff string) string {
	serverLine := "(none)"
	if len(servers) > 0 {
		serverLine = strings.Join(servers, ", ")
	}
	return _ollamaSystemPrompt + "\n\nAffected server(s): " + serverLine + "\n\n--- BEGIN DIFF ---\n" + diff + "\n--- END DIFF ---\n"
}

// extractServersFromStatus returns the deduplicated, sorted list of server
// names derived from the first path segment of every changed file in the
// worktree status. The opnborg backup store lays out each appliance's files
// under a top-level folder named after the server (e.g.
// "fw01.lan/current.xml" -> "fw01.lan"), so the first segment is the
// authoritative server identifier. Root-level files with no slash (rare; e.g.
// a stray ".gitignore") contribute no server and are skipped. An empty or nil
// status yields an empty (non-nil) slice.
func extractServersFromStatus(status git.Status) []string {
	seen := make(map[string]struct{})
	for pth := range status {
		idx := strings.IndexByte(pth, '/')
		if idx <= 0 {
			continue
		}
		srv := pth[:idx]
		if _, hit := seen[srv]; !hit {
			seen[srv] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for srv := range seen {
		out = append(out, srv)
	}
	sort.Strings(out)
	return out
}

// ollamaGenerate POSTs the prompt to the configured Ollama model and returns
// the generated text. It is the single network call site for the feature.
func ollamaGenerate(config *OPNCall, prompt string) (string, error) {
	endpoint := strings.TrimRight(config.Ollama.URL, "/") + _ollamaGeneratePath
	body, err := json.Marshal(ollamaGenerateRequest{
		Model:  config.Ollama.Model,
		Prompt: prompt,
		Stream: false,
	})
	if err != nil {
		return "", fmt.Errorf("ollama marshal: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), _ollamaTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", _app+"/"+SemVer)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama call %s: %w", endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama %s: HTTP %s", endpoint, resp.Status)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("ollama read: %w", err)
	}
	var out ollamaGenerateResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("ollama decode: %w", err)
	}
	return out.Response, nil
}

// ollamaTagsModel is one entry in the /api/tags response model list.
type ollamaTagsModel struct {
	Name string `json:"name"`
}

// ollamaTagsResponse is the JSON body returned by Ollama /api/tags. It lists
// the models currently materialised on the daemon (i.e. already pulled), which
// is what the config dashboard uses to decide whether the configured model is
// "ready" without having to run a generation.
type ollamaTagsResponse struct {
	Models []ollamaTagsModel `json:"models"`
}

// ollamaHealth is the result of a config-dashboard probe against the Ollama
// daemon. The three booleans are layered: ServerReachable means a TCP/HTTP
// connection succeeded; APIReady means the daemon answered /api/tags with a
// parseable JSON body (so the REST API is up); ModelReady means the configured
// model is present in that list (i.e. already pulled and available to run).
// Err carries the first failure reason, if any, for display.
type ollamaHealth struct {
	ServerReachable bool
	APIReady        bool
	ModelReady      bool
	Err             string
}

// ollamaHealthCheck probes the configured Ollama daemon and reports server
// reachability, REST API readiness, and whether the configured model is loaded.
// It is called from the config dashboard on every render, so it uses a short
// timeout (_ollamaHealthTimeout) and never blocks the page on a wedged daemon.
// When Ollama is not enabled the returned health is all-false with no probe.
func ollamaHealthCheck(config *OPNCall) ollamaHealth {
	var h ollamaHealth
	if !config.Ollama.Enable {
		return h
	}
	endpoint := strings.TrimRight(config.Ollama.URL, "/") + _ollamaTagsPath
	ctx, cancel := context.WithTimeout(context.Background(), _ollamaHealthTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		h.Err = fmt.Sprintf("build request: %v", err)
		return h
	}
	req.Header.Set("User-Agent", _app+"/"+SemVer)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.Err = fmt.Sprintf("server unreachable: %v", err)
		return h
	}
	defer resp.Body.Close()
	h.ServerReachable = true
	if resp.StatusCode != http.StatusOK {
		h.Err = fmt.Sprintf("api: HTTP %s", resp.Status)
		return h
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		h.Err = fmt.Sprintf("read body: %v", err)
		return h
	}
	var tags ollamaTagsResponse
	if err := json.Unmarshal(raw, &tags); err != nil {
		h.Err = fmt.Sprintf("decode /api/tags: %v", err)
		return h
	}
	h.APIReady = true
	model := strings.TrimSpace(config.Ollama.Model)
	for _, m := range tags.Models {
		if ollamaModelMatch(m.Name, model) {
			h.ModelReady = true
			break
		}
	}
	if !h.ModelReady {
		h.Err = fmt.Sprintf("model %q not in /api/tags (have %d models)", model, len(tags.Models))
	}
	return h
}

// ollamaModelMatch reports whether a model name returned by /api/tags
// corresponds to the configured model. Ollama returns fully-qualified names
// such as "llama3:latest" while the configured OLLAMA_DESC_MODEL may be the bare
// "llama3", so an exact match or a "<model>:<tag>" prefix match both count.
func ollamaModelMatch(have, want string) bool {
	if have == "" || want == "" {
		return false
	}
	if have == want {
		return true
	}
	return strings.HasPrefix(have, want+":")
}

// onlyUnifiChanges reports whether every changed file in the worktree status
// is a Unifi .unf backup. Such commits are committed with the default message
// without consulting the model: the .unf format is an opaque binary archive an
// LLM cannot meaningfully summarise. An empty status returns false so a clean
// worktree is never misclassified.
func onlyUnifiChanges(status git.Status) bool {
	if len(status) == 0 {
		return false
	}
	for p := range status {
		if !strings.HasSuffix(p, _unifiBackupExt) {
			return false
		}
	}
	return true
}

// hasUnifiAutobackupChange reports whether any changed file in the worktree
// status lives under the Unifi autoBackup store, i.e. its path contains the
// "unifi-autobackup" segment. opnborg syncs Unifi controller autoBackup
// archives into the store as opaque files it cannot meaningfully describe, so
// when such a file is part of a changeset the Ollama commit-message generation
// is skipped entirely and the commit subject is set to
// _unifiAutobackupSubject verbatim. This catches both pure-Unifi changesets
// and mixed changesets where a Unifi autoBackup rotation lands in the same
// commit as an OPNsense XML backup.
func hasUnifiAutobackupChange(status git.Status) bool {
	for p := range status {
		if strings.Contains(p, _unifiAutobackupSubject) {
			return true
		}
	}
	return false
}

// fileDiffStat summarises a single changed file for the enriched diff. The
// model uses it to ground its commit message in concrete change geometry
// (which file, what kind of change, how big, which OPNsense XML sections are
// touched) instead of only the raw unified hunks.
type fileDiffStat struct {
	path       string
	kind       string
	insertions int
	deletions  int
	oldBytes   int
	newBytes   int
	oldLines   int
	newLines   int
	sections   []string
}

// changeKind maps a go-git FileStatus to a human-readable change label that the
// model can reason about: added, modified, deleted, renamed, copied, or
// untracked. The worktree status is the authoritative signal because the diff
// is computed against HEAD directly.
func changeKind(st *git.FileStatus) string {
	switch {
	case st.Worktree == git.Untracked:
		return "added"
	case st.Worktree == git.Deleted:
		return "deleted"
	case st.Staging == git.Renamed || st.Worktree == git.Renamed:
		return "renamed"
	case st.Staging == git.Copied || st.Worktree == git.Copied:
		return "copied"
	case st.Worktree == git.Modified, st.Staging == git.Modified:
		return "modified"
	default:
		return "modified"
	}
}

// countHunkAddsDels walks the unified-diff text for a file and counts the lines
// added (lines starting with '+', excluding the '+++' header) and removed
// (lines starting with '-', excluding the '---' header). These numbers feed
// the per-file and commit-level stats blocks so the model sees the change
// magnitude at a glance.
func countHunkAddsDels(diff string) (int, int) {
	var ins, del int
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+++"):
			continue
		case strings.HasPrefix(line, "---"):
			continue
		case strings.HasPrefix(line, "+"):
			ins++
		case strings.HasPrefix(line, "-"):
			del++
		}
	}
	return ins, del
}

// xmlTopLevelSections inspects the old and new file content for OPNsense
// <opnsense> configuration XML and returns the names of the direct child
// elements of <opnsense> that appear in either side. This lets the model anchor
// its description to the affected configuration subtrees (filter, aliases,
// interfaces, gateways, nat, ipsec, vpn, cert, users, ...). Non-XML files or
// malformed XML yield an empty slice; the function never errors so a parse
// failure can never block the diff.
func xmlTopLevelSections(old, new string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, content := range []string{old, new} {
		dec := xml.NewDecoder(strings.NewReader(content))
		depth := 0
		for {
			tok, err := dec.Token()
			if err != nil {
				break
			}
			start, ok := tok.(xml.StartElement)
			if !ok {
				continue
			}
			depth++
			// The first StartElement is the document root (<opnsense> for a
			// config.xml). Its direct children (depth == 2) are the top-level
			// configuration sections we want to surface.
			if depth == 2 {
				name := start.Name.Local
				if _, hit := seen[name]; !hit {
					seen[name] = struct{}{}
					out = append(out, name)
				}
			}
		}
	}
	return out
}

// renderCommitSummary produces the header block prefixed to the enriched diff.
// It lists every changed file with its change kind and +/- line counts plus the
// commit-wide totals, giving the model a compact overview before it reads the
// per-file hunks.
func renderCommitSummary(stats []fileDiffStat, totalIns, totalDel int) string {
	var b strings.Builder
	b.WriteString("=== COMMIT SUMMARY ===\n")
	fmt.Fprintf(&b, "files changed: %d\n", len(stats))
	fmt.Fprintf(&b, "total insertions: %d\n", totalIns)
	fmt.Fprintf(&b, "total deletions: %d\n", totalDel)
	b.WriteString("files:\n")
	for _, s := range stats {
		fmt.Fprintf(&b, "  - %s (%s, +%d/-%d)\n", s.path, s.kind, s.insertions, s.deletions)
	}
	b.WriteString("=== END SUMMARY ===\n\n")
	return b.String()
}

// renderFileBlock produces the enriched per-file block: a metadata header
// (change kind, byte/line sizes, detected XML sections), the unified hunks with
// widened context, and for small files the full resulting content so the model
// can describe the complete new state rather than only the patch.
func renderFileBlock(stat fileDiffStat, diff, workContent string) string {
	var b strings.Builder
	b.WriteString("=== FILE: ")
	b.WriteString(stat.path)
	b.WriteString(" ===\n")
	b.WriteString("change: ")
	b.WriteString(stat.kind)
	b.WriteString("\n")
	fmt.Fprintf(&b, "size: %d bytes -> %d bytes\n", stat.oldBytes, stat.newBytes)
	fmt.Fprintf(&b, "lines: %d -> %d\n", stat.oldLines, stat.newLines)
	fmt.Fprintf(&b, "insertions: %d, deletions: %d\n", stat.insertions, stat.deletions)
	if len(stat.sections) > 0 {
		b.WriteString("opnsense-xml-sections: " + strings.Join(stat.sections, ", ") + "\n")
	}
	b.WriteString("--- unified diff ---\n")
	b.WriteString(diff)
	if stat.kind != "deleted" && len(workContent) > 0 && len(workContent) <= _ollamaSmallFileBytes {
		b.WriteString("--- full new content ---\n")
		b.WriteString(workContent)
		if !strings.HasSuffix(workContent, "\n") {
			b.WriteString("\n")
		}
	}
	b.WriteString("=== END FILE ===\n\n")
	return b.String()
}

// gitDiffText computes an enriched unified diff between the HEAD tree and the
// working tree of the repository. Beyond the raw hunks it emits a commit-level
// summary, per-file metadata (change kind, byte/line sizes, +/- counts,
// detected OPNsense XML top-level sections), widened context lines, and for
// small files the full resulting content. It reads working-tree files via the
// worktree filesystem and HEAD blobs via the object store, so it never needs to
// build an intermediate tree from the index. The result is capped at
// _ollamaMaxDiffBytes; an over-long diff is truncated with a marker so the
// model knows the input was clipped. An empty string is returned when there is
// no HEAD yet (first commit) or no changes to diff.
func gitDiffText(repo *git.Repository, wtree *git.Worktree) (string, error) {
	head, err := repo.Head()
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return "", nil
		}
		return "", err
	}
	headCommit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return "", err
	}
	headTree, err := headCommit.Tree()
	if err != nil {
		return "", err
	}
	status, err := wtree.Status()
	if err != nil {
		return "", err
	}
	if status.IsClean() {
		return "", nil
	}
	// Collect and sort changed paths so the diff is deterministic across runs
	// and across hosts, regardless of map iteration order.
	paths := make([]string, 0, len(status))
	for pth := range status {
		paths = append(paths, pth)
	}
	sort.Strings(paths)
	var buf bytes.Buffer
	var totalIns, totalDel int
	fs := wtree.Filesystem
	stats := make([]fileDiffStat, 0, len(paths))
	for _, pth := range paths {
		st := status[pth]
		// Skip deleted-from-worktree files that are also absent from HEAD
		// (pure index bookkeeping); a real deletion still diffs against the
		// HEAD blob below.
		headContent, err := headFileContent(headTree, pth)
		if err != nil {
			return "", err
		}
		var workContent string
		if st.Worktree != git.Deleted {
			if f, err := fs.Open(pth); err == nil {
				b, rerr := io.ReadAll(f)
				f.Close()
				if rerr != nil {
					return "", rerr
				}
				workContent = string(b)
			}
		}
		diff, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
			A:        difflib.SplitLines(headContent),
			FromFile: "a/" + pth,
			B:        difflib.SplitLines(workContent),
			ToFile:   "b/" + pth,
			Context:  _ollamaDiffContext,
		})
		if err != nil {
			return "", err
		}
		ins, del := countHunkAddsDels(diff)
		stat := fileDiffStat{
			path:       pth,
			kind:       changeKind(st),
			insertions: ins,
			deletions:  del,
			oldBytes:   len(headContent),
			newBytes:   len(workContent),
			oldLines:   len(difflib.SplitLines(headContent)),
			newLines:   len(difflib.SplitLines(workContent)),
			sections:   xmlTopLevelSections(headContent, workContent),
		}
		stats = append(stats, stat)
		totalIns += ins
		totalDel += del
		buf.WriteString(renderFileBlock(stat, diff, workContent))
		if buf.Len() > _ollamaMaxDiffBytes {
			break
		}
	}
	out := buf.String()
	if len(out) > _ollamaMaxDiffBytes {
		out = out[:_ollamaMaxDiffBytes] + "\n--- DIFF TRUNCATED ---\n"
	}
	if out == "" {
		return "", nil
	}
	return renderCommitSummary(stats, totalIns, totalDel) + out, nil
}

// headFileContent returns the textual content of the file at pth in the HEAD
// tree, or an empty string when the file is absent from HEAD (a newly added
// file). A read error is propagated.
func headFileContent(tree *object.Tree, pth string) (string, error) {
	entry, err := tree.FindEntry(pth)
	if err != nil {
		if errors.Is(err, object.ErrEntryNotFound) || errors.Is(err, object.ErrFileNotFound) {
			return "", nil
		}
		return "", err
	}
	f, err := tree.TreeEntryFile(entry)
	if err != nil {
		return "", err
	}
	return f.Contents()
}

// _ollamaTldrPrompt is the prompt sent to the model in the second REST call,
// after the detailed analysis has been generated. It asks the model to distil
// its own detailed analysis into a single TLDR sentence so the commit message
// can lead with a one-line summary and keep the exhaustive review as the
// "Detailed Analysis:" body. The model is told to return only the TLDR line
// (no preamble, no markdown fences, no "tag:" line, no quotation) so the
// result can be embedded verbatim as the first line of the commit message.
const _ollamaTldrPrompt = `You are a senior infrastructure and Unix firewall engineer. Below is the detailed security and impact analysis you just authored for an automated opnborg backup commit of an OPNsense firewall configuration.

Summarise that analysis into a single, concise TLDR sentence that captures the essence of the change and its security impact. The sentence must be self-contained (it will be the headline of the git commit message) and written in plain prose, no bullet points, no markdown.

Output contract (obey exactly, no preamble, no markdown fences):
- Return exactly one line.
- Start with "TLDR: " followed by the summary sentence.
- No trailing newline, no extra prose, no "tag:" line, no quotation marks.
- Keep it under 160 characters.

If the analysis is empty or unintelligible, return exactly: TLDR: opnborg auto update

--- BEGIN DETAILED ANALYSIS ---
%s
--- END DETAILED ANALYSIS ---`

// generateCommitMessage produces the commit message for an upcoming backup
// commit. When Ollama-assisted generation is disabled (or the change set is
// entirely Unifi .unf files) the default message _commitMsg is returned so the
// commit proceeds without any network round-trip. When the change set touches
// any file under the Unifi autoBackup store (a path containing
// "unifi-autobackup") the model generation is skipped and the commit subject
// is set to _unifiAutobackupSubject verbatim, since those archives are opaque
// Unifi controller rotations opnborg cannot meaningfully describe. When
// enabled and the change set is describable, two REST calls are made to the
// model: the first produces the exhaustive detailed analysis (headline +
// in-depth security/impact review + trailing "tag:" severity line), and the
// second distils that analysis into a single TLDR sentence. The final commit
// message is assembled as the TLDR line on top, a blank line, a "Detailed
// Analysis:" marker, and the full detailed analysis underneath. On any error
// or empty response at either stage the default message is used as a fallback
// so a model outage never blocks the backup from being committed.
func generateCommitMessage(config *OPNCall, repo *git.Repository, wtree *git.Worktree) string {
	if !config.Ollama.Enable {
		return _commitMsg
	}
	status, err := wtree.Status()
	if err != nil {
		displayChan <- []byte("[OLLAMA][STATUS][FAIL] " + err.Error())
		return _commitMsg
	}
	if hasUnifiAutobackupChange(status) {
		return _unifiAutobackupSubject + "\n\n" + _unifiAutobackupTag + "\n"
	}
	if onlyUnifiChanges(status) {
		return _commitMsg
	}
	diff, err := gitDiffText(repo, wtree)
	if err != nil {
		displayChan <- []byte("[OLLAMA][DIFF][FAIL] " + err.Error())
		return _commitMsg
	}
	if strings.TrimSpace(diff) == "" {
		return _commitMsg
	}
	servers := extractServersFromStatus(status)
	msg, err := ollamaGenerate(config, ollamaPrompt(servers, diff))
	if err != nil {
		displayChan <- []byte("[OLLAMA][FAIL] " + err.Error())
		return _commitMsg
	}
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return _commitMsg
	}
	// Second pass: ask the model to resume its detailed analysis as a single
	// TLDR sentence, then assemble the final commit message with the TLDR
	// on top and the full detailed analysis under a "Detailed Analysis:"
	// marker. A failure here degrades gracefully to the detailed analysis
	// alone (no TLDR header) so the commit still ships with the rich body.
	tldr, terr := ollamaGenerate(config, fmt.Sprintf(_ollamaTldrPrompt, msg))
	tldr = normalizeTldrHeadline(tldr)
	if terr != nil || tldr == "" {
		displayChan <- []byte("[OLLAMA][TLDR][FAIL] " + fallbackTldrErr(terr))
		displayChan <- []byte("[OLLAMA][OK] commit message generated (detailed analysis only)")
		return msg
	}
	displayChan <- []byte("[OLLAMA][OK] commit message generated (TLDR + detailed analysis)")
	return assembleAnnotatedCommitMessage(tldr, msg)
}

// normalizeTldrHeadline coerces the model's TLDR response into the canonical
// "TLDR: <sentence>" form so the downstream author-name extraction and the
// audit-page message splitter both reliably recognise it. Models do not
// always obey the one-line / "TLDR: " contract: they may wrap the marker in
// markdown emphasis ("**TLDR:**"), drop the space after the colon, prepend a
// preamble line, or omit the marker entirely. This function takes the first
// non-empty line, strips leading markdown / quote characters, recognises a
// case-insensitive "TLDR" marker optionally followed by markdown and a colon
// (stripping it so the canonical prefix is re-added uniformly), and prepends
// "TLDR: " when no marker is present. An empty input yields an empty result
// so the caller can fall back to the detailed-analysis-only message.
// normalizeTldrHeadline coerces the model's TLDR response into the canonical
// "TLDR: <sentence>" form so the downstream author-name extraction and the
// audit-page message splitter both reliably recognise it. Models do not
// always obey the one-line / "TLDR: " contract: they may wrap the marker in
// markdown emphasis ("**TLDR:**"), drop the space after the colon, prepend a
// preamble line, or omit the marker entirely. This function scans the lines
// for the first one carrying a TLDR marker (falling back to the first
// non-empty line), strips the marker plus any surrounding markdown so the
// canonical prefix is re-added uniformly, and prepends "TLDR: " when no
// marker is present. An empty input yields an empty result so the caller can
// fall back to the detailed-analysis-only message.
func normalizeTldrHeadline(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var headline string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		stripped := strings.TrimLeft(line, "*_`#>\"' ")
		if strings.HasPrefix(strings.ToLower(stripped), "tldr") {
			headline = stripped
			break
		}
		if headline == "" {
			headline = line
		}
	}
	if headline == "" {
		return ""
	}
	lower := strings.ToLower(headline)
	if strings.HasPrefix(lower, "tldr") {
		rest := headline[len("tldr"):]
		rest = strings.TrimLeft(rest, "*_`#")
		if len(rest) > 0 && rest[0] == ':' {
			rest = rest[1:]
			rest = strings.TrimLeft(rest, "*_`# ")
			headline = strings.TrimSpace(rest)
		}
	}
	if headline == "" {
		return ""
	}
	return _authorNameTldrPrefix + headline
}

// fallbackTldrErr renders a non-nil TLDR-stage error for the log line, or a
// generic "empty response" note when the model returned nothing usable.
func fallbackTldrErr(err error) string {
	if err == nil {
		return "empty response"
	}
	return err.Error()
}

// assembleAnnotatedCommitMessage builds the final commit message from the
// TLDR summary line and the full detailed analysis. The layout is:
//
//	TLDR: <summary line>
//
//	Detailed Analysis:
//	<detailed analysis, including the trailing tag: severity line>
//
// Exactly one blank line separates the TLDR from the "Detailed Analysis:"
// header, and exactly one blank line separates the header from the body.
func assembleAnnotatedCommitMessage(tldr, detailed string) string {
	tldr = strings.TrimSpace(tldr)
	detailed = strings.TrimSpace(detailed)
	return tldr + "\n\nDetailed Analysis:\n\n" + detailed + "\n"
}

// _authorNameMaxRunes caps the git commit author name derived from an
// Ollama TLDR headline. Git author names have no hard length limit, but the
// TLDR sentence can run up to the model's 160-character ceiling, which is too
// long for `git log --format=%an`. We cap at 72 runes (the conventional commit
// headline width) so the commit log stays readable.
const _authorNameMaxRunes = 72

// _authorNameTldrPrefix is the leading marker of an Ollama TLDR headline. It is
// stripped before the headline is reused as the commit author name.
const _authorNameTldrPrefix = "TLDR: "

// authorFromCommitMessage returns a short, sanitised version of the TLDR
// headline carried by an Ollama-assisted commit message, suitable for use as
// the git commit author name in place of the static OPNBORG-AUTO-COMMIT
// handle. It takes the first line of the message, strips the leading "TLDR: "
// marker, collapses runs of whitespace into single spaces, drops control
// characters and the characters git's identity parser treats specially (<, >,
// ", \) so the resulting name is always a valid git identity, and truncates to
// _authorNameMaxRunes. An empty input, an input with no first line, or one
// whose first line carries no TLDR marker returns "" so the caller can fall
// back to the static _authorName handle.
func authorFromCommitMessage(msg string) string {
	firstLine := strings.TrimSpace(msg)
	if firstLine == "" {
		return ""
	}
	if i := strings.IndexByte(firstLine, '\n'); i >= 0 {
		firstLine = strings.TrimSpace(firstLine[:i])
	}
	if !strings.HasPrefix(firstLine, _authorNameTldrPrefix) {
		return ""
	}
	firstLine = strings.TrimSpace(firstLine[len(_authorNameTldrPrefix):])
	if firstLine == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(firstLine))
	lastSpace := false
	n := 0
	for _, r := range firstLine {
		switch {
		case r == ' ' || r == '\t':
			if !lastSpace {
				b.WriteRune(' ')
				lastSpace = true
				n++
			}
			continue
		case r < 0x20 || r == '<' || r == '>' || r == '"' || r == '\\':
			continue
		default:
			b.WriteRune(r)
			lastSpace = false
			n++
		}
		if n >= _authorNameMaxRunes {
			break
		}
	}
	return strings.TrimSpace(b.String())
}
