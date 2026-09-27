package worktree

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// heldElsewhere refuses a target checked out in a checkout other than the
// source: whoever works there would have files and HEAD change under them.
func heldElsewhere(record Record, target, tip, checkout string) error {
	if samePlace(checkout, record.Path) {
		return &StateError{Reason: fmt.Sprintf("%s is checked out in the worktree itself, %s, which left its branch %s; land only from the branch: git -C %s switch %s goes back",
			target, checkout, record.Branch, checkout, record.Branch)}
	}
	return &StateError{Reason: fmt.Sprintf("%s is checked out in %s, not in %s the worktree was made from, and a merge there would change files under whoever works in it; land it from there with git -C %s merge --ff-only %s, or name another branch with --into",
		target, checkout, record.Source, checkout, tip)}
}

// samePlace says whether two paths name one directory, symbolic links
// resolved: git lists a checkout by the path it was added at, and a record
// keeps the one the launch resolved.
func samePlace(one, other string) bool {
	return resolved(one) == resolved(other)
}

// underWay refuses moving a branch that a rebase or a bisect in some checkout
// of the repository is working on. Git lists such a checkout as detached, so
// checkedOutAt does not see it, and a branch moved under a rebase makes its
// last step fail and looks like lost work. It reads what git itself asks
// before it lets a branch move: the rebase's head-name and BISECT_START, in
// every checkout's own Git directory.
func underWay(commonDir, branch string) error {
	ref := branchRef(branch)
	for _, dir := range checkoutGitDirs(commonDir) {
		place := checkoutOf(commonDir, dir)
		for _, rebase := range []string{"rebase-merge", "rebase-apply"} {
			if name, err := os.ReadFile(filepath.Join(dir, rebase, "head-name")); err == nil && strings.TrimSpace(string(name)) == ref {
				return &StateError{Reason: fmt.Sprintf("a rebase of %s is under way in %s, and moving the branch now would make its last step fail; finish it with git -C %s rebase --continue, or give it up with git -C %s rebase --abort, then land again",
					branch, place, place, place)}
			}
		}
		if start, err := os.ReadFile(filepath.Join(dir, "BISECT_START")); err == nil {
			if name := strings.TrimSpace(string(start)); name == branch || name == ref {
				return &StateError{Reason: fmt.Sprintf("a bisect that started on %s is under way in %s, and it goes back to the branch when it ends; end it with git -C %s bisect reset, then land again",
					branch, place, place)}
			}
		}
	}
	return nil
}

// checkoutGitDirs lists the Git directories of the repository's checkouts:
// the shared one, which is the main checkout's, and one under worktrees/ for
// each linked checkout.
func checkoutGitDirs(commonDir string) []string {
	dirs := []string{commonDir}
	linked, _ := filepath.Glob(filepath.Join(commonDir, "worktrees", "*"))
	return append(dirs, linked...)
}

// checkoutOf names the checkout a Git directory belongs to, for a refusal: the
// path its gitdir file records for a linked one, the directory holding .git
// for the main one, or the Git directory itself when neither says.
func checkoutOf(commonDir, dir string) string {
	if dir == commonDir {
		if filepath.Base(dir) == ".git" {
			return filepath.Dir(dir)
		}
		return dir
	}
	if gitFile, err := os.ReadFile(filepath.Join(dir, "gitdir")); err == nil {
		return filepath.Dir(strings.TrimSpace(string(gitFile)))
	}
	return dir
}
