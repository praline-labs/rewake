package worktree

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Landing is what land did: the branch it took, the branch it moved and how.
type Landing struct {
	Branch string `json:"branch"`
	Target string `json:"target"`
	Old    string `json:"old"`
	New    string `json:"new"`
	// Commits is how many commits the target gained; 0 when it held the
	// branch already.
	Commits int `json:"commits"`
	// Checkout is where the target is checked out and was fast-forwarded,
	// or empty when it is checked out nowhere and only its ref moved.
	Checkout string `json:"checkout,omitempty"`
}

// DivergedError is a target that moved on since the branch left it: no
// fast-forward reaches it, and rewake does not rewrite history.
type DivergedError struct {
	Branch string
	Target string
	Path   string
}

func (e *DivergedError) Error() string {
	return fmt.Sprintf("%s cannot be fast-forwarded to %s: %s has commits %s does not", e.Target, e.Branch, e.Target, e.Branch)
}

// RefusedError is a fast-forward git refused in the checkout holding the
// target: changes there that the merge would overwrite, most often.
type RefusedError struct {
	Checkout string
	Reason   string
}

func (e *RefusedError) Error() string {
	return fmt.Sprintf("git refused the fast-forward in %s: %s", e.Checkout, e.Reason)
}

// Land fast-forwards a branch of the repository to the checkout's branch:
// into, or the branch checked out in the source when into is empty. Commits
// keep their hashes: a fast-forward or nothing. A target checked out in the
// source is merged there, so its files follow; one checked out nowhere has its
// ref moved, and only if it is still where it was read. A target checked out in
// any other checkout is refused: that may be a worker's, the worktree's own
// among them, and a merge there would change files and HEAD under it.
func Land(record Record, into string) (Landing, error) {
	// legacy(rewake <2026-09-27): records of earlier builds name no branch, their checkouts are detached; remove when no such record is left under the worktree root
	if record.Branch == "" {
		return Landing{}, &StateError{Reason: fmt.Sprintf("%s is a detached checkout from before rewake gave worktrees a branch, so it has no branch to land; make one in it with git switch -c <name> and merge that by hand", record.Ref())}
	}
	if repositoryGone(record) {
		return Landing{}, &StateError{Reason: fmt.Sprintf("the repository %s of %s is gone", record.CommonDir, record.Ref())}
	}
	tip, err := refTip(record.CommonDir, branchRef(record.Branch))
	if err != nil {
		return Landing{}, err
	}
	if tip == "" {
		return Landing{}, &StateError{Reason: fmt.Sprintf("the branch %s of %s is gone", record.Branch, record.Ref())}
	}
	target, err := landTarget(record, into)
	if err != nil {
		return Landing{}, err
	}
	old, err := refTip(record.CommonDir, branchRef(target))
	if err != nil {
		return Landing{}, err
	}
	if old == "" {
		return Landing{}, &StateError{Reason: fmt.Sprintf("the repository at %s has no branch %s to land into", record.Source, target)}
	}
	landing := Landing{Branch: record.Branch, Target: target, Old: old, New: old}
	if held, err := isAncestor(record.CommonDir, tip, old); err != nil || held {
		return landing, err
	}
	if forward, err := isAncestor(record.CommonDir, old, tip); err != nil {
		return Landing{}, err
	} else if !forward {
		return Landing{}, &DivergedError{Branch: record.Branch, Target: target, Path: record.Path}
	}
	count, err := gitDirOutput(record.CommonDir, "rev-list", "--count", old+".."+tip)
	if err != nil {
		return Landing{}, err
	}
	if landing.Commits, err = strconv.Atoi(count); err != nil {
		return Landing{}, fmt.Errorf("git rev-list --count said %q", count)
	}
	if err := underWay(record.CommonDir, target); err != nil {
		return Landing{}, err
	}
	checkouts, err := checkedOutAt(record.CommonDir, branchRef(target))
	if err != nil {
		return Landing{}, err
	}
	for _, checkout := range checkouts {
		if !samePlace(checkout, record.Source) {
			return Landing{}, heldElsewhere(record, target, tip, checkout)
		}
	}
	if len(checkouts) > 0 {
		landing.Checkout = checkouts[0]
		// The commit, not the branch's name: the branch may move between the
		// look above and the merge, and what was counted is what lands.
		if _, err := runWithHooks(exec.Command("git", "-C", landing.Checkout, "merge", "--ff-only", "--quiet", tip)); err != nil {
			return Landing{}, &RefusedError{Checkout: landing.Checkout, Reason: oneLine(err.Error())}
		}
	} else if _, err := runWithHooks(gitDirCommand(record.CommonDir, "update-ref", "-m", "rewake worktree land "+record.Ref(), branchRef(target), tip, old)); err != nil {
		return Landing{}, fmt.Errorf("git update-ref of %s failed: %w", target, err)
	}
	if landing.New, err = refTip(record.CommonDir, branchRef(target)); err != nil {
		return Landing{}, err
	}
	return landing, nil
}

