package codex

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/grant"
	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/inbox"
)

// idleWait is why a notice carrying a grant waits: the roots a turn starts
// with are the ones it keeps, and a grant steered into a running turn would
// reach only its later work (docs/grants.md).
const idleWait = "a task carrying a grant waits for the session to be idle, so the grant holds from the task's first turn"

// rootsChange is what one notice does to the session's roots.
type rootsChange struct {
	// roots replace the thread's; nil leaves them as they are.
	roots   []string
	notes   []string
	applied []string
	// journal is saved once the notice is taken; nil when unchanged.
	journal []grant.Entry
	// followed is the conversation whose grants from before a resume this
	// notice settles; recorded once the notice is taken.
	followed string
}

// planRoots works out the roots a notice carries: the snapshot as read, less
// what rewake granted for tasks settled since, plus the thread's Git metadata
// for --grant-git and the directories the message grants.
func (s *serverSession) planRoots(thread string, read func() threadRead, git bool, granted inbox.Message) (rootsChange, error) {
	var change rootsChange
	entries := s.grantJournal()
	settled := s.settledGrants(entries)
	restoring := s.resumedHints(thread, entries)
	if restoring == nil && !s.followedThread(thread) {
		// Nothing a resume left to restore here; the next notice need not
		// look again.
		change.followed = thread
	}
	if !git && len(granted.GrantDirs) == 0 && len(settled) == 0 && restoring == nil {
		return change, nil
	}
	snapshot := read()
	environment, unavailable := snapshot.local(thread)
	if unavailable != "" {
		if len(granted.GrantDirs) > 0 {
			// A task is never handed over without the grant it was sent with.
			return change, fmt.Errorf("its directory grant could not be applied: %s", unavailable)
		}
		if git {
			change.notes = append(change.notes, "Git metadata access unchanged: "+unavailable)
		}
		// What settled tasks were granted waits for a later notice.
		return change, nil
	}
	roots := slices.Clone(environment.Roots)
	active := snapshot.status == "active"
	now := time.Now()
	journaled := false
	if restoring != nil {
		var notes []string
		notes, journaled, change.followed = s.restoreRoots(thread, restoring, &roots, &entries, environment, now)
		change.notes = append(change.notes, notes...)
		settled = s.settledGrants(entries)
	}
	var revoked []string
	for _, index := range settled {
		entry := &entries[index]
		entry.EndedAt = &now
		entry.Outcome = grant.Dropped
		// Two settled tasks can have held one directory: the root goes once,
		// and both took it back.
		if slices.Contains(revoked, entry.Path) {
			entry.Outcome = grant.Revoked
			continue
		}
		if at := slices.IndexFunc(roots, func(root string) bool { return hasRoot([]string{root}, entry.Path) }); at >= 0 {
			roots = slices.Delete(roots, at, at+1)
			entry.Outcome = grant.Revoked
			revoked = append(revoked, entry.Path)
		}
	}
	if len(revoked) > 0 {
		change.notes = append(change.notes, "taken back, their tasks reported on: "+strings.Join(revoked, ", "))
	}
	journaled = journaled || len(settled) > 0
	if git {
		if note := addGitRoots(&roots, environment, active); note != "" {
			change.notes = append(change.notes, note)
		}
	}
	if len(granted.GrantDirs) > 0 {
		added, writable, notes, err := s.addGrantedRoots(&roots, &entries, granted, git, environment.Cwd, thread, now)
		if err != nil {
			return change, err
		}
		change.notes = append(change.notes, notes...)
		change.applied = slices.Concat(added, writable)
		if len(added) > 0 {
			journaled = true
			note := "write granted: " + strings.Join(added, ", ")
			if active {
				note += ", for subsequent turns; the active turn keeps its existing permissions"
			}
			change.notes = append(change.notes, note)
		}
		if len(writable) > 0 {
			change.notes = append(change.notes, "already writable: "+strings.Join(writable, ", "))
		}
	}
	if journaled {
		change.journal = entries
	}
	if !slices.Equal(roots, environment.Roots) {
		change.roots = roots
	}
	return change, nil
}

