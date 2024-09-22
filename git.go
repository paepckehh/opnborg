package opnborg

import (
	"os"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

const _currentDir = "."

// gitCheckIn commits all config files into a local repository
func gitCheckIn(config *OPNCall) error {

	// change into Storage Path
	err := os.Chdir(config.Path)
	if err != nil {
		return err
	}

	// Open git repo
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
	_, err = wtree.Commit("opnborg auto update", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "OPNBORG-AUTO-COMMIT",
			Email: config.Email,
			When:  time.Now(),
		},
	})
	if err != nil {
		return err
	}
	return nil
}
