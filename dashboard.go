package opnborg

import (
	"errors"
	"fmt"
	"html"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// dashboardStats describes the state of the on-disk backup store, the local
// git repository, and the upstream sync health. It is gathered on every WebUI
// index render (the page auto-refreshes every 15s) and rendered into the
// dashboard section at the bottom of the page.
type dashboardStats struct {
	// backup folder
	storePath     string
	servers       int
	archives      int
	unifiArchives int // archive entries with a .unf extension (Unifi controller backups)
	archiveBytes  int64
	newestArchive time.Time
	// git local repo
	gitEnabled    bool
	gitRepo       bool
	gitHead       string // short hash, "" when no commits
	gitCommits    int
	gitLastCommit time.Time
	gitLastMsg    string
	gitDirty      int // number of uncommitted worktree entries
	gitError      string
	// upstream sync
	upstreamConfigured bool
	upstreamURL        string
	upstreamRemote     string // remote-tracking ref short name, e.g. origin/main
	upstreamInSync     bool
	upstreamAhead      int
	upstreamBehind     int
	upstreamNever      bool // no remote-tracking ref recorded yet
	upstreamLastTS     time.Time
	upstreamLastOK     bool
	upstreamLastMsg    string
}

// gatherDashboard walks the backup store and inspects the local git repository
// to populate a dashboardStats snapshot. It never performs network I/O for the
// upstream panel (it compares the local HEAD against the last recorded
// remote-tracking ref), so a page render cannot block on a remote. All paths
// are resolved against config.Path and the function does NOT call os.Chdir,
// so it is safe to invoke from the httpd render goroutine concurrently with the
// per-server backup workers.
func gatherDashboard(config *OPNCall) dashboardStats {
	d := dashboardStats{
		storePath:          config.Path,
		gitEnabled:         config.Git.Enable,
		upstreamConfigured: config.Git.Upstream != "",
		upstreamURL:        config.Git.Upstream,
	}
	gatherBackupFolder(config, &d)
	if config.Git.Enable {
		gatherGitRepo(config, &d)
		gatherUpstream(config, &d)
	}
	// last push outcome (guarded by gitLastPushMu)
	gitLastPushMu.Lock()
	d.upstreamLastTS = gitLastPushTS
	d.upstreamLastOK = gitLastPushOK
	d.upstreamLastMsg = gitLastPushMsg
	gitLastPushMu.Unlock()
	return d
}

// gatherBackupFolder walks config.Path counting per-server archive trees,
// archive entries, total bytes, and the newest archive mtime. The .git
// metadata directory is excluded so its objects do not skew the backup stats.
func gatherBackupFolder(config *OPNCall, d *dashboardStats) {
	root := config.Path
	if root == "" {
		root = "."
	}
	// immediate children that look like a server backup slot either hold a
	// .archive subdir or a current.* regular file.
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() || e.Name() == ".git" || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		serverDir := filepath.Join(root, e.Name())
		isServer := false
		if _, err := os.Stat(filepath.Join(serverDir, _archive)); err == nil {
			isServer = true
		}
		if !isServer {
			matches, _ := filepath.Glob(filepath.Join(serverDir, "current.*"))
			if len(matches) > 0 {
				isServer = true
			}
		}
		if isServer {
			d.servers++
		}
	}
	// walk the whole tree (excluding .git) to tally archive entries
	_ = filepath.WalkDir(root, func(path string, info fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if info.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		// an archive entry lives under a <server>/.archive/... subtree
		if strings.Contains(filepath.ToSlash(path), "/"+_archive+"/") {
			d.archives++
			if strings.HasSuffix(info.Name(), ".unf") {
				d.unifiArchives++
			}
			if fi, err := info.Info(); err == nil {
				d.archiveBytes += fi.Size()
				if fi.ModTime().After(d.newestArchive) {
					d.newestArchive = fi.ModTime()
				}
			}
		}
		return nil
	})
}

