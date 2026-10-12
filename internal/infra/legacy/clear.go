// Package legacy clears the state rewake 1.x left beside a 2.0 root, at the
// first launch that finds it (docs/v2/stage3-upgrade.md): it refuses while a
// 1.x session runs, a 1.x lock is held or a check cannot be made, and deletes
// only a tree validated after it was set aside.
package legacy

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/infra/buildtime"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// clearWait bounds the wait for the clear lock (T9), and roomWait the wait
// for each 1.x room's launch lock at admission. There is no total budget: a
// tree of n rooms may take n times roomWait, and the launch says so when that
// passes clearWait. The workflow suite builds them shorter (package buildtime).
var (
	clearWait = buildtime.Duration("builtClearWait", builtClearWait, 10*time.Second)
	roomWait  = buildtime.Duration("builtRoomWait", builtRoomWait, 2*time.Second)
)

var builtClearWait, builtRoomWait string

// Clear runs the clearing over the base of cleanup and answers nil when the
// launch may go on. A *Refusal names what keeps it out. It writes one line to
// say on what it removed, and one before a long scan.
func Clear(cleanup *state.Cleanup, say io.Writer) error {
	return run{cleanup: cleanup, reader: procReader, say: say}.clear()
}

// procReader is the /proc the clearing judges wrappers by; tests point it at
// a tree of their own.
var procReader = proc.Default

type run struct {
	cleanup *state.Cleanup
	reader  proc.Reader
	say     io.Writer
}

func (r run) clear() error {
	trees, err := r.cleanup.Trees()
	if err != nil {
		return &Refusal{failure: err}
	}
	if trees.Empty() {
		return nil
	}
	clearLock, err := r.cleanup.LockClear(clearWait)
	if errors.Is(err, state.ErrClearBusy) {
		return &Refusal{failure: errors.New("another launch is checking the state of rewake 1.x; launch again")}
	}
	if err != nil {
		return &Refusal{failure: err}
	}
	defer clearLock.Close()
	// The first read only chose the slow path; this one, under the lock, is
	// what the clearing acts on.
	if trees, err = r.cleanup.Trees(); err != nil {
		return &Refusal{failure: err}
	}

	held, refusal := r.admit(trees)
	if refusal != nil {
		return refusal
	}
	state.Step("legacy admitted")
	var done summary
	for _, id := range trees.Validated {
		done.finish(r.cleanup, id, r.recorded(id))
	}

	own := ""
	if trees.Rooms {
		id, err := r.setAside()
		if err != nil {
			closeAll(held)
			return &Refusal{failure: err}
		}
		own = id
		state.Step("legacy set aside")
	}
	candidates := trees.Candidates
	if own != "" {
		candidates = append([]string{own}, candidates...)
	}
	refusal = r.validate(candidates, own, held, &done)
	done.say(r.say)
	if refusal != nil {
		return refusal
	}
	return nil
}

// admit runs steps 1–3 over rooms/ and every candidate. The launch locks of
// rooms/ stay held for the clearing; those of the candidates are released.
func (r run) admit(trees state.Trees) (map[string]*state.TreeLock, *Refusal) {
	s := r.scanner()
	r.announce(trees)
	var held map[string]*state.TreeLock
	if trees.Rooms {
		held = s.lockRooms(r.cleanup.Rooms(), roomWait)
	}
	for _, id := range trees.Candidates {
		closeAll(s.lockRooms(r.cleanup.Candidate(id), roomWait))
	}
	if len(s.findings) > 0 {
		closeAll(held)
		return nil, &Refusal{findings: s.findings}
	}
	return held, nil
}

// announce says on stderr that the scan may take long: past the clear bound
// in all, at the per-room bound each.
func (r run) announce(trees state.Trees) {
	n := 0
	for _, tree := range r.treePaths(trees) {
		if entries, err := listDir(tree); err == nil {
			n += len(entries)
		}
	}
	if time.Duration(n)*roomWait > clearWait {
		_, _ = fmt.Fprintf(r.say, "rewake: checking %d rooms of rewake 1.x, up to %s each\n", n, roomWait)
	}
}

func (r run) treePaths(trees state.Trees) []string {
	var paths []string
	if trees.Rooms {
		paths = append(paths, r.cleanup.Rooms())
	}
	for _, id := range trees.Candidates {
		paths = append(paths, r.cleanup.Candidate(id))
	}
	return paths
}

func (r run) scanner() *scanner {
	return &scanner{cleanup: r.cleanup, reader: r.reader, here: r.reader.Namespace()}
}

// setAside renames rooms/ to a candidate under a fresh id, drawing another
// when the name is taken. A failed rename or flush stops the clearing.
func (r run) setAside() (string, error) {
	for range 3 {
		id := drawTreeID(r.reader)
		err := r.cleanup.SetAside(id)
		if errors.Is(err, state.ErrTreeExists) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("could not set the state of rewake 1.x aside, so nothing was removed: %w", err)
		}
		return id, nil
	}
	return "", errors.New("could not set the state of rewake 1.x aside: every name drawn was taken, so nothing was removed")
}

// drawTreeID draws the id of a tree set aside; tests make it collide.
var drawTreeID = newTreeID

// newTreeID is this clearer's pid, its start and a random part, so that it
// cannot recur across boots.
func newTreeID(reader proc.Reader) string {
	start, _ := reader.StartTime(os.Getpid())
	random := make([]byte, 8)
	_, _ = rand.Read(random)
	return fmt.Sprintf("%d.%d.%s", os.Getpid(), start, hex.EncodeToString(random))
}

