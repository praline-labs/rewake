package codex

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/grant"
	"github.com/iiiokojiadbi/rewake/internal/grantauth"
	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
)

// A cold resume restores the roots the conversation last saved, a grant
// among them, into a run whose journal is empty: rewake would never take that
// grant back. And a grant given on the conversation's first turn is not saved
// at all, so the resumed conversation has lost it (docs/grants.md#after-a-cold-resume).
//
// So at the first notice into a conversation, the adapter looks in the copies
// of the journals for the grants it had, and asks the main that sent each to
// confirm it again for this run. A confirmed grant is journaled here, and a
// root it lost is added back; a grant not confirmed — its task closed, its
// main gone, the copy forged — has its roots taken out, so a copy nobody
// confirms holds nothing. A root no copy names is left alone: rewake cannot
// tell it from one the person gave.

// resumedHints are the grants the copies name for a conversation that this run
// has not looked at yet, less those it journaled itself.
func (s *serverSession) resumedHints(thread string, entries []grant.Entry) []grant.Hint {
	if thread == "" || s.mailbox == "" || s.followedThread(thread) {
		return nil
	}
	var hints []grant.Hint
	for _, hint := range grant.Hints(s.mailbox, thread) {
		if !slices.ContainsFunc(entries, func(entry grant.Entry) bool { return entry.Message == hint.Message }) {
			hints = append(hints, hint)
		}
	}
	return hints
}

func (s *serverSession) followedThread(thread string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.followed[thread]
}

// follow records a conversation whose grants from before a resume a notice
// the harness took has settled.
func (s *serverSession) follow(thread string) {
	if thread == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.followed == nil {
		s.followed = map[string]bool{}
	}
	s.followed[thread] = true
}

// restoreRoots confirms each hinted grant again and changes the roots to
// match: a confirmed grant journaled, its lost roots added back; one refused
// taken out. It returns the notes, whether the journal changed, and the
// conversation once no main is left to answer.
func (s *serverSession) restoreRoots(thread string, hints []grant.Hint, roots *[]string, entries *[]grant.Entry, environment threadEnvironment, now time.Time) (notes []string, journaled bool, followed string) {
	rules := grant.CurrentEnv(s.stateRoot, harness.AllProtectedDirs()).Rules()
	waiting := false
	for _, hint := range hints {
		restored := grantauth.RestoreHint(s.mailbox, s.name, s.epoch, thread, hint)
		err := restored.Err
		if errors.Is(err, grantauth.ErrUnreachable) {
			// Its main is running and did not answer: asked again at the
			// next notice, the roots left as they are until then.
			waiting = true
			continue
		}
		confirmed := restored.Grant
		if err == nil {
			for _, directory := range confirmed.Dirs {
				if err = rules.Recheck(directory, slices.Contains(confirmed.Broad, directory)); err != nil {
					break
				}
			}
		}
		// Out first, confirmed or not: a confirmed root goes back in below
		// and is journaled, rather than taken for one the thread had anyway.
		taken := takeRoots(roots, hint.Paths, environment.Cwd)
		if err == nil {
			message := inbox.Message{ID: confirmed.ID, From: hint.From, FromEpoch: hint.FromEpoch, GrantDirs: confirmed.Dirs, GrantBroad: confirmed.Broad, GrantGit: confirmed.Git}
			var added []string
			added, _, _, err = s.addGrantedRoots(roots, entries, message, confirmed.Git && s.gitWrite, environment.Cwd, thread, now)
			if err == nil {
				journaled = true
				if len(added) > 0 {
					notes = append(notes, fmt.Sprintf("restored after the resume, confirmed again by %s: %s", hint.From, strings.Join(added, ", ")))
				}
				continue
			}
		}
		if len(taken) > 0 {
			notes = append(notes, fmt.Sprintf("taken back after the resume, the grant of task %s not confirmed again: %s (%v)", hint.Message, strings.Join(taken, ", "), err))
		}
	}
	if !waiting {
		followed = thread
	}
	return notes, journaled, followed
}

// takeRoots takes the named paths out of the roots, the launch directory
// excepted, and returns those it took.
func takeRoots(roots *[]string, paths []string, cwd string) []string {
	var taken []string
	for _, path := range paths {
		if path == cwd {
			continue
		}
		if at := slices.IndexFunc(*roots, func(root string) bool { return hasRoot([]string{root}, path) }); at >= 0 {
			*roots = slices.Delete(*roots, at, at+1)
			taken = append(taken, path)
		}
	}
	return taken
}