// gatherGitRepo opens the local git repository (without initialising) and
// collects the HEAD hash, commit count, last commit metadata, and the dirty
// worktree entry count. Any open error is recorded in d.gitError so the panel
// can render "not a git repo" / "git error" instead of panicking.
func gatherGitRepo(config *OPNCall, d *dashboardStats) {
	repo, err := git.PlainOpen(config.Path)
	if err != nil {
		if errors.Is(err, git.ErrRepositoryNotExists) {
			d.gitError = "repository not initialised"
		} else {
			d.gitError = err.Error()
		}
		return
	}
	d.gitRepo = true
	head, err := repo.Head()
	if err != nil {
		// no commits yet (unborn HEAD) — still a valid repo, just empty
		return
	}
	d.gitHead = head.Hash().String()
	if len(d.gitHead) > 7 {
		d.gitHead = d.gitHead[:7]
	}
	commit, err := repo.CommitObject(head.Hash())
	if err == nil {
		d.gitLastCommit = commit.Author.When
		d.gitLastMsg = strings.TrimSpace(commit.Message)
		if len(d.gitLastMsg) > 80 {
			d.gitLastMsg = d.gitLastMsg[:77] + "..."
		}
	}
	// count commits up to a sane cap so a huge history does not stall the page
	const commitCap = 10000
	iter, err := repo.Log(&git.LogOptions{From: head.Hash()})
	if err == nil {
		defer iter.Close()
		for {
			if _, err := iter.Next(); err != nil {
				break
			}
			d.gitCommits++
			if d.gitCommits >= commitCap {
				break
			}
		}
	}
	// dirty worktree entry count
	if wtree, err := repo.Worktree(); err == nil {
		if status, err := wtree.Status(); err == nil {
			for file, s := range status {
				if s.Staging == git.Unmodified && s.Worktree == git.Unmodified {
					continue
				}
				d.gitDirty++
				_ = file
			}
		}
	}
}

// gatherUpstream compares the local HEAD against the remote-tracking ref
// recorded in the local repository (refs/remotes/origin/<branch>) to derive a
// sync status without any network round-trip. When no tracking ref exists the
// upstream has never been fetched/pushed, so the panel reports "never synced".
func gatherUpstream(config *OPNCall, d *dashboardStats) {
	if config.Git.Upstream == "" {
		return
	}
	repo, err := git.PlainOpen(config.Path)
	if err != nil {
		return
	}
	head, err := repo.Head()
	if err != nil {
		return
	}
	branch := head.Name().Short()
	d.upstreamRemote = _origin + "/" + branch
	trackRef := plumbing.NewRemoteReferenceName(_origin, branch)
	remoteRef, err := repo.Reference(trackRef, true)
	if err != nil {
		// fall back to refs/remotes/origin/HEAD symbolic ref
		if sym, err2 := repo.Reference(plumbing.ReferenceName("refs/remotes/origin/HEAD"), true); err2 == nil {
			remoteRef = sym
		} else {
			d.upstreamNever = true
			return
		}
	}
	localHash := head.Hash()
	remoteHash := remoteRef.Hash()
	if localHash == remoteHash {
		d.upstreamInSync = true
		return
	}
	// ahead: commits in local not in remote. behind: commits in remote not in
	// local. compute via the object graph (no network).
	d.upstreamAhead, d.upstreamBehind = countDivergence(repo, localHash, remoteHash)
}

// countDivergence returns (ahead, behind) commit counts between two commit
// hashes using the local object graph. ahead counts commits reachable from
// local but not remote; behind counts commits reachable from remote but not
// local. Either side missing returns zeros.
func countDivergence(repo *git.Repository, local, remote plumbing.Hash) (int, int) {
	localSet := reachableHashes(repo, local)
	remoteSet := reachableHashes(repo, remote)
	var ahead, behind int
	for h := range localSet {
		if _, ok := remoteSet[h]; !ok {
			ahead++
		}
	}
	for h := range remoteSet {
		if _, ok := localSet[h]; !ok {
			behind++
		}
	}
	return ahead, behind
}

