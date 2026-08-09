package opnborg

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
	gitcfg "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	xssh "golang.org/x/crypto/ssh"
)

const (
	_currentDir = "."
	_dotGit     = ".git"
	_gitignore  = ".gitignore"
	// _ignore is the canonical .gitignore content written into the storage
	// root. It keeps the archive/history files, the symlink targets, the
	// logs, and the on-disk security-approval ledger out of the commit
	// history. The approval ledger (approval.db and its SQLite WAL sidecars
	// approval.db-wal / approval.db-shm) is a local-only runtime database:
	// it must never be added, evaluated, or committed by the opnborg commit
	// cycle, so it is ignored alongside the other non-version-controlled
	// store artifacts. The "approval.db*" glob covers the main database and
	// every sidecar spelling in a single line.
	_ignore         = ".archive\nCONFIG*\nLogs\napproval.db*\n"
	_origin         = "origin"
	_commitMsg      = "opnborg auto update"
	_authorName     = "OPNBORG-AUTO-COMMIT"
	_defaultSSHUser = "git"
	// _aggressivePackWindow is the delta-compression window applied to the
	// storage repo so the per-tick repack behaves like `git gc --aggressive`
	// (git's aggressive default is pack.deltaWindow=250). It is written once
	// during gitInit.
	_aggressivePackWindow = uint(250)
	// _pruneGrace is how recently an unreachable loose object must have been
	// created to be spared from pruning, mirroring git's gc.pruneExpire safety
	// window so a concurrent tick never loses an object it still needs.
	_pruneGrace = time.Hour
)

// gitLastPush tracks the outcome of the most recent upstream push so the
// WebUI dashboard can surface upstream sync health. It is mutated only from
// gitPush (which runs inside gitCheckIn on the main loop) and read from the
// httpd render goroutine, so a dedicated mutex guards it.
var (
	gitLastPushMu  sync.Mutex
	gitLastPushTS  time.Time
	gitLastPushOK  bool
	gitLastPushMsg string
)

// gitRepo opens the existing git repository at the given path, or initialises a
// fresh non-bare repository when none is present yet. The path is expected to
// already exist (the storage root is created elsewhere); this only manages the
// .git metadata side of the folder.
func gitRepo(path string) (*git.Repository, error) {
	repo, err := git.PlainOpen(path)
	if err == nil {
		return repo, nil
	}
	if !errors.Is(err, git.ErrRepositoryNotExists) {
		return nil, err
	}
	return git.PlainInit(path, false)
}