// landTarget is the branch to land into: into, or the one checked out in the
// source.
func landTarget(record Record, into string) (string, error) {
	target := strings.TrimPrefix(into, "refs/heads/")
	if target != "" {
		// A name git would refuse, and a glob for-each-ref would expand.
		if _, err := gitDirOutput(record.CommonDir, "check-ref-format", "--branch", target); err != nil || strings.HasPrefix(target, "-") {
			return "", &UnusableError{Reason: fmt.Sprintf("%q is not a branch name git accepts", into)}
		}
	} else {
		if !isDir(record.Source) {
			return "", &StateError{Reason: fmt.Sprintf("the checkout %s the worktree was made from is gone; name the branch to land into with --into <branch>", record.Source)}
		}
		head, detached, err := currentBranch(record.Source)
		if err != nil {
			return "", err
		}
		if detached {
			return "", &StateError{Reason: fmt.Sprintf("%s has no branch checked out; name the branch to land into with --into <branch>", record.Source)}
		}
		target = head
	}
	if target == record.Branch {
		return "", &UnusableError{Reason: fmt.Sprintf("%s is the worktree's own branch; land it into another", target)}
	}
	return target, nil
}

// DropBranch deletes a checkout's branch when nothing goes with it: another
// branch, tag or remote-tracking ref holds its tip, and no checkout has it
// out. It says whether the branch went; one that stays holds commits only it
// has, or is checked out somewhere.
func DropBranch(record Record) (bool, error) {
	if record.Branch == "" || repositoryGone(record) {
		return false, nil
	}
	ref := branchRef(record.Branch)
	tip, err := refTip(record.CommonDir, ref)
	if err != nil || tip == "" {
		return false, err
	}
	holders, err := gitDirOutput(record.CommonDir, "for-each-ref", "--contains", tip, "--format=%(refname)", "refs/heads", "refs/tags", "refs/remotes")
	if err != nil {
		return false, err
	}
	held := false
	for _, holder := range strings.Split(holders, "\n") {
		held = held || holder != "" && holder != ref
	}
	if !held {
		return false, nil
	}
	if checkouts, err := checkedOutAt(record.CommonDir, ref); err != nil || len(checkouts) > 0 {
		return false, err
	}
	// The tip read above, so a commit made since keeps the branch.
	if err := gitDir(record.CommonDir, "update-ref", "-d", ref, tip); err != nil {
		return false, fmt.Errorf("git update-ref -d %s failed: %w", ref, err)
	}
	return true, nil
}

func branchRef(branch string) string { return "refs/heads/" + branch }

// refTip is the commit a full ref names, or "" when there is no such ref.
// for-each-ref takes the name as a prefix of whole components, so
// refs/heads/foo lists refs/heads/foo/bar as well, and only the line of the
// name itself counts. Not rev-parse: a full name it does not find it tries
// again under refs/heads/ and the rest, and would take a branch named
// refs/heads/foo for the missing foo.
func refTip(commonDir, ref string) (string, error) {
	out, err := gitDirOutput(commonDir, "for-each-ref", "--format=%(objectname) %(refname)", ref)
	if err != nil {
		return "", fmt.Errorf("cannot read %s: %w", ref, err)
	}
	for _, line := range strings.Split(out, "\n") {
		if tip, name, ok := strings.Cut(line, " "); ok && name == ref {
			return tip, nil
		}
	}
	return "", nil
}

// isAncestor says whether ancestor is in the history of commit, a commit
// being its own ancestor.
func isAncestor(commonDir, ancestor, commit string) (bool, error) {
	_, err := gitDirOutput(commonDir, "merge-base", "--is-ancestor", ancestor, commit)
	var exit *exec.ExitError
	switch {
	case err == nil:
		return true, nil
	case errors.As(err, &exit) && exit.ExitCode() == 1:
		return false, nil
	}
	return false, fmt.Errorf("git merge-base --is-ancestor %s %s failed: %w", ancestor, commit, err)
}

// currentBranch is the branch checked out in dir, or detached when it has
// none.
func currentBranch(dir string) (string, bool, error) {
	out, err := gitOutput(dir, "symbolic-ref", "--quiet", "HEAD")
	var exit *exec.ExitError
	switch {
	case err == nil:
		return strings.TrimPrefix(out, "refs/heads/"), false, nil
	case errors.As(err, &exit) && exit.ExitCode() == 1:
		return "", true, nil
	}
	return "", false, fmt.Errorf("cannot read the branch checked out in %s: %w", dir, err)
}

// checkedOutAt lists the checkouts of the repository that have a branch out.
func checkedOutAt(commonDir, ref string) ([]string, error) {
	out, err := gitDirOutput(commonDir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("cannot list the worktrees of %s: %w", commonDir, err)
	}
	var found []string
	for _, block := range strings.Split(out, "\n\n") {
		path, branch := "", ""
		for _, line := range strings.Split(block, "\n") {
			if value, ok := strings.CutPrefix(line, "worktree "); ok {
				path = value
			} else if value, ok := strings.CutPrefix(line, "branch "); ok {
				branch = value
			}
		}
		if path != "" && branch == ref {
			found = append(found, path)
		}
	}
	return found, nil
}

// oneLine joins git's lines, a list of files included, into one.
func oneLine(message string) string {
	var parts []string
	for _, line := range strings.Split(message, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			parts = append(parts, line)
		}
	}
	return strings.Join(parts, " ")
}
