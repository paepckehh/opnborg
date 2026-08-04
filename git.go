package opnborg

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
)

const (
	_currentDir     = "."
	_gitignore      = ".gitignore"
	_ignore         = ".archive\nCONFIG*\nLogs\n"
	_origin         = "origin"
	_commitMsg      = "opnborg auto update"
	_authorName     = "OPNBORG-AUTO-COMMIT"
	_defaultSSHUser = "git"
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
// missing, so the archive/history files and symlink targets stay out of the
// commit history. Pre-existing files are left untouched.
func gitEnsureIgnore(config *OPNCall) error {
	ignore := filepath.Join(config.Path, _gitignore)
	if _, err := os.Stat(ignore); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.WriteFile(ignore, []byte(_ignore), 0660); err != nil {
		displayChan <- []byte("[GIT][REPO][ERROR][FAIL:UNABLE-TO-CREATE-GIT-IGNORE-FILE] " + config.Path)
		return err
	}
	return nil
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

// gitSSHAuth builds an SSH public-key auth method from the private key file at
// keyPath, using the user derived from the upstream URL. Host key verification
// relies on go-git's default known-hosts callback (~/.ssh/known_hosts).
func gitSSHAuth(upstream, keyPath string) (*ssh.PublicKeys, error) {
	auth, err := ssh.NewPublicKeysFromFile(sshUserFromURL(upstream), keyPath, "")
	if err != nil {
		return nil, fmt.Errorf("ssh key %s: %w", keyPath, err)
	}
	return auth, nil
}

// gitEnsureOrigin makes sure an "origin" remote pointing at upstream exists in
// the repository, recreating it when the recorded URL drifted so the push
// targets the currently configured upstream.
func gitEnsureOrigin(repo *git.Repository, upstream string) error {
	rem, err := repo.Remote(_origin)
	if err == nil {
		cfg := rem.Config()
		if len(cfg.URLs) == 0 || cfg.URLs[0] != upstream {
			if err := repo.DeleteRemote(_origin); err != nil {
				return err
			}
		} else {
			return nil
		}
	} else if !errors.Is(err, git.ErrRemoteNotFound) {
		return err
	}
	if _, err := repo.CreateRemote(&config.RemoteConfig{
		Name: _origin,
		URLs: []string{upstream},
	}); err != nil {
		return err
	}
	return nil
}

// gitCommit stages the whole worktree and commits any pending changes. It
// returns committed=false (a no-op) when the worktree is clean.
func gitCommit(config *OPNCall, repo *git.Repository) (bool, error) {
	wtree, err := repo.Worktree()
	if err != nil {
		return false, err
	}
	status, err := wtree.Status()
	if err != nil {
		return false, err
	}
	if status.IsClean() {
		return false, nil
	}
	if _, err := wtree.Add(_currentDir); err != nil {
		return false, err
	}
	commit, err := wtree.Commit(_commitMsg, &git.CommitOptions{
		Author: &object.Signature{
			Name:  _authorName,
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
	return true, nil
}

// gitPush pushes the current branch to the configured upstream SSH repository,
// authenticating with the private key at config.Git.SSHKey. It is a no-op when
// no upstream is configured. The outcome (timestamp, success flag, message) is
// recorded for the WebUI dashboard upstream-sync panel.
func gitPush(config *OPNCall, repo *git.Repository) error {
	upstream := config.Git.Upstream
	if upstream == "" {
		return nil
	}
	auth, err := gitSSHAuth(upstream, config.Git.SSHKey)
	if err != nil {
		recordPush(false, "ssh key: "+err.Error())
		return err
	}
	if err := gitEnsureOrigin(repo, upstream); err != nil {
		recordPush(false, "origin: "+err.Error())
		return err
	}
	if err := repo.Push(&git.PushOptions{
		RemoteName: _origin,
		Auth:       auth,
	}); err != nil {
		displayChan <- []byte("[GIT][REPO][PUSH][FAIL] " + err.Error())
		recordPush(false, err.Error())
		return err
	}
	displayChan <- []byte("[GIT][REPO][PUSH][FINISH] " + upstream)
	recordPush(true, "pushed to "+upstream)
	return nil
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
// in place. It is safe to call repeatedly (open-or-init semantics).
func gitInit(config *OPNCall) error {
	if err := os.Chdir(config.Path); err != nil {
		return err
	}
	if err := gitEnsureIgnore(config); err != nil {
		return err
	}
	if _, err := gitRepo(config.Path); err != nil {
		return err
	}
	return nil
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

// gitCheckIn is the per-tick entry point: open the storage git repository,
// commit any pending backup changes, and push to the upstream remote when one
// is configured. It returns committed=true when a new commit was created.
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
	if !committed {
		return false, nil
	}
	if err := gitPush(config, repo); err != nil {
		return true, err
	}
	return true, nil
}
