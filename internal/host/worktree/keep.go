package worktree

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// KeepReasons says what removing a checkout would lose or cut off. Whatever
// cannot be told is an error, and rm then refuses too: without --force it
// never loses work.
func KeepReasons(record Record) ([]string, error) {
	reasons, _, err := KeepReasonsAndCheck(record)
	return reasons, err
}

// KeepReasonsAndCheck is KeepReasons with the look at the checkout it took.
func KeepReasonsAndCheck(record Record) ([]string, Check, error) {
	return keepReasons(record, true, nil)
}

// keepReasons is KeepReasonsAndCheck, with files git ignores a reason only
// when ignored says so: rm and finish are the person's own commands and keep
// them, the close's release lets them go (Release). A run named ending is not
// counted as working there: the close forecast asks on behalf of the worker
// whose end it foresees (CloseForecast).
func keepReasons(record Record, ignored bool, ending *Owner) ([]string, Check, error) {
	var reasons []string
	running, err := runningIn(record, ending)
	if err != nil {
		return nil, Check{}, err
	}
	if len(running) > 0 {
		reasons = append(reasons, "rewake sessions still run in it: "+strings.Join(running, ", "))
	}
	if record.Launching() {
		// Made and not yet claimed: the session registers only after the
		// checkout is in place, and until then nothing else says it is used.
		if record.Launcher.Judgeable() {
			reasons = append(reasons, "a rewake launch that made it is still starting its session")
		} else {
			reasons = append(reasons, "a rewake launch made it and has not claimed it yet, and from here rewake cannot tell whether that launch still runs — it started in another pid namespace, or /proc cannot be read; run this where the launch ran, or use --force once you know it has ended")
		}
	}
	check, err := Inspect(record)
	if err != nil {
		return nil, Check{}, err
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
	if ignored && check.Ignored {
		reasons = append(reasons, "it holds files git ignores, a .env or a local build, which removal deletes")
	}
	if check.Hidden {
		reasons = append(reasons, "a tracked file in it is marked assume-unchanged or skip-worktree, so git status cannot show its edits, which removal deletes; git ls-files -v names it")
	}
	if check.Unreachable {
		reasons = append(reasons, "its HEAD "+short(check.Head)+" is on no branch, tag or remote-tracking ref, and those commits would be left to garbage collection")
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

// RunningIn names the rewake sessions still running with their work in a
// checkout: the one it was made for, and any started there since — after the
// first ended, or by hand in its directory, as a taken name's refusal
// suggests. It looks in every room of the current state directory and of the
// one the owner registered in. A process rewake did not start is not seen, nor
// a session of 1.x: an owner record naming a room of another layout fails the
// root's check and is skipped.
func RunningIn(record Record) ([]string, error) {
	return runningIn(record, nil)
}

// runningIn is RunningIn with the run except names left out, wherever its
// working directory is.
func runningIn(record Record, except *Owner) ([]string, error) {
	var roots []string
	if root, err := state.Root(); err == nil {
		roots = append(roots, root)
	}
	owner := record.Session
	if owner != nil && owner.Dir != "" {
		if root, err := state.RootForRoom(owner.Dir); err == nil && !slices.Contains(roots, root) {
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
				if except != nil && except.is(room, session) {
					continue
				}
				made := owner != nil && owner.is(room, session)
				label := session.Name + " in room " + filepath.Base(room)
				if session.Alive() && (made || Within(session.CWD, record.Path)) && !slices.Contains(found, label) {
					found = append(found, label)
				}
			}
		}
	}
	return found, nil
}

// is says whether a session of room is this owner's run.
func (o *Owner) is(room string, session registry.Session) bool {
	return filepath.Clean(o.Dir) == room && session.Name == o.Name && session.Epoch() == o.Epoch
}

// short is a commit as the person reads it.
func short(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return strings.TrimSpace(commit)
}
