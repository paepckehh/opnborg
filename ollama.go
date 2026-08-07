package opnborg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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
	// _ollamaMaxDiffBytes caps the unified-diff payload sent to the model so a
	// multi-megabyte full-config XML rotation does not overrun the model
	// context window. When a diff exceeds the cap the tail is dropped and a
	// truncation marker is appended so the model knows the input was clipped.
	_ollamaMaxDiffBytes = 96 * 1024
	// _ollamaGeneratePath is the REST endpoint appended to OPN_OLLAMA_URL.
	_ollamaGeneratePath = "/api/generate"
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
// XML firewall configuration semantics.
const _ollamaSystemPrompt = `You are a senior infrastructure and Unix firewall engineer with deep expertise in OPNsense and Unifi network appliances. You are reviewing an automated git commit produced by opnborg, a daemon that backs up OPNsense firewall configuration as XML and Unifi controller backups as .unf files.

Your task: read the unified diff below and author the git commit message for it.

Output contract (obey exactly, no preamble, no markdown fences):
- Line 1: a short, concise commit headline (imperative mood, <= 72 characters, no trailing period).
- Line 2: empty.
- Lines 3+: an extensive, detailed explanation of what this commit changes in the context of configuring an OPNsense XML firewall. Describe which configuration sections changed (e.g. firewall rules, aliases, interfaces, routing, NAT, VPN, IPSec, certificates, users, groups, services, plugins), what the previous state implied, and what the new state enables or restricts. When the diff is an OPNsense config.xml rotation, reason about the <opnsense>/<filter> rule tree, <aliases>, <interfaces>, <gateways>, <nat>, <ipsec>, <vpn>, <cert> and related subtrees you can infer from the diff. Do not invent facts not supported by the diff. Keep the explanation grounded and technical.

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

// gitDiffText computes a unified diff between the HEAD tree and the working
// tree of the repository. It reads working-tree files via the worktree
// filesystem and HEAD blobs via the object store, so it never needs to build
// an intermediate tree from the index. The result is capped at
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
	var buf bytes.Buffer
	fs := wtree.Filesystem
	for pth, st := range status {
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
			Context:  3,
		})
		if err != nil {
			return "", err
		}
		buf.WriteString(diff)
		if buf.Len() > _ollamaMaxDiffBytes {
			break
		}
	}
	out := buf.String()
	if len(out) > _ollamaMaxDiffBytes {
		out = out[:_ollamaMaxDiffBytes] + "\n--- DIFF TRUNCATED ---\n"
	}
	return out, nil
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
