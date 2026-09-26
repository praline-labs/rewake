package worktree

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
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
	// Missing is a checkout whose directory is gone: removed, or moved by
	// hand, in which case whatever it held is out of sight.
	Missing bool `json:"missing,omitempty"`
	// Forgotten is a missing checkout the repository no longer lists either:
	// nothing of it is left but rewake's record.
	Forgotten bool `json:"forgotten,omitempty"`
	// Changes lists what `git status` shows: edits, staged or untracked files.
	Changes bool `json:"changes,omitempty"`
	// Ignored is files git ignores, which `git worktree remove` deletes
	// without asking: a .env, a local build.
	Ignored bool `json:"ignored,omitempty"`
	// Unreachable is a HEAD no branch, tag or remote-tracking ref holds:
	// removing the checkout would leave its commits to garbage collection.
	// It holds for the commit a checkout was made at as well, once the branch
	// that held it is gone.
	Unreachable bool `json:"unreachable,omitempty"`
	// Head is the commit checked out now, as the checkout or, when it is
	// missing, the repository's list of worktrees says.
	Head string `json:"head,omitempty"`
}

// Dirty says whether removing the checkout would lose work.
func (c Check) Dirty() bool { return c.Changes || c.Ignored || c.Unreachable }

// Inspect looks at a checkout as it is now.
func Inspect(record Record) (Check, error) {
	if info, err := os.Stat(record.Path); err != nil || !info.IsDir() {
		return inspectMissing(record)
	}
	status, err := gitOutput(record.Path, "status", "--porcelain", "--untracked-files=normal", "--ignored=matching")
	if err != nil {
		return Check{}, fmt.Errorf("git status in %s failed: %w", record.Path, err)
	}
	head, err := gitOutput(record.Path, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	if err != nil {
		return Check{}, fmt.Errorf("cannot read HEAD in %s: %w", record.Path, err)
	}
	check := Check{Head: head}
	for _, line := range strings.Split(status, "\n") {
		switch {
		case strings.HasPrefix(line, "!! "):
			check.Ignored = true
		case line != "":
			check.Changes = true
		}
	}
	if check.Unreachable, err = unreachable(record.CommonDir, head); err != nil {
		return Check{}, err
	}
	return check, nil
}

// inspectMissing looks at a checkout whose directory is gone through what the
// repository still records of it.
func inspectMissing(record Record) (Check, error) {
	check := Check{Missing: true}
	head, listed, err := listedHead(record)
	if err != nil {
		return Check{}, err
	}
	if !listed {
		check.Forgotten = true
		return check, nil
	}
	check.Head = head
	if head != "" {
		if check.Unreachable, err = unreachable(record.CommonDir, head); err != nil {
			return Check{}, err
		}
	}
	return check, nil
}

// unreachable says whether no branch, tag or remote-tracking ref holds a
// commit. A commit git cannot find at all is an error, not an answer.
func unreachable(commonDir, head string) (bool, error) {
	holders, err := gitDirOutput(commonDir, "for-each-ref", "--contains", head, "--count=1", "--format=%(refname)", "refs/heads", "refs/tags", "refs/remotes")
	if err != nil {
		return false, fmt.Errorf("cannot tell whether %s is on a branch: %w", head, err)
	}
	return holders == "", nil
}

// listedHead reads a checkout's HEAD from the repository's own list of
// worktrees, and says whether the repository lists it at all.
func listedHead(record Record) (string, bool, error) {
	out, err := gitDirOutput(record.CommonDir, "worktree", "list", "--porcelain")
	if err != nil {
		return "", false, fmt.Errorf("cannot list the worktrees of %s: %w", record.CommonDir, err)
	}
	for _, block := range strings.Split(out, "\n\n") {
		path, head := "", ""
		for _, line := range strings.Split(block, "\n") {
			if value, ok := strings.CutPrefix(line, "worktree "); ok {
				path = value
			} else if value, ok := strings.CutPrefix(line, "HEAD "); ok {
				head = value
			}
		}
		if path != "" && samePath(path, record.Path) {
			return head, true, nil
		}
	}
	return "", false, nil
}

// samePath compares a path git printed with a record's, which may reach the
// same place through a symbolic link.
func samePath(listed, recorded string) bool {
	if filepath.Clean(listed) == filepath.Clean(recorded) {
		return true
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(recorded))
	return err == nil && filepath.Clean(listed) == filepath.Join(parent, filepath.Base(recorded))
}

// Remove takes a checkout away with the public `git worktree remove`, and its
// record after it. force removes one with changes as well. A checkout whose
// directory is gone is removed the same way, which on a missing directory
// touches only its own entry — never `git worktree prune`, which would take
// every other missing checkout of the repository along; one the repository
// has forgotten leaves only the record to remove.
func Remove(record Record, force bool) error {
	forgotten := false
	if info, err := os.Stat(record.Path); err != nil || !info.IsDir() {
		_, listed, err := listedHead(record)
		if err != nil {
			return err
		}
		forgotten = !listed
	}
	if !forgotten {
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

func gitDirOutput(commonDir string, args ...string) (string, error) {
	if _, err := os.Stat(commonDir); err != nil {
		return "", err
	}
	return run(exec.Command("git", append([]string{"--git-dir", commonDir}, args...)...))
}

// run runs git with an environment of its own choosing: a GIT_DIR or a
// GIT_WORK_TREE inherited from a hook that started this would point every call
// at another repository.
//
// Nothing it runs may wait for a person, since the caller is most often an
// agent: no terminal prompt for credentials a hook or a filter might ask for,
// and a session of its own, so git has no terminal to open at all. And a look
// must not stand in a session's way: `git status` otherwise refreshes the
// index under index.lock, and a commit the session makes at that moment fails.
func run(command *exec.Cmd) (string, error) {
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		switch key {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR", "GIT_OBJECT_DIRECTORY", "GIT_PREFIX", "GIT_NAMESPACE",
			"GIT_TERMINAL_PROMPT", "GIT_OPTIONAL_LOCKS":
			continue
		}
		command.Env = append(command.Env, entry)
	}
	command.Env = append(command.Env, "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0")
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
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