// gitEnsureIgnore writes the .gitignore file in the storage root when it is
// missing, so the archive/history files, the symlink targets, the logs, and
// the on-disk security-approval ledger stay out of the commit history. When a
// .gitignore already exists it is reconciled rather than clobbered: the
// reconcile ensures the approval-ledger ignore line ("approval.db*") is present
// so the ledger database and its SQLite WAL sidecars (approval.db-wal /
// approval.db-shm) are never added, evaluated, or committed by the opnborg
// commit cycle. This migration matters because older opnborg releases
// deliberately stripped every ledger-ignore line so the ledger was
// version-controlled; that policy is inverted here; the ledger is now a
// local-only runtime database and must stay untracked. All other lines
// (including operator-added custom ignores) are preserved.
func gitEnsureIgnore(config *OPNCall) error {
	ignore := filepath.Join(config.Path, _gitignore)
	if _, err := os.Stat(ignore); err == nil {
		return reconcileGitignoreApprovalLedger(ignore)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.WriteFile(ignore, []byte(_ignore), 0660); err != nil {
		displayChan <- []byte("[GIT][REPO][ERROR][FAIL:UNABLE-TO-CREATE-GIT-IGNORE-FILE] " + config.Path)
		return err
	}
	return nil
}

// reconcileGitignoreApprovalLedger guarantees the .gitignore at ignore carries
// a line that ignores the security-approval ledger (the main database
// approval.db and every SQLite WAL sidecar approval.db-wal / approval.db-shm).
// When the file already contains such a line it is left untouched; otherwise
// the canonical "approval.db*" glob is appended on its own line. Operator-added
// custom ignores and the canonical archive/CONFIG/Logs lines are always
// preserved. The file is only rewritten when its content would change, so a
// clean .gitignore incurs no write.
func reconcileGitignoreApprovalLedger(ignore string) error {
	raw, err := os.ReadFile(ignore)
	if err != nil {
		return err
	}
	if gitignoreIgnoresApprovalLedger(string(raw)) {
		return nil
	}
	// Ensure the existing content ends with a newline so the appended line
	// lands on its own row, then append the canonical ledger glob.
	out := string(raw)
	if len(out) == 0 || out[len(out)-1] != '\n' {
		out += "\n"
	}
	out += "approval.db*\n"
	return os.WriteFile(ignore, []byte(out), 0660)
}

// gitignoreIgnoresApprovalLedger reports whether a whole .gitignore body
// already contains a line that would ignore the security-approval ledger: the
// main database file (approval.db) or any of its SQLite WAL sidecars
// (approval.db-wal, approval.db-shm). A gitignore pattern without a slash
// matches by basename at any depth, so a bare "approval.db",
// "approval.db-wal", "approval.db-shm", or a trailing-star
// "approval.db*" glob each satisfies the check. Blank lines, comments, and
// any pattern that does not reduce to one of the ledger filenames (with an
// optional leading slash) are reported as not ignoring the ledger.
func gitignoreIgnoresApprovalLedger(body string) bool {
	for line := range strings.SplitSeq(body, "\n") {
		if ignoresApprovalLedger(line) {
			return true
		}
	}
	return false
}

// ignoresApprovalLedger reports whether a single .gitignore line would ignore
// the security-approval ledger: the main database file (approval.db) or any of
// its SQLite WAL sidecars (approval.db-wal, approval.db-shm). A gitignore
// pattern without a slash matches by basename at any depth, so a bare
// "approval.db" or "approval.db*" line ignores the ledger everywhere in the
// store. The check is conservative: blank lines, comments, and any pattern
// that does not reduce to one of the ledger filenames (with an optional
// leading slash) are reported as not ignoring the ledger, so legitimate
// operator-added ignores are never treated as covering the ledger.
func ignoresApprovalLedger(line string) bool {
	// A range over SplitSeq yields an empty trailing element for a
	// newline-terminated input; an empty line never ignores anything.
	if strings.TrimSpace(line) == "" {
		return false
	}
	if strings.HasPrefix(strings.TrimSpace(line), "#") {
		return false
	}
	p := strings.TrimSpace(line)
	p = strings.TrimPrefix(p, "/")
	if strings.Contains(p, "/") {
		// An anchored path pattern is out of scope: the ledger lives at the
		// store root, so a nested ignore never targets it.
		return false
	}
	ledger := []string{_approvalDBName, _approvalDBName + "-wal", _approvalDBName + "-shm"}
	if slices.Contains(ledger, p) {
		return true
	}
	// A trailing-* glob anchored to the ledger basename (e.g. "approval.db*")
	// also matches the sidecars.
	if base, ok := strings.CutSuffix(p, "*"); ok {
		if base == _approvalDBName || strings.HasPrefix(base, _approvalDBName+"-") {
			return true
		}
	}
	return false
}

// isApprovalDBPath reports whether a worktree path is a file of the
// security-approval ledger (approval.db) or one of its SQLite WAL sidecars
// (approval.db-wal, approval.db-shm). It matches by base name so ledger files
// are detected regardless of which store subfolder they land in. gitCommit
// uses it to skip every approval-ledger path at staging time so a ledger
// update can never enter a commit, even when a stale or hand-edited .gitignore
// failed to ignore the file.
func isApprovalDBPath(p string) bool {
	base := filepath.Base(p)
	if !strings.HasPrefix(base, "approval") {
		return false
	}
	return strings.HasSuffix(base, ".db") || strings.Contains(base, ".db-")
}

// sshUserFromURL extracts the SSH user from an SSH git URL. It handles both the
// scp-style form ("user@host:path") and the ssh:// URI form
// ("ssh://user@host[:port]/path"). It falls back to the conventional "git"
// user when the URL does not carry an explicit one.
func sshUserFromURL(upstream string) string {
	if strings.HasPrefix(upstream, "ssh://") {
		rest := upstream[len("ssh://"):]
		if at := strings.IndexByte(rest, '@'); at > 0 {
			return rest[:at]
		}
		return _defaultSSHUser
	}
	if at := strings.IndexByte(upstream, '@'); at > 0 {
		return upstream[:at]
	}
	return _defaultSSHUser
}

// gitSSHAuth builds an SSH public-key auth method from the private key file
// at keyPath, using the user derived from the upstream URL. Host key
// verification never reads or writes a known_hosts file (go-git's default
// callback would fail with "cannot create known hosts callback: $HOME is not
// defined" in container/CI runs that unset HOME). Instead:
//
//   - When hostKeyFingerprint is empty, host key verification is skipped
//     entirely (insecure) so the upstream push works in unattended
//     environments. Pin a fingerprint whenever the upstream is reachable over
//     an untrusted network.
//   - When hostKeyFingerprint is set, only an upstream host whose presented
//     public key SHA-256 fingerprint matches is accepted; any other key
//     aborts the push.
//
// The fingerprint may be given either in the OpenSSH presentation form
// ("SHA256:<base64-no-pad>") or as the raw base64 body, and is matched
// case-insensitively against xssh.FingerprintSHA256(remote).
func gitSSHAuth(upstream, keyPath, hostKeyFingerprint string) (*ssh.PublicKeys, error) {
	auth, err := ssh.NewPublicKeysFromFile(sshUserFromURL(upstream), keyPath, "")
	if err != nil {
		return nil, fmt.Errorf("ssh key %s: %w", keyPath, err)
	}
	want := normalizeFingerprint(hostKeyFingerprint)
	if want == "" {
		auth.HostKeyCallback = xssh.InsecureIgnoreHostKey()
		return auth, nil
	}
	auth.HostKeyCallback = func(_ string, _ net.Addr, key xssh.PublicKey) error {
		got := normalizeFingerprint(xssh.FingerprintSHA256(key))
		if strings.EqualFold(got, want) {
			return nil
		}
		return fmt.Errorf("upstream host key fingerprint mismatch: got SHA256:%s, want SHA256:%s", got, want)
	}
	return auth, nil
}

// normalizeFingerprint reduces a host-key fingerprint to its comparable base64
// body so both "SHA256:<base64>" and bare "<base64>" inputs match. An empty
// input yields an empty result.
func normalizeFingerprint(fp string) string {
	fp = strings.TrimSpace(fp)
	if fp == "" {
		return ""
	}
	return strings.TrimPrefix(fp, _sshFingerprintPrefix)
}

// _sshFingerprintPrefix is the OpenSSH SHA-256 fingerprint prefix used by
// xssh.FingerprintSHA256 and ssh-keyscan output.
const _sshFingerprintPrefix = "SHA256:"

// _refspec is the default fetch/push refspec written into the origin remote so
// every local branch tracks its upstream counterpart. The leading "+" makes the
// push a forced update, which keeps the upstream in lockstep with the local
// auto-commit history even when histories diverge (e.g. after a manual upstream
// rewrite).
const _refspec = "+refs/heads/*:refs/heads/*"

// gitEnsureOrigin makes sure an "origin" remote pointing at upstream exists in
// the repository, recreating it when the recorded URL or refspecs drifted so
// the push targets the currently configured upstream and the right refs. A
// fresh remote is always created with the default head refspec so pushes have
// a valid mapping even when callers omit an explicit RefSpecs list.
//
// The URL check requires an exact single-URL match: go-git merges every
// configured fetch "url" and "pushurl" into one cfg.URLs slice (pushurls last)
// and Remote.Push pushes to cfg.URLs[len-1]. A stale pushurl would therefore
// pass a cfg.URLs[0] check while silently redirecting the push, so any extra
// URL (mismatched or merely redundant) is treated as drift and repaired.
func gitEnsureOrigin(repo *git.Repository, upstream string) error {
	rem, err := repo.Remote(_origin)
	if err == nil {
		cfg := rem.Config()
		urlDrift := len(cfg.URLs) != 1 || cfg.URLs[0] != upstream
		refspecDrift := len(cfg.Fetch) != 1 || cfg.Fetch[0] != gitcfg.RefSpec(_refspec)
		if urlDrift || refspecDrift {
			if err := repo.DeleteRemote(_origin); err != nil {
				return err
			}
		} else {
			return nil
		}
	} else if !errors.Is(err, git.ErrRemoteNotFound) {
		return err
	}
	if _, err := repo.CreateRemote(&gitcfg.RemoteConfig{
		Name:  _origin,
		URLs:  []string{upstream},
		Fetch: []gitcfg.RefSpec{gitcfg.RefSpec(_refspec)},
	}); err != nil {
		return err
	}
	return nil
}

// gitCommit stages the whole worktree and commits any pending changes. It
// returns committed=false (a no-op) when the worktree is clean. The on-disk
// security-approval ledger (approval.db and its SQLite WAL sidecars
// approval.db-wal / approval.db-shm) is never staged: even if a stale or
// hand-edited .gitignore failed to ignore it, the staging loop below skips
// every approval-ledger path individually so a ledger update can never enter a
// commit. When the only pending changes are approval-ledger files the tick is
// treated as a clean no-op.
// gitCommit stages the whole worktree and commits any pending changes. It
// returns committed=false (a no-op) when the worktree is clean. The on-disk
// security-approval ledger (approval.db and its SQLite WAL sidecars
// approval.db-wal / approval.db-shm) is never committed: it is gitignored (see
// _ignore / reconcileGitignoreApprovalLedger, reconciled at the start of every
// gitCheckIn) so wtree.Add(".") never stages it, and the status partition
// below treats a changeset that touches only approval-ledger files as a clean
// no-op so no commit (and no Ollama round-trip) is spent on a ledger rotation.
func gitCommit(config *OPNCall, repo *git.Repository) (bool, error) {
	wtree, err := repo.Worktree()
	if err != nil {
		return false, err
	}
	status, err := wtree.Status()
	if err != nil {
		return false, err
	}
	// Partition the status into real changes and approval-ledger files.
	// Approval-ledger files are gitignored and never staged; if they are the
	// only changes the tick is a clean no-op so no commit (and no Ollama
	// round-trip) is spent on a ledger rotation.
	hasReal := false
	for p := range status {
		if !isApprovalDBPath(p) {
			hasReal = true
			break
		}
	}
	if !hasReal {
		return false, nil
	}
	if _, err := wtree.Add(_currentDir); err != nil {
		return false, err
	}
	// Choose the commit message. The default is the static _commitMsg string.
	// When Ollama-assisted generation is enabled (OLLAMA_DESC_URL +
	// OLLAMA_DESC_MODEL both set) the diff between HEAD and the worktree is
	// routed to the model so it can author a short headline plus an extensive
	// explanation in the context of an OPNsense XML firewall configuration.
	// Commits whose changes are all Unifi .unf files keep the default message
	// without consulting the model. Any model error falls back to the default
	// so the backup is never left uncommitted.
	commitMsg := _commitMsg
	authorName := _authorName
	if msg := generateCommitMessage(config, repo, wtree); msg != "" {
		commitMsg = msg
		// When Ollama authored the message, replace the static
		// OPNBORG-AUTO-COMMIT author handle with a short, sanitised
		// version of the TLDR summary headline so the commit log surfaces
		// the change at a glance. Falls back to _authorName when the
		// message carries no TLDR line (default message, .unf bypass, or
		// a model outage that degraded to the detailed analysis alone).
		if name := authorFromCommitMessage(msg); name != "" {
			authorName = name
		}
	}
	commit, err := wtree.Commit(commitMsg, &git.CommitOptions{
		Author: &object.Signature{
			Name:  authorName,
			Email: config.Email,
			When:  time.Now(),
		},
		All:               true,
		AllowEmptyCommits: false,
	})
	if err != nil {
		return false, err
	}
	obj, err := repo.CommitObject(commit)
	if err != nil {
		return false, err
	}
	displayChan <- []byte("[GIT][REPO][COMMIT][" + obj.Hash.String() + "] " + obj.Message)
	// Record the commit in the security-approval ledger when its Ollama
	// security-impact tag is above low/none/backup (medium / high /
	// critical). A ledger problem is a non-fatal no-op so a backup is never
	// left uncommitted.
	approvalTrackCommit(config, obj.Hash.String(), obj.Message, obj.Author.When)
	return true, nil
}

// gitPush pushes the current branch to the configured upstream SSH repository,
// authenticating with the private key at config.Git.SSHKey. It is a no-op when
// no upstream is configured. Before pushing it re-syncs the origin remote URL
// and refspecs with the current configuration so a drifted upstream URL or a
// remote created without refspecs never silently keeps commits local. The
// outcome (timestamp, success flag, message) is recorded for the WebUI
// dashboard upstream-sync panel.
func gitPush(config *OPNCall, repo *git.Repository) error {
	upstream := config.Git.Upstream
	if upstream == "" {
		return nil
	}
	if config.Git.SSHKey == "" {
		recordPush(false, "missing OPN_GIT_SSH_KEY")
		return errors.New("git push: upstream configured but OPN_GIT_SSH_KEY is empty")
	}
	auth, err := gitSSHAuth(upstream, config.Git.SSHKey, config.Git.SSHHostKey)
	if err != nil {
		recordPush(false, "ssh key: "+err.Error())
		return err
	}
	if err := gitEnsureOrigin(repo, upstream); err != nil {
		recordPush(false, "origin: "+err.Error())
		return err
	}
	// Resolve the current branch (HEAD) so the push carries an explicit refspec
	// mapping the local branch onto its upstream twin, regardless of any
	// configured default fetch refspec. A detached HEAD falls back to pushing
	// HEAD to "refs/heads/master" so an auto-init'd repo with no commits yet on
	// a named branch still syncs.
	//
	// A freshly init'd repo with no commits yet has no HEAD. In that state
	// there is nothing to push, so the sync is treated as a no-op success
	// rather than an error: the next tick that produces a commit will have a
	// HEAD and push it upstream.
	head, err := repo.Head()
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			displayChan <- []byte("[GIT][REPO][PUSH][NOHEAD] " + upstream)
			recordPush(true, "no HEAD yet, nothing to push")
			return nil
		}
		recordPush(false, "head: "+err.Error())
		return err
	}
	// remoteBranch is the upstream branch name the push targets, and also the
	// name of the local remote-tracking ref (refs/remotes/origin/<branch>) we
	// keep in sync after a push so the WebUI dashboard can derive the upstream
	// state without a network round-trip. go-git's Push does not update the
	// local tracking ref itself (unlike canonical git), so we do it here.
	var refspec gitcfg.RefSpec
	var remoteBranch string
	if head.Name().IsBranch() {
		remoteBranch = head.Name().Short()
		refspec = gitcfg.RefSpec("+refs/heads/" + remoteBranch + ":refs/heads/" + remoteBranch)
	} else {
		remoteBranch = "master"
		refspec = gitcfg.RefSpec("+HEAD:refs/heads/master")
	}
	pushedHash := head.Hash()
	if err := repo.Push(&git.PushOptions{
		RemoteName: _origin,
		Auth:       auth,
		RefSpecs:   []gitcfg.RefSpec{refspec},
	}); err != nil {
		if errors.Is(err, git.NoErrAlreadyUpToDate) {
			displayChan <- []byte("[GIT][REPO][PUSH][UPTODATE] " + upstream)
			recordPushTrackingRef(repo, remoteBranch, pushedHash)
			recordPush(true, "already up-to-date")
			return nil
		}
		displayChan <- []byte("[GIT][REPO][PUSH][FAIL] " + err.Error())
		recordPush(false, err.Error())
		return err
	}
	recordPushTrackingRef(repo, remoteBranch, pushedHash)
	displayChan <- []byte("[GIT][REPO][PUSH][FINISH] " + upstream)
	recordPush(true, "pushed to "+upstream)
	return nil
}

