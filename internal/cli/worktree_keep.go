package cli

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
	"github.com/praline-labs/rewake/internal/worktree"
)

// fateOfBranch deletes a removed checkout's branch when nothing goes with it,
// and says why it stays, or why there was none to delete: each case has its
// own next step, and advice to merge a branch that is gone, or held by another
// checkout, would send the reader the wrong way.
func (r *worktreeRemoval) fateOfBranch(record worktree.Record) {
	if record.Branch == "" {
		return
	}
	dropping, err := worktree.DropBranch(record)
	switch {
	case err != nil:
		r.BranchKept = err.Error()
	case dropping.Dropped:
		r.BranchDropped = true
	case dropping.Gone != "":
		r.BranchGone = dropping.Gone
	case dropping.CheckedOut != "":
		r.BranchKept = "it is checked out in " + dropping.CheckedOut + "; git branch -D " + record.Branch + " drops it once that checkout has left it"
	default:
		r.BranchKept = "it holds commits no other branch, tag or remote-tracking ref has; git -C " + record.Source + " merge --ff-only " + record.Branch + " takes them, git -C " + record.Source + " branch -D " + record.Branch + " drops them"
	}
}

// keepReasons says what removing a checkout would lose or cut off. Whatever
// cannot be told is an error, and rm then refuses too: without --force it
// never loses work.
func keepReasons(record worktree.Record) ([]string, error) {
	reasons, _, err := keepReasonsAndCheck(record)
	return reasons, err
}

// keepReasonsAndCheck is keepReasons with the look at the checkout it took.
func keepReasonsAndCheck(record worktree.Record) ([]string, worktree.Check, error) {
	var reasons []string
	running, err := runningIn(record)
	if err != nil {
		return nil, worktree.Check{}, err
	}
	if len(running) > 0 {
		reasons = append(reasons, "rewake sessions still run in it: "+strings.Join(running, ", "))
	}
	if record.Launching() {
		// Made and not yet claimed: the session registers only after the
		// checkout is in place, and until then nothing else says it is used.
		reasons = append(reasons, "a rewake launch that made it is still starting its session")
	}
	check, err := worktree.Inspect(record)
	if err != nil {
		return nil, worktree.Check{}, err
	}
	if check.Missing && !check.Forgotten {
		reasons = append(reasons, "its directory "+record.Path+" is gone while the repository still lists it; if it was moved, git worktree repair <new path> run in the repository reconnects it, and whatever it holds is out of sight here")
	}
	if check.Forgotten && !check.Missing {
		reasons = append(reasons, "its repository "+record.CommonDir+" is gone, so git cannot tell what "+record.Path+" holds")
	}
	if check.Changes {
		reasons = append(reasons, "it has changes git status shows")
	}
	if check.Ignored {
		reasons = append(reasons, "it holds files git ignores, a .env or a local build, which removal deletes")
	}
	if check.Unreachable {
		reasons = append(reasons, "its HEAD "+shortCommit(check.Head)+" is on no branch, tag or remote-tracking ref, and those commits would be left to garbage collection")
	}
	if check.Locked {
		why := ""
		if check.LockReason != "" {
			why = " (" + check.LockReason + ")"
		}
		reasons = append(reasons, "it is locked with git worktree lock"+why+", which says somebody wants it kept; git worktree unlock "+record.Path+" lets it go")
	}
	if check.Submodules {
		reasons = append(reasons, "it holds a submodule checked out, which git worktree remove refuses to take along")
	}
	return reasons, check, nil
}

// runningIn names the rewake sessions still running with their work in a
// checkout: the one it was made for, and any started there since — after the
// first ended, or by hand in its directory, as a taken name's refusal
// suggests. It looks in every room of the current state directory and of the
// one the owner registered in. A process rewake did not start is not seen.
func runningIn(record worktree.Record) ([]string, error) {
	var roots []string
	if root, err := state.Root(); err == nil {
		roots = append(roots, root)
	}
	owner := record.Session
	if owner != nil && owner.Dir != "" {
		if root := state.RootForRoom(owner.Dir); !slices.Contains(roots, root) {
			roots = append(roots, root)
		}
	}
	var found []string
	for _, root := range roots {
		rooms, err := state.RoomDirs(root)
		if err != nil {
			return nil, fmt.Errorf("cannot list the rooms of %s: %w", root, err)
		}
		for _, room := range rooms {
			sessions, err := registry.ListReadOnly(room)
			if err != nil {
				return nil, fmt.Errorf("cannot list the sessions of %s: %w", room, err)
			}
			for _, session := range sessions {
				made := owner != nil && filepath.Clean(owner.Dir) == room && session.Name == owner.Name && session.Epoch() == owner.Epoch
				label := session.Name + " in room " + filepath.Base(room)
				if session.Alive() && (made || worktree.Within(session.CWD, record.Path)) && !slices.Contains(found, label) {
					found = append(found, label)
				}
			}
		}
	}
	return found, nil
}
