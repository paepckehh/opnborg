package opnborg

import (
	"os"
	"time"

	"github.com/alecthomas/chroma/v2/quick"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

const (
	_currentDir = "."
	_gitignore  = ".gitignore"
	_ignore     = ".archive\nCONFIG*\nLogs\n"
)

// gitCheckIn commits all config files into a local repository
func gitCheckIn(config *OPNCall) error {

	// change into Storage Path
	err := os.Chdir(config.Path)
	if err != nil {
		return err
	}

	// verify & create Gitignore file
	if _, err := os.Stat(_gitignore); err != nil {
		if err := os.WriteFile(_gitignore, []byte(_ignore), 0660); err != nil {
			displayChan <- []byte("[BACKUP][ERROR][FAIL:UNABLE-TO-CREATE-GIT-IGNORE-FILE] " + config.Path)
			return err
		}
	}

	// open git repo
	repo, err := git.PlainOpen(_currentDir)
	if err != nil {
		// Init a new repository using the ObjectFormat SHA256, when open fails
		repo, err = git.PlainInit(_currentDir, false)
		if err != nil {
			return err
		}
	}

	// Activate Working Tree
	wtree, err := repo.Worktree()
	if err != nil {
		return err
	}

	// Add Working Tree State
	_, err = wtree.Add(".")
	if err != nil {
		return err
	}

	// Commit Current State
	commit, err := wtree.Commit("opnborg auto update", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "OPNBORG-AUTO-COMMIT",
			Email: config.Email,
			When:  time.Now(),
		},
		AllowEmptyCommits: false,
	})
	if err != nil {
		return err
	}

	// repack if possible
	_ = repo.RepackObjects(&git.RepackConfig{
		UseRefDeltas:             true,
		OnlyDeletePacksOlderThan: time.Now(),
	})

	// Fetch & Verify HEAD to show last commit
	obj, err := repo.CommitObject(commit)
	if err != nil {
		return err
	}

	// add patch details
	err = quick.Highlight(os.Stdout, obj.String(), "diff", "TTY265", "github")
	if err != nil {
		return err
	}
	if config.GitPush {

		// Push to (Remote) Upstream Repo
		if err := repo.Push(&git.PushOptions{}); err != nil {
			displayChan <- []byte("[GIT][REPO][PUSH][FAIL]")
			return err
		}
		displayChan <- []byte("[CHANGES-DETECTED][GIT][REPO][PUSH][FINISH]")
	}
	return nil
}