// validate runs step 6 over every candidate, this launch's own first, and a
// rooms/ recreated meanwhile. A candidate that passes is validated and
// deleted; one that does not stays, back at rooms/ when that name is free.
func (r run) validate(candidates []string, own string, held map[string]*state.TreeLock, done *summary) *Refusal {
	s := r.scanner()
	var recreated map[string]*state.TreeLock
	if _, err := os.Lstat(r.cleanup.Rooms()); err == nil {
		recreated = s.lockRooms(r.cleanup.Rooms(), 0)
	} else if !errors.Is(err, os.ErrNotExist) {
		s.add(finding{kind: unreadable, path: r.cleanup.Rooms(), err: err})
	}
	defer closeAll(recreated)
	outside := s.findings

	refusal := &Refusal{findings: outside}
	for _, id := range candidates {
		tree := r.cleanup.Candidate(id)
		c := r.scanner()
		var locks map[string]*state.TreeLock
		if id == own {
			locks = c.recheck(tree, held)
		} else {
			locks = c.lockRooms(tree, 0)
		}
		if len(c.findings) > 0 || len(outside) > 0 {
			closeAll(locks)
			refusal.findings = append(refusal.findings, c.findings...)
			r.keep(id, refusal)
			continue
		}
		state.Step("legacy validated")
		err := r.markValidated(id, tree)
		closeAll(locks)
		if err != nil {
			refusal.failure = errors.Join(refusal.failure, err)
			continue
		}
		done.finish(r.cleanup, id, r.recorded(id))
	}
	if len(refusal.findings) > 0 || refusal.failure != nil {
		return refusal
	}
	return nil
}

// markValidated records the counts of a tree that passed and then renames it
// validated, keeping its id: until both are durable nothing of it is deleted.
func (r run) markValidated(id, tree string) error {
	data, err := json.Marshal(count(tree))
	if err != nil {
		return err
	}
	if err := r.cleanup.WriteSummary(id, data); err != nil {
		return fmt.Errorf("could not record what the state of rewake 1.x holds, so nothing was removed: %w", err)
	}
	if err := r.cleanup.MarkValidated(id); err != nil {
		return fmt.Errorf("could not mark the state of rewake 1.x validated, so nothing was removed: %w", err)
	}
	return nil
}

// keep leaves a candidate that did not pass, renaming it back to rooms/ when
// that name is free, so a 1.x process holding a file in it writes where it
// expects; a rooms/ that appeared keeps it where it is.
func (r run) keep(id string, refusal *Refusal) {
	if _, err := os.Lstat(r.cleanup.Rooms()); !errors.Is(err, os.ErrNotExist) {
		return
	}
	if err := r.cleanup.Restore(id); err != nil && !errors.Is(err, state.ErrTreeExists) {
		refusal.failure = errors.Join(refusal.failure, fmt.Errorf("could not put the state of rewake 1.x back: %w", err))
	}
}

// recorded reads the counts written before a tree was validated; missing or
// unreadable, each is unknown — what is left of a tree is not what it held.
func (r run) recorded(id string) Counts {
	unknownCounts := Counts{Rooms: Unknown, Unread: Unknown, Undelivered: Unknown}
	data, err := r.cleanup.ReadSummary(id)
	if err != nil {
		return unknownCounts
	}
	var counts Counts
	if err := json.Unmarshal(data, &counts); err != nil {
		return unknownCounts
	}
	return counts
}

func closeAll(locks map[string]*state.TreeLock) {
	for _, lock := range locks {
		lock.Close()
	}
}

// summary gathers what this invocation removed and what it left to remove.
type summary struct {
	removed  []Counts
	deferred int
}

// finish deletes a validated tree; a deletion that fails is deferred to the
// next launch, which finds the tree validated and its counts recorded.
func (s *summary) finish(cleanup *state.Cleanup, id string, counts Counts) {
	if err := cleanup.Delete(id); err != nil {
		s.deferred++
		return
	}
	s.removed = append(s.removed, counts)
	_ = cleanup.DiscardSummary(id)
}

// say writes the one line of an invocation, if it removed or deferred anything.
func (s summary) say(w io.Writer) {
	var parts []string
	if len(s.removed) > 0 {
		total := Counts{}
		for _, counts := range s.removed {
			total = Counts{Rooms: add(total.Rooms, counts.Rooms), Unread: add(total.Unread, counts.Unread), Undelivered: add(total.Undelivered, counts.Undelivered)}
		}
		parts = append(parts, fmt.Sprintf("removed the state of rewake 1.x (%s, %s, %s)",
			plural(total.Rooms, "room", "rooms"), plural(total.Unread, "unread letter", "unread letters"),
			plural(total.Undelivered, "undelivered report", "undelivered reports")))
	}
	if s.deferred > 0 {
		parts = append(parts, fmt.Sprintf("%s of rewake 1.x could not be deleted yet; the next launch deletes it", plural(s.deferred, "validated tree", "validated trees")))
	}
	if len(parts) > 0 {
		_, _ = fmt.Fprintf(w, "rewake: %s.\n", strings.Join(parts, "; "))
	}
}

func plural(n int, one, many string) string {
	switch n {
	case Unknown:
		return "unknown " + many
	case 1:
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}
