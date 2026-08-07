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
)

// _ollamaSystemPrompt is the persona and output contract sent to the model. It
// instructs the model to act as an infrastructure / Unix firewall expert and
// return a strict two-part commit message: a single short headline line
// followed by a blank line and an extensive explanation grounded in OPNsense
// XML firewall configuration semantics. The diff payload is enriched with a
// commit-level summary, per-file metadata (change kind, byte/line sizes, +/-
// counts, detected OPNsense XML top-level sections), widened context, and for
// small files the full resulting content; the prompt tells the model how to
// read that structure so it can ground its description in concrete change
// geometry rather than only the raw hunks.
const _ollamaSystemPrompt = `You are a senior infrastructure and Unix firewall engineer with deep expertise in OPNsense and Unifi network appliances. You are reviewing an automated git commit produced by opnborg, a daemon that backs up OPNsense firewall configuration as XML and Unifi controller backups as .unf files.

Your task: read the enriched diff below and author the git commit message for it.

The diff is structured. Use every section to ground your description:
- The "=== COMMIT SUMMARY ===" block lists every changed file with its change kind (added/modified/deleted/renamed/copied), per-file +insertions/-deletions counts, and commit-wide totals. Use it to state the scope of the change up front.
- Each "=== FILE: <path> ===" block carries a "change:" line, byte and line sizes before/after, per-file insertions/deletions, and when present an "opnsense-xml-sections:" line naming the OPNsense <opnsense> top-level child elements touched by this file (e.g. filter, aliases, interfaces, gateways, nat, ipsec, vpn, cert, users, group, service, package). Anchor your explanation to those named subtrees.
- The "--- unified diff ---" section shows the actual hunks with widened context. Reason about the surrounding OPNsense XML structure (parent elements, sibling rules, aliases, interface names) the context reveals.
- The "--- full new content ---" section, when present, gives the complete resulting file (small files only). Use it to describe the full new state, not just the patch.

Output contract (obey exactly, no preamble, no markdown fences):
- Line 1: a short, concise commit headline (imperative mood, <= 72 characters, no trailing period).
- Line 2: empty.
- Lines 3+: an extensive, detailed explanation of what this commit changes in the context of configuring an OPNsense XML firewall. Describe which configuration sections changed (e.g. firewall rules, aliases, interfaces, routing, NAT, VPN, IPSec, certificates, users, groups, services, plugins), what the previous state implied, and what the new state enables or restricts. When the diff is an OPNsense config.xml rotation, reason about the <opnsense>/<filter> rule tree, <aliases>, <interfaces>, <gateways>, <nat>, <ipsec>, <vpn>, <cert> and related subtrees you can infer from the diff and the opnsense-xml-sections metadata. Do not invent facts not supported by the diff. Keep the explanation grounded and technical.

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

// ollamaPrompt assembles the full prompt sent to the model: the system persona
// plus the unified diff payload.
func ollamaPrompt(diff string) string {
	return _ollamaSystemPrompt + "\n\n--- BEGIN DIFF ---\n" + diff + "\n--- END DIFF ---\n"
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
	b.WriteString("=== FILE: " + stat.path + " ===\n")
	b.WriteString("change: " + stat.kind + "\n")
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

// generateCommitMessage produces the commit message for an upcoming backup
// commit. When Ollama-assisted generation is disabled (or the change set is
// entirely Unifi .unf files) the default message _commitMsg is returned so the
// commit proceeds without any network round-trip. When enabled, the HEAD vs
// worktree diff is sent to the model; on any error or empty response the
// default message is used as a fallback so a model outage never blocks the
// backup from being committed.
func generateCommitMessage(config *OPNCall, repo *git.Repository, wtree *git.Worktree) string {
	if !config.Ollama.Enable {
		return _commitMsg
	}
	status, err := wtree.Status()
	if err != nil {
		displayChan <- []byte("[OLLAMA][STATUS][FAIL] " + err.Error())
		return _commitMsg
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
	msg, err := ollamaGenerate(config, ollamaPrompt(diff))
	if err != nil {
		displayChan <- []byte("[OLLAMA][FAIL] " + err.Error())
		return _commitMsg
	}
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return _commitMsg
	}
	displayChan <- []byte("[OLLAMA][OK] commit message generated")
	return msg
}