// recordPushTrackingRef writes (or refreshes) the local remote-tracking ref
// refs/remotes/origin/<branch> so it points at the commit hash just pushed
// upstream. go-git's Push does not maintain this ref, so without it the
// dashboard upstream-sync panel would always report "never pushed" even after a
// successful push. A stale tracking ref is force-updated so it reflects the
// last known upstream tip.
func recordPushTrackingRef(repo *git.Repository, branch string, hash plumbing.Hash) {
	if branch == "" || hash.IsZero() {
		return
	}
	refName := plumbing.NewRemoteReferenceName(_origin, branch)
	if err := repo.Storer.SetReference(plumbing.NewHashReference(refName, hash)); err != nil {
		displayChan <- []byte("[GIT][REPO][PUSH][TRACK] " + err.Error())
	}
}

// recordPush stores the outcome of an upstream push attempt under gitLastPushMu
// so the WebUI dashboard can render upstream sync health without re-running the
// push.
func recordPush(ok bool, msg string) {
	gitLastPushMu.Lock()
	gitLastPushTS = time.Now()
	gitLastPushOK = ok
	gitLastPushMsg = msg
	gitLastPushMu.Unlock()
}

// gitInit ensures the storage folder is a git repository and the .gitignore is
// in place. It is safe to call repeatedly (open-or-init semantics). It also
// raises the repo's pack.deltaWindow to the aggressive value so the per-tick
// repack (gitGC) behaves like `git gc --aggressive` rather than the default
// 10-object window.
func gitInit(config *OPNCall) error {
	if err := os.Chdir(config.Path); err != nil {
		return err
	}
	if err := gitEnsureIgnore(config); err != nil {
		return err
	}
	repo, err := gitRepo(config.Path)
	if err != nil {
		return err
	}
	// Open the security-approval ledger so it is ready before the first
	// worker pass. A failure is logged but non-fatal: the daemon keeps
	// running and the ledger is lazily re-opened on first use. When the
	// ledger file does not yet exist on disk it is created fresh from the
	// schema; in that case the full storage-repo git history is scanned
	// once so every pre-existing security-relevant commit (medium / high /
	// critical tag) is backfilled into the ledger for operator triage.
	// Re-tracking a commit already present is a safe no-op, so the scan is
	// idempotent and never clobbers a recorded approval state.
	fresh := !approvalDBExists(config.Path)
	if _, err := approvalDBOpen(config.Path); err != nil {
		displayChan <- []byte("[APPROVAL][DB][OPEN][FAIL] " + err.Error())
	} else if fresh {
		displayChan <- []byte("[APPROVAL][DB][INIT] first-time ledger, scanning repo history")
		approvalBackfillFromHistory(config)
	}
	return gitEnsureAggressiveWindow(repo)
}