// addGrantedRoots adds each granted directory a root does not cover already,
// with its Git metadata when the task also carries --grant-git, and journals
// what it added. A directory inside a root rewake itself granted for another
// task is refused: the worker writes in its parent through that grant, and
// could put a link in its place before the harness resolves it again
// (docs/grants.md#what-a-grant-does-not-stop). An error fails the task.
func (s *serverSession) addGrantedRoots(roots *[]string, entries *[]grant.Entry, granted inbox.Message, git bool, cwd, thread string, now time.Time) (added, writable, notes []string, err error) {
	if slices.ContainsFunc(*entries, func(entry grant.Entry) bool { return entry.Message == granted.ID }) {
		return nil, nil, nil, fmt.Errorf("message %s was granted once already", granted.ID)
	}
	journaled := func(root string) bool {
		return slices.ContainsFunc(*entries, func(entry grant.Entry) bool { return entry.Live() && entry.Path == root })
	}
	covered := func(directory string) bool {
		return slices.ContainsFunc(*roots, func(root string) bool { return grant.Within(directory, root) && !journaled(root) })
	}
	for _, directory := range granted.GrantDirs {
		for _, root := range *roots {
			if journaled(root) && root != directory && grant.Within(directory, root) {
				return nil, nil, nil, fmt.Errorf("%s lies inside %s, which another task's grant lets this session write, so it could be replaced by a link before it is used; grant it once that task is reported on", directory, root)
			}
		}
	}
	live := 0
	for _, entry := range *entries {
		if entry.Live() {
			live++
		}
	}
	if live+len(granted.GrantDirs) > grant.MaxLive {
		return nil, nil, nil, fmt.Errorf("this session holds %d granted directories, and %d more would pass the %d rewake keeps; wait for earlier tasks to be reported on", live, len(granted.GrantDirs), grant.MaxLive)
	}
	record := func(path, of string) {
		*entries = append(*entries, grant.Entry{
			Path: path, For: of, Message: granted.ID, At: now, Outcome: grant.Granted,
			Thread: thread, From: granted.From, FromEpoch: granted.FromEpoch,
		})
	}
	var rules *grant.Rules
	for _, directory := range granted.GrantDirs {
		if covered(directory) {
			writable = append(writable, directory)
		} else {
			// A root another live task was granted is journaled again for
			// this one, so it stays until both are reported on.
			if !hasRoot(*roots, directory) {
				*roots = append(*roots, directory)
			}
			record(directory, "")
			added = append(added, directory)
		}
		if !git {
			continue
		}
		metadata, err := gitMetadataDirectories(directory)
		if err != nil {
			notes = append(notes, fmt.Sprintf("no Git metadata granted for %s: %v", directory, err))
			continue
		}
		if err := sharedElsewhere(directory, metadata, cwd); err != nil {
			return nil, nil, nil, err
		}
		if rules == nil {
			built := grant.CurrentEnv(s.stateRoot, harness.AllProtectedDirs()).Rules()
			rules = &built
		}
		for _, path := range metadata {
			// Only a root that is the metadata itself covers it: the harness
			// keeps .git read-only inside every root, the one it lies in too.
			if hasRoot(*roots, path) && !journaled(path) {
				continue
			}
			// The metadata can lie outside the directory, and so outside
			// what was checked when the grant was sent.
			if err := rules.Check(path, path, false); err != nil {
				notes = append(notes, fmt.Sprintf("no Git metadata granted for %s: %v", directory, err))
				continue
			}
			if !hasRoot(*roots, path) {
				*roots = append(*roots, path)
			}
			record(path, directory)
		}
	}
	return added, writable, notes, nil
}

// sharedElsewhere refuses the Git metadata of a linked worktree whose
// repository is not the session's own. Its common directory holds the hooks
// and the configuration every checkout of that repository runs — main's
// included, outside any sandbox — so writing it is not writing one checkout.
// The session's own repository was writable before the grant.
func sharedElsewhere(directory string, metadata []string, cwd string) error {
	if len(metadata) < 2 {
		return nil
	}
	common := metadata[len(metadata)-1]
	if own, err := gitMetadataDirectories(cwd); err == nil && own[len(own)-1] == common {
		return nil
	}
	return fmt.Errorf("%s is a worktree of the repository at %s, whose Git metadata — hooks and configuration included — every checkout of it shares; --grant-git does not give that to a session working elsewhere: grant the directory without it, or have the owner give the access", directory, common)
}

// settledGrants lists the live entries whose tasks are settled, leaving out a
// directory another live task still holds.
func (s *serverSession) settledGrants(entries []grant.Entry) []int {
	if s.mailbox == "" {
		return nil
	}
	known := map[string]bool{}
	settled := func(id string) bool {
		if value, ok := known[id]; ok {
			return value
		}
		known[id] = inbox.Settled(s.mailbox, s.name, id)
		return known[id]
	}
	held := map[string]bool{}
	for _, entry := range entries {
		if entry.Live() && !settled(entry.Message) {
			held[entry.Path] = true
		}
	}
	var indexes []int
	for index, entry := range entries {
		if entry.Live() && settled(entry.Message) && !held[entry.Path] {
			indexes = append(indexes, index)
		}
	}
	return indexes
}

// grantJournal is a copy of what this run granted. The wrapper's memory is
// the record rewake takes grants back by: the file beside the mailbox is one
// a sandboxed worker could write, or empty, and it is kept only for `rewake
// list` to show.
func (s *serverSession) grantJournal() []grant.Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.grants)
}

// saveJournal records what a notice the harness took granted and took back,
// in memory first. A copy on disk that cannot be written costs only the
// listing, and the detail says so.
func (s *serverSession) saveJournal(entries []grant.Entry) string {
	if entries == nil {
		return ""
	}
	s.mu.Lock()
	s.grants = slices.Clone(entries)
	s.mu.Unlock()
	if s.mailbox == "" {
		return "the grant is not shown by rewake list: this session has no state directory"
	}
	if err := grant.Save(s.mailbox, s.name, s.epoch, entries); err != nil {
		return "the grant is not shown by rewake list: " + err.Error()
	}
	return ""
}
