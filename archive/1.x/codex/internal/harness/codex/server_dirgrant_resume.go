package codex

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/praline-labs/rewake/internal/grant"
	"github.com/praline-labs/rewake/internal/grantauth"
	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/inbox"
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

// resumeWait is why a notice into a resumed conversation waits: its grants are
// being confirmed again, and each main may take seconds to answer.
const resumeWait = "the grants this conversation had before its resume are being confirmed again by the mains that sent them"

// answerLife bounds how long an answer is used for: a notice that kept
// failing to go out asks again rather than act on what a main said long ago.
const answerLife = 30 * time.Second

// resumeAnswers are the mains' answers for one conversation, by message.
type resumeAnswers struct {
	asking bool
	at     time.Time
	by     map[string]grantauth.Restored
}

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
// the harness took has settled, and lets go of the answers it used.
func (s *serverSession) follow(thread, followed string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if answers := s.answers[thread]; answers != nil && !answers.asking {
		delete(s.answers, thread)
	}
	if followed == "" {
		return
	}
	if s.followed == nil {
		s.followed = map[string]bool{}
	}
	s.followed[followed] = true
}

// confirming says whether a notice into the conversation has to wait for its
// grants to be confirmed again, and starts asking when nobody has yet. The
// mains are asked in the background, all at once, while the notice holds no
// reservation: each may take its full exchange to answer, and a reservation
// held through them in turn would lapse and fail the notice for good.
func (s *serverSession) confirming(thread string) bool {
	hints := s.resumedHints(thread, s.grantJournal())
	if len(hints) == 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	answers := s.answers[thread]
	if answers != nil && answers.asking {
		return true
	}
	if answers != nil && time.Since(answers.at) < answerLife && !slices.ContainsFunc(hints, func(hint grant.Hint) bool {
		_, asked := answers.by[hint.Message]
		return !asked
	}) {
		return false
	}
	if s.answers == nil {
		s.answers = map[string]*resumeAnswers{}
	}
	s.answers[thread] = &resumeAnswers{asking: true}
	go s.ask(thread, hints)
	return true
}

// ask puts each hint to the main that sent it, and keeps what they answered.
func (s *serverSession) ask(thread string, hints []grant.Hint) {
	outcomes := make([]grantauth.Restored, len(hints))
	var wait sync.WaitGroup
	for index, hint := range hints {
		wait.Go(func() { outcomes[index] = grantauth.RestoreHint(s.mailbox, s.name, s.epoch, thread, hint) })
	}
	wait.Wait()
	by := map[string]grantauth.Restored{}
	for _, outcome := range outcomes {
		by[outcome.Hint.Message] = outcome
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.answers[thread] = &resumeAnswers{at: time.Now(), by: by}
}

// answersFor are the answers kept for a conversation, none while they are
// still being asked for or once they are too old to act on.
func (s *serverSession) answersFor(thread string) map[string]grantauth.Restored {
	s.mu.Lock()
	defer s.mu.Unlock()
	answers := s.answers[thread]
	if answers == nil || answers.asking || time.Since(answers.at) >= answerLife {
		return nil
	}
	return answers.by
}

// restoreRoots changes the roots to match what the mains answered for each
// hinted grant: every root a settled hint names is taken out, and then the
// roots of each confirmed grant are added back and journaled. Taking every
// one out first is what lets two copies naming one directory end the same in
// either order: a refused hint cannot take out a root a confirmed one holds.
// A hint with no answer yet leaves the roots as they are. It returns the
// notes, whether the journal changed, and the conversation once no main is
// left to answer.
func (s *serverSession) restoreRoots(thread string, hints []grant.Hint, roots *[]string, entries *[]grant.Entry, environment threadEnvironment, now time.Time) (notes []string, journaled bool, followed string) {
	rules := grant.CurrentEnv(s.stateRoot, harness.AllProtectedDirs()).Rules()
	answers := s.answersFor(thread)
	waiting := false
	type decided struct {
		hint    grant.Hint
		granted grantauth.Grant
		err     error
		taken   []string
	}
	var decisions []decided
	for _, hint := range hints {
		restored, asked := answers[hint.Message]
		if !asked || errors.Is(restored.Err, grantauth.ErrUnreachable) {
			// Its main is running and did not answer, or was not asked yet:
			// asked again at the next notice, the roots left as they are
			// until then.
			waiting = true
			continue
		}
		decision := decided{hint: hint, granted: restored.Grant, err: restored.Err}
		if decision.err == nil {
			for _, directory := range decision.granted.Dirs {
				if decision.err = rules.Recheck(directory, slices.Contains(decision.granted.Broad, directory)); decision.err != nil {
					break
				}
			}
		}
		decisions = append(decisions, decision)
	}
	// Out first, confirmed or not: a confirmed root goes back in below and is
	// journaled, rather than taken for one the thread had anyway. A root a
	// grant journaled in this conversation holds stays: restored at an earlier
	// notice, it is no longer a hint, and a hint refused now for the same
	// directory would take it from under a grant still counted as held.
	held := func(path string) bool {
		return slices.ContainsFunc(*entries, func(entry grant.Entry) bool {
			return entry.Live() && entry.Thread == thread && entry.Path == path
		})
	}
	for index := range decisions {
		paths := slices.DeleteFunc(slices.Clone(decisions[index].hint.Paths), held)
		decisions[index].taken = takeRoots(roots, paths, environment.Cwd)
	}
	for index := range decisions {
		decision := &decisions[index]
		if decision.err != nil {
			continue
		}
		confirmed := decision.granted
		message := inbox.Message{ID: confirmed.ID, From: decision.hint.From, FromEpoch: decision.hint.FromEpoch, GrantDirs: confirmed.Dirs, GrantBroad: confirmed.Broad, GrantGit: confirmed.Git}
		var added []string
		added, _, _, decision.err = s.addGrantedRoots(roots, entries, message, confirmed.Git && s.gitWrite, environment.Cwd, thread, now)
		if decision.err == nil {
			journaled = true
			if len(added) > 0 {
				notes = append(notes, fmt.Sprintf("restored after the resume, confirmed again by %s: %s", decision.hint.From, strings.Join(added, ", ")))
			}
		}
	}
	for _, decision := range decisions {
		if decision.err == nil {
			continue
		}
		// A root another hint's confirmed grant put back is not taken back.
		var gone []string
		for _, path := range decision.taken {
			if !hasRoot(*roots, path) {
				gone = append(gone, path)
			}
		}
		if len(gone) > 0 {
			notes = append(notes, fmt.Sprintf("taken back after the resume, the grant of task %s not confirmed again: %s (%v)", decision.hint.Message, strings.Join(gone, ", "), decision.err))
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