// validateGitConfig enforces the cross-field rules of the git backup feature:
// an upstream URL requires a readable SSH private key file, and a key file is
// not meaningful without an upstream to push to. The feature being disabled
// clears any stale upstream/key values so callers downstream can rely on
// Enable as the single gate.
func validateGitConfig(config *OPNCall) error {
	if !config.Git.Enable {
		config.Git.Upstream = ""
		config.Git.SSHKey = ""
		return nil
	}
	if config.Git.Upstream == "" && config.Git.SSHKey == "" {
		return nil
	}
	if config.Git.Upstream == "" {
		return errors.New("OPN_GIT_SSH_KEY requires OPN_GIT_UPSTREAM (the upstream SSH git URL to sync with)")
	}
	if config.Git.SSHKey == "" {
		return errors.New("OPN_GIT_UPSTREAM requires OPN_GIT_SSH_KEY (path to the SSH private key used for upstream auth)")
	}
	info, err := os.Stat(config.Git.SSHKey)
	if err != nil {
		return fmt.Errorf("OPN_GIT_SSH_KEY: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("OPN_GIT_SSH_KEY: %s is a directory, expected a private key file", config.Git.SSHKey)
	}
	return nil
}

// gitEnsureAggressiveWindow raises the repository's pack.deltaWindow to the
// aggressive value when it is still at or below the go-git default (10). This
// makes the per-tick repack performed by gitGC use the same wider delta window
// as `git gc --aggressive` (pack.deltaWindow=250). It is a no-op once the
// window is already at or above the target, and it never lowers a user-set
// higher value. Called once at gitInit.
func gitEnsureAggressiveWindow(repo *git.Repository) error {
	cfg, err := repo.ConfigScoped(gitcfg.LocalScope)
	if err != nil {
		return err
	}
	if cfg.Pack.Window >= _aggressivePackWindow {
		return nil
	}
	cfg.Pack.Window = _aggressivePackWindow
	return repo.SetConfig(cfg)
}

// gitGC performs the internal go-git equivalent of `git gc --aggressive`: it
// repacks all reachable objects into a fresh packfile (consolidating loose
// objects and old packs via OFS deltas, using the aggressive delta window set
// by gitEnsureAggressiveWindow) and then prunes unreachable loose objects
// older than the grace window. It runs after every successful commit+push so
// the on-disk backup store stays compact over time without invoking an
// external git binary. Both steps are no-ops on storages that do not support
// packing/loose objects (e.g. the in-memory test backend).
func gitGC(repo *git.Repository) error {
	if err := repo.RepackObjects(&git.RepackConfig{
		UseRefDeltas: false,
	}); err != nil {
		if errors.Is(err, git.ErrPackedObjectsNotSupported) {
			return nil
		}
		return err
	}
	if err := repo.Prune(git.PruneOptions{
		OnlyObjectsOlderThan: time.Now().Add(-_pruneGrace),
		Handler:              repo.DeleteObject,
	}); err != nil {
		if errors.Is(err, git.ErrLooseObjectsNotSupported) {
			return nil
		}
		return err
	}
	return nil
}

// commit any pending backup changes, push to the upstream remote when one is
// configured, and finally run the internal equivalent of `git gc --aggressive`
// so the on-disk backup store stays compact. The gc only runs when a new
// commit was created this tick (a clean worktree needs no repack).
//
// When an upstream is configured the push runs on every tick, not only after
// a fresh commit: the local repo may hold commits a previous tick failed to
// push (transient upstream outage), or the upstream may have drifted from the
// local HEAD. The configured refspec is a forced update (+refs/heads/*), so a
// divergent upstream is resynced to the local history rather than leaving the
// remote out of step. A clean tick with nothing to push is a no-op success.
//
// It returns committed=true when a new commit was created this tick.
func gitCheckIn(config *OPNCall) (bool, error) {
	if err := os.Chdir(config.Path); err != nil {
		return false, err
	}
	if err := gitEnsureIgnore(config); err != nil {
		return false, err
	}
	repo, err := gitRepo(config.Path)
	if err != nil {
		return false, err
	}
	committed, err := gitCommit(config, repo)
	if err != nil {
		displayChan <- []byte("[GIT][REPO][COMMIT][FAIL] " + err.Error())
		return false, err
	}
	// Always attempt the upstream sync when an upstream is configured, even on
	// a clean tick, so the remote is reconciled with the local HEAD every pass.
	if config.Git.Upstream != "" {
		if err := gitPush(config, repo); err != nil {
			return committed, err
		}
	}
	if !committed {
		return false, nil
	}
	if err := gitGC(repo); err != nil {
		displayChan <- []byte("[GIT][REPO][GC][FAIL] " + err.Error())
		return true, err
	}
	displayChan <- []byte("[GIT][REPO][GC][FINISH]")
	return true, nil
}
