package worktree

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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
	// Forgotten is a checkout git has nothing left to say of: missing, and
	// no longer listed by its repository, or a repository that is gone
	// itself. A directory still there holds what nobody can tell.
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
	// Branch is the branch checked out in it now, empty when it is detached
	// or missing.
	Branch string `json:"branch,omitempty"`
}

// Dirty says whether removing the checkout would lose work.
func (c Check) Dirty() bool { return c.Changes || c.Ignored || c.Unreachable }

// Inspect looks at a checkout as it is now.
func Inspect(record Record) (Check, error) {
	present := isDir(record.Path)
	if repositoryGone(record) {
		return Check{Missing: !present, Forgotten: true}, nil
	}
	if !present {
		return inspectMissing(record)
	}
	status, err := gitOutput(record.Path, "status", "--porcelain", "-z", "--untracked-files=normal", "--ignored=matching")
	if err != nil {
		return Check{}, fmt.Errorf("git status in %s failed: %w", record.Path, err)
	}
	head, err := gitOutput(record.Path, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	if err != nil {
		return Check{}, fmt.Errorf("cannot read HEAD in %s: %w", record.Path, err)
	}
	check := Check{Head: head}
	if check.Branch, _, err = currentBranch(record.Path); err != nil {
		return Check{}, err
	}
	for _, entry := range strings.Split(status, "\x00") {
		switch {
		case strings.HasPrefix(entry, "!! "):
			// A copy .worktreeinclude made, still equal to the source's,
			// is no work of the checkout's.
			if !includedCopy(record, strings.TrimPrefix(entry, "!! ")) {
				check.Ignored = true
			}
		case entry != "":
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
// has forgotten leaves only the record to remove. A directory whose repository
// is gone has no git left to remove it, and only force deletes it. It holds
// the repository's lock, as Create does: git worktree remove reads the
// entries of the others as well. A root that is gone takes no lock, and is not
// made again for one: no checkout is made in it without Create making it
// first, and a root the person removed should stay removed.
func Remove(record Record, force bool) error {
	directory := filepath.Dir(record.Path)
	if _, err := os.Stat(filepath.Dir(directory)); errors.Is(err, fs.ErrNotExist) {
		return remove(record, force)
	}
	return withRepositoryLock(directory, func() error { return remove(record, force) })
}

func remove(record Record, force bool) error {
	present := isDir(record.Path)
	forgotten := repositoryGone(record)
	if !forgotten && !present {
		_, listed, err := listedHead(record)
		if err != nil {
			return err
		}
		forgotten = !listed
	}
	switch {
	case !forgotten:
		args := []string{"worktree", "remove"}
		if force {
			args = append(args, "--force")
		}
		if err := gitDir(record.CommonDir, append(args, record.Path)...); err != nil {
			return fmt.Errorf("git worktree remove failed: %w", err)
		}
	case present:
		if !force {
			return fmt.Errorf("the repository of %s is gone, so git cannot tell what it holds; only a forced removal deletes it", record.Path)
		}
		if err := os.RemoveAll(record.Path); err != nil {
			return err
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

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// repositoryGone says whether the Git directory a checkout was made from no
// longer exists: the repository was deleted, and git with it can answer
// nothing about the checkout. Any other failure to look is not an answer.
func repositoryGone(record Record) bool {
	_, err := os.Stat(record.CommonDir)
	return errors.Is(err, os.ErrNotExist)
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
	_, err := run(gitDirCommand(commonDir, args...))
	return err
}

func gitOutput(dir string, args ...string) (string, error) {
	return run(exec.Command("git", append([]string{"-C", dir}, args...)...))
}

func gitDirOutput(commonDir string, args ...string) (string, error) {
	if _, err := os.Stat(commonDir); err != nil {
		return "", err
	}
	return run(gitDirCommand(commonDir, args...))
}

// gitDirCommand runs in the Git directory as well: the process may stand in a
// checkout that is gone by now — the one a launch ran in, or the one finish
// just removed — and git refuses to start in a directory that does not exist.
func gitDirCommand(commonDir string, args ...string) *exec.Cmd {
	command := exec.Command("git", append([]string{"--git-dir", commonDir}, args...)...)
	command.Dir = commonDir
	return command
}

// run runs git with an environment of its own choosing, and without the
// repository's hooks.
func run(command *exec.Cmd) (string, error) {
	return runGit(command, false)
}

// runWithHooks runs git as a person would in their own checkout, hooks
// included: land moves the source's branch, and the hooks the repository keeps
// for a merge or a ref update are the person's to run there.
func runWithHooks(command *exec.Cmd) (string, error) {
	return runGit(command, true)
}

// keptGitEnv are the GIT_ variables that pass: which configuration files the
// person's git reads, and the identity it signs with. None of them points at a
// repository or changes what a command does to one.
var keptGitEnv = []string{
	"GIT_CONFIG_GLOBAL", "GIT_CONFIG_SYSTEM", "GIT_CONFIG_NOSYSTEM",
	"GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_AUTHOR_DATE",
	"GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL", "GIT_COMMITTER_DATE",
}

// runGit leaves out every other inherited GIT_ variable: a GIT_DIR or a
// GIT_WORK_TREE from a hook that started this would point every call at
// another repository, and GIT_CONFIG_PARAMETERS or GIT_CONFIG_COUNT would carry
// a parent's configuration into it.
//
// Nothing it runs may wait for a person, since the caller is most often an
// agent: no terminal prompt for credentials a hook or a filter might ask for,
// and a session of its own, so git has no terminal to open at all. A look must
// not stand in a session's way: `git status` otherwise refreshes the index
// under index.lock, and a commit the session makes at that moment fails. No
// filesystem monitor is started for a checkout: its daemon would outlive the
// call. And unless hooks are asked for, none runs: a post-checkout hook in a
// new checkout would run the repository's code with no terminal before the
// session starts — the preparation rewake does not do — and a reference hook
// would run on a branch rewake makes or drops. Clean and smudge filters stay:
// without them a large file kept by a filter would be checked out as its
// pointer. The settings go by GIT_CONFIG_COUNT, for this call only; nothing
// is written to the repository's configuration.
func runGit(command *exec.Cmd, hooks bool) (string, error) {
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "GIT_") && !slices.Contains(keptGitEnv, key) {
			continue
		}
		command.Env = append(command.Env, entry)
	}
	command.Env = append(command.Env, "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0", "GIT_MERGE_AUTOEDIT=no")
	settings := [][2]string{{"core.fsmonitor", "false"}}
	if !hooks {
		settings = append(settings, [2]string{"core.hooksPath", os.DevNull})
	}
	command.Env = append(command.Env, fmt.Sprintf("GIT_CONFIG_COUNT=%d", len(settings)))
	for i, setting := range settings {
		command.Env = append(command.Env, fmt.Sprintf("GIT_CONFIG_KEY_%d=%s", i, setting[0]), fmt.Sprintf("GIT_CONFIG_VALUE_%d=%s", i, setting[1]))
	}
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