// reachableHashes returns the set of commit hashes reachable from the given
// start commit (inclusive). A missing or unreadable start yields an empty set.
// The walk is bounded by a generous cap so a pathological history cannot stall
// a page render.
func reachableHashes(repo *git.Repository, start plumbing.Hash) map[plumbing.Hash]struct{} {
	out := make(map[plumbing.Hash]struct{})
	const reachableCap = 50000
	var walk func(h plumbing.Hash)
	walk = func(h plumbing.Hash) {
		if _, ok := out[h]; ok {
			return
		}
		if len(out) >= reachableCap {
			return
		}
		commit, err := repo.CommitObject(h)
		if err != nil {
			return
		}
		out[h] = struct{}{}
		_ = commit.Parents().ForEach(func(p *object.Commit) error {
			walk(p.Hash)
			return nil
		})
	}
	walk(start)
	return out
}

// humanBytes formats a byte count as a compact human-readable string (e.g.
// "1.2 MiB"). It uses binary units to match the storage semantics of the
// backup archive.
func humanBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

// getDashboard renders the bottom-of-page dashboard: three panels for the
// backup folder stats, the local git repo state, and the upstream sync health.
// It is appended to the index page in getStartHTML. The dashboard is always
// rendered (even when git is disabled) so an operator always has a view onto
// the on-disk store; disabled panels collapse to a one-line status. A nil
// config handle (e.g. the httpd not yet armed in tests) collapses the whole
// section to a one-line placeholder.
func getDashboard(config *OPNCall) string {
	if config == nil {
		return "<div class=\"dashboard\"><h2>BorgDASHBOARD</h2><div class=\"dash-row\"><span class=\"dash-value dash-muted\">awaiting config</span></div></div>"
	}
	d := gatherDashboard(config)
	var s strings.Builder
	s.WriteString("<div class=\"dashboard\"><h2>BorgDASHBOARD</h2>")
	s.WriteString("<div class=\"dashboard-grid\">")

	// panel 1: backup folder
	s.WriteString("<div class=\"dash-panel\"><div class=\"dash-title\">Backup Store</div>")
	s.WriteString("<div class=\"dash-row\"><span class=\"dash-label\">Path</span><span class=\"dash-value\">" + html.EscapeString(d.storePath) + "</span></div>")
	s.WriteString("<div class=\"dash-row\"><span class=\"dash-label\">Servers</span><span class=\"dash-value\">" + strconv.Itoa(d.servers) + "</span></div>")
	s.WriteString("<div class=\"dash-row\"><span class=\"dash-label\">Archives</span><span class=\"dash-value\">" + strconv.Itoa(d.archives) + "</span></div>")
	s.WriteString("<div class=\"dash-row\"><span class=\"dash-label\">Unifi Backups</span><span class=\"dash-value\">" + strconv.Itoa(d.unifiArchives) + "</span></div>")
	s.WriteString("<div class=\"dash-row\"><span class=\"dash-label\">Size</span><span class=\"dash-value\">" + humanBytes(d.archiveBytes) + "</span></div>")
	newest := "n/a"
	if !d.newestArchive.IsZero() {
		newest = d.newestArchive.UTC().Format(time.RFC3339)
	}
	s.WriteString("<div class=\"dash-row\"><span class=\"dash-label\">Newest</span><span class=\"dash-value\">" + newest + "</span></div>")
	s.WriteString("</div>")

	// panel 2: local git repo
	s.WriteString("<div class=\"dash-panel\"><div class=\"dash-title\">Local Git Repo</div>")
	if !d.gitEnabled {
		s.WriteString("<div class=\"dash-row\"><span class=\"dash-value dash-muted\">git management disabled (OPN_GIT_ENABLE unset)</span></div>")
	} else if d.gitError != "" {
		s.WriteString("<div class=\"dash-row\"><span class=\"dash-value dash-err\">" + html.EscapeString(d.gitError) + "</span></div>")
	} else {
		head := d.gitHead
		if head == "" {
			head = "n/a (no commits)"
		}
		lastC := "n/a"
		if !d.gitLastCommit.IsZero() {
			lastC = d.gitLastCommit.UTC().Format(time.RFC3339)
		}
		dirtyState := "<span class=\"dash-ok\">clean</span>"
		if d.gitDirty > 0 {
			dirtyState = "<span class=\"dash-warn\">" + strconv.Itoa(d.gitDirty) + " pending</span>"
		}
		msg := d.gitLastMsg
		if msg == "" {
			msg = "n/a"
		}
		s.WriteString("<div class=\"dash-row\"><span class=\"dash-label\">HEAD</span><span class=\"dash-value\">" + head + "</span></div>")
		s.WriteString("<div class=\"dash-row\"><span class=\"dash-label\">Commits</span><span class=\"dash-value\">" + strconv.Itoa(d.gitCommits) + "</span></div>")
		s.WriteString("<div class=\"dash-row\"><span class=\"dash-label\">Last Commit</span><span class=\"dash-value\">" + lastC + "</span></div>")
		s.WriteString("<div class=\"dash-row\"><span class=\"dash-label\">Worktree</span><span class=\"dash-value\">" + dirtyState + "</span></div>")
		s.WriteString("<div class=\"dash-row\"><span class=\"dash-label\">Message</span><span class=\"dash-value\">" + html.EscapeString(msg) + "</span></div>")
	}
	s.WriteString("</div>")

	// panel 3: upstream sync health
	s.WriteString("<div class=\"dash-panel\"><div class=\"dash-title\">Upstream Sync</div>")
	if !d.upstreamConfigured {
		s.WriteString("<div class=\"dash-row\"><span class=\"dash-value dash-muted\">no upstream configured (OPN_GIT_UPSTREAM unset)</span></div>")
	} else {
		s.WriteString("<div class=\"dash-row\"><span class=\"dash-label\">URL</span><span class=\"dash-value\">" + html.EscapeString(d.upstreamURL) + "</span></div>")
		s.WriteString("<div class=\"dash-row\"><span class=\"dash-label\">Tracking</span><span class=\"dash-value\">" + html.EscapeString(d.upstreamRemote) + "</span></div>")
		status := ""
		switch {
		case d.upstreamNever:
			status = "<span class=\"dash-warn\">never pushed (no remote-tracking ref)</span>"
		case d.upstreamInSync:
			status = "<span class=\"dash-ok\">in sync</span>"
		case d.upstreamAhead > 0 && d.upstreamBehind == 0:
			status = "<span class=\"dash-warn\">" + strconv.Itoa(d.upstreamAhead) + " ahead (push pending)</span>"
		case d.upstreamAhead == 0 && d.upstreamBehind > 0:
			status = "<span class=\"dash-err\">" + strconv.Itoa(d.upstreamBehind) + " behind upstream</span>"
		default:
			status = "<span class=\"dash-err\">diverged: " + strconv.Itoa(d.upstreamAhead) + " ahead / " + strconv.Itoa(d.upstreamBehind) + " behind</span>"
		}
		s.WriteString("<div class=\"dash-row\"><span class=\"dash-label\">State</span><span class=\"dash-value\">" + status + "</span></div>")
		last := "never"
		if !d.upstreamLastTS.IsZero() {
			last = d.upstreamLastTS.UTC().Format(time.RFC3339)
		}
		pushState := "<span class=\"dash-muted\">no push attempted yet</span>"
		if !d.upstreamLastTS.IsZero() {
			if d.upstreamLastOK {
				pushState = "<span class=\"dash-ok\">last push OK</span>"
			} else {
				pushState = "<span class=\"dash-err\">last push failed</span>"
			}
		}
		s.WriteString("<div class=\"dash-row\"><span class=\"dash-label\">Last Push</span><span class=\"dash-value\">" + last + "</span></div>")
		s.WriteString("<div class=\"dash-row\"><span class=\"dash-label\">Push Result</span><span class=\"dash-value\">" + pushState + "</span></div>")
		if d.upstreamLastMsg != "" {
			s.WriteString("<div class=\"dash-row\"><span class=\"dash-label\">Detail</span><span class=\"dash-value\">" + html.EscapeString(d.upstreamLastMsg) + "</span></div>")
		}
	}
	s.WriteString("</div>")

	s.WriteString(_configButton)

	s.WriteString("</div></div>")
	return s.String()
}
