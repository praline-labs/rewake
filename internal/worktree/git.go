package worktree

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// inspect reads what a checkout needs from the repository holding from: its
// Git directory, the top of the checkout from stands in, where in it from is,
// and the commit at HEAD.
func inspect(from string) (Record, error) {
	top, err := gitOutput(from, "rev-parse", "--show-toplevel")
	if err != nil {
		return Record{}, &UnusableError{Reason: fmt.Sprintf("%s is not in a Git working tree, so there is nothing to check out (%v)", from, err)}
	}
	common, err := gitOutput(from, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return Record{}, fmt.Errorf("cannot find the Git directory of %s: %w", top, err)
	}
	commit, err := gitOutput(from, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	if err != nil || commit == "" {
		return Record{}, &UnusableError{Reason: fmt.Sprintf("the repository at %s has no commit to check out yet", top)}
	}
	prefix, err := gitOutput(from, "rev-parse", "--show-prefix")
	if err != nil {
		return Record{}, err
	}
	common, err = filepath.EvalSymlinks(common)
	if err != nil {
		return Record{}, err
	}
	subdir := ""
	if prefix != "" {
		subdir = filepath.Clean(prefix)
	}
	return Record{
		Repository: repositoryDir(common),
		CommonDir:  common,
		Source:     top,
		Commit:     commit,
		Subdir:     subdir,
	}, nil
}

// Check is what a command should know of a checkout before removing it.
type Check struct {
	// Missing is a checkout whose directory is gone.
	Missing bool `json:"missing,omitempty"`
	// Changes lists what `git status` shows: edits, staged or untracked files.
	Changes bool `json:"changes,omitempty"`
	// Unreachable is a HEAD moved to commits no branch or tag holds: removing
	// the checkout would leave them to garbage collection.
	Unreachable bool `json:"unreachable,omitempty"`
	// Head is the commit checked out now.
	Head string `json:"head,omitempty"`
}

// Dirty says whether removing the checkout would lose work.
func (c Check) Dirty() bool { return c.Changes || c.Unreachable }

// Inspect looks at a checkout as it is now.
func Inspect(record Record) (Check, error) {
	if info, err := os.Stat(record.Path); err != nil || !info.IsDir() {
		return Check{Missing: true}, nil
	}
	status, err := gitOutput(record.Path, "status", "--porcelain", "--untracked-files=normal")
	if err != nil {
		return Check{}, fmt.Errorf("git status in %s failed: %w", record.Path, err)
	}
	head, err := gitOutput(record.Path, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	if err != nil {
		return Check{}, fmt.Errorf("cannot read HEAD in %s: %w", record.Path, err)
	}
	check := Check{Changes: status != "", Head: head}
	if head != record.Commit {
		holders, err := gitOutput(record.Path, "for-each-ref", "--contains", head, "--count=1", "--format=%(refname)", "refs/heads", "refs/tags")
		if err != nil {
			return Check{}, fmt.Errorf("cannot tell whether %s is on a branch: %w", head, err)
		}
		check.Unreachable = holders == ""
	}
	return check, nil
}

// Remove takes a checkout away with the public `git worktree remove`, and its
// record after it. force removes one with changes as well; a checkout whose
// directory is already gone is pruned from the repository.
func Remove(record Record, force bool) error {
	if info, err := os.Stat(record.Path); err != nil || !info.IsDir() {
		if err := gitDir(record.CommonDir, "worktree", "prune"); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("git worktree prune failed: %w", err)
		}
	} else {
		args := []string{"worktree", "remove"}
		if force {
			args = append(args, "--force")
		}
		if err := gitDir(record.CommonDir, append(args, record.Path)...); err != nil {
			return fmt.Errorf("git worktree remove failed: %w", err)
		}
	}
	if err := os.Remove(recordPath(record)); err != nil && !os.IsNotExist(err) {
		return err
	}
	// The repository's directory goes with its last checkout; one still in use
	// refuses to go, which is the answer wanted.
	_ = os.Remove(filepath.Dir(record.Path))
	return nil
}

// git runs git in a directory.
func git(dir string, args ...string) error {
	_, err := run(exec.Command("git", append([]string{"-C", dir}, args...)...))
	return err
}

// gitDir runs git against a repository's Git directory, which stays valid when
// the checkout a command came from is gone.
func gitDir(commonDir string, args ...string) error {
	if _, err := os.Stat(commonDir); err != nil {
		return err
	}
	_, err := run(exec.Command("git", append([]string{"--git-dir", commonDir}, args...)...))
	return err
}

func gitOutput(dir string, args ...string) (string, error) {
	return run(exec.Command("git", append([]string{"-C", dir}, args...)...))
}

// run runs git with an environment of its own choosing: a GIT_DIR or a
// GIT_WORK_TREE inherited from a hook that started this would point every call
// at another repository.
func run(command *exec.Cmd) (string, error) {
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		switch key {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR", "GIT_OBJECT_DIRECTORY", "GIT_PREFIX", "GIT_NAMESPACE":
			continue
		}
		command.Env = append(command.Env, entry)
	}
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		if message := strings.TrimSpace(stderr.String()); message != "" {
			return "", errors.New(message)
		}
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}
