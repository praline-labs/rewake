package legacy

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// findingKind is what a check of a 1.x tree found that keeps a launch out.
type findingKind int

const (
	// live is a wrapper /proc shows running.
	live findingKind = iota
	// busy is a 1.x lock not taken: a 1.x command holds it.
	busy
	// unreadable is a check that could not be made; it is never absence.
	unreadable
)

// finding is one reason to refuse, with what names it to the person.
type finding struct {
	kind    findingKind
	room    string
	session string
	path    string
	err     error
}

// scanner runs the checks of admission and validation (steps 1–3 and 6) over
// a tree of 1.x — rooms/ or a candidate — and gathers what it finds.
type scanner struct {
	cleanup  *state.Cleanup
	reader   proc.Reader
	here     string
	findings []finding
}

func (s *scanner) add(f finding) { s.findings = append(s.findings, f) }

// rooms lists the rooms of a tree; anything in it that is not a room of its
// own — a link, a file — is a check that cannot be made.
func (s *scanner) rooms(tree string) []string {
	entries, err := listDir(tree)
	if err != nil {
		s.add(finding{kind: unreadable, path: tree, err: err})
		return nil
	}
	var rooms []string
	for _, entry := range entries {
		path := filepath.Join(tree, entry.Name())
		if !entry.IsDir() {
			s.add(finding{kind: unreadable, path: path, err: errors.New("it is not a room of rewake 1.x")})
			continue
		}
		rooms = append(rooms, entry.Name())
	}
	sort.Strings(rooms)
	return rooms
}

// lockRooms takes the launch lock of every room of tree, each waiting at most
// wait, and checks it. Whatever it took it returns, by room, for the caller
// to hold or close.
func (s *scanner) lockRooms(tree string, wait time.Duration) map[string]*state.TreeLock {
	held := map[string]*state.TreeLock{}
	for _, room := range s.rooms(tree) {
		if lock := s.lockRoom(tree, room, wait); lock != nil {
			held[room] = lock
		}
		s.checkRoom(tree, room)
	}
	return held
}

func (s *scanner) lockRoom(tree, room string, wait time.Duration) *state.TreeLock {
	path := filepath.Join(tree, room)
	lock, err := s.cleanup.LockRoom(path, wait)
	switch {
	case errors.Is(err, state.ErrLegacyBusy):
		s.add(finding{kind: busy, room: room, path: filepath.Join(path, ".launch.lock")})
		return nil
	case err != nil:
		s.add(finding{kind: unreadable, room: room, path: path, err: err})
		return nil
	}
	return lock
}

// recheck validates this launch's own candidate with the launch locks its
// admission took in rooms/: each must be the file now at the renamed path,
// since locking it again would contend with itself. A room that appeared in
// the tree after admission gets a fresh lock taken without waiting; one that
// went is a change nothing explains.
func (s *scanner) recheck(tree string, held map[string]*state.TreeLock) map[string]*state.TreeLock {
	rooms := s.rooms(tree)
	seen := map[string]bool{}
	for _, room := range rooms {
		seen[room] = true
		lock, ok := held[room]
		if !ok {
			if fresh := s.lockRoom(tree, room, 0); fresh != nil {
				held[room] = fresh
			}
		} else if same, err := lock.At(filepath.Join(tree, room, ".launch.lock")); err != nil || !same {
			if err == nil {
				err = errors.New("it is not the file this launch locked before setting the tree aside")
			}
			s.add(finding{kind: unreadable, room: room, path: filepath.Join(tree, room, ".launch.lock"), err: err})
		}
		s.checkRoom(tree, room)
	}
	for room := range held {
		if !seen[room] {
			s.add(finding{kind: unreadable, room: room, path: filepath.Join(tree, room), err: errors.New("it went while the tree was set aside")})
		}
	}
	return held
}

// checkRoom probes the name and mailbox locks of a room and reads its session
// records.
func (s *scanner) checkRoom(tree, room string) {
	dir := filepath.Join(tree, room)
	sessions := filepath.Join(dir, "sessions")
	entries, err := listDir(sessions)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		s.add(finding{kind: unreadable, room: room, path: sessions, err: err})
	}
	for _, entry := range entries {
		name := entry.Name()
		path := filepath.Join(sessions, name)
		switch {
		case strings.HasPrefix(name, ".") && strings.HasSuffix(name, ".lock"):
			s.probe(room, path)
		case !strings.HasPrefix(name, ".") && strings.HasSuffix(name, ".json"):
			s.session(room, strings.TrimSuffix(name, ".json"), path)
		}
	}
	inbox := filepath.Join(dir, "inbox")
	mailboxes, err := listDir(inbox)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		s.add(finding{kind: unreadable, room: room, path: inbox, err: err})
	}
	for _, entry := range mailboxes {
		switch {
		case entry.IsDir():
			s.probe(room, filepath.Join(inbox, entry.Name(), ".lock"))
		case entry.Type()&os.ModeSymlink != 0:
			s.add(finding{kind: unreadable, room: room, path: filepath.Join(inbox, entry.Name()), err: errors.New("it is a link, which is never followed")})
		}
	}
}

// probe tries a name or mailbox lock without waiting and lets it go at once.
func (s *scanner) probe(room, path string) {
	free, err := s.cleanup.TryLock(path)
	switch {
	case err != nil:
		s.add(finding{kind: unreadable, room: room, path: path, err: err})
	case !free:
		s.add(finding{kind: busy, room: room, path: path})
	}
}

func (s *scanner) session(room, name, path string) {
	w, err := readWrapper(path)
	if err != nil {
		s.add(finding{kind: unreadable, room: room, session: name, path: path, err: err})
		return
	}
	switch state, err := alive(s.reader, s.here, w); state {
	case running:
		s.add(finding{kind: live, room: room, session: name, path: path})
	case unknown:
		s.add(finding{kind: unreadable, room: room, session: name, path: path, err: err})
	}
}

// Refusal is a launch kept out by the state of rewake 1.x: what was found,
// each with what names it.
type Refusal struct {
	findings []finding
	failure  error
}

func (r *Refusal) Error() string {
	var lines []string
	if r.failure != nil {
		lines = append(lines, r.failure.Error())
	}
	sawLive := false
	for _, f := range r.findings {
		switch f.kind {
		case live:
			sawLive = true
			lines = append(lines, fmt.Sprintf("a rewake 1.x session runs: %s in room %s", f.session, f.room))
		case busy:
			lines = append(lines, fmt.Sprintf("a rewake 1.x command runs in room %s: it holds %s", f.room, f.path))
		case unreadable:
			lines = append(lines, fmt.Sprintf("%s cannot be checked: %v; you may remove it once no rewake 1.x session runs", f.path, f.err))
		}
	}
	if sawLive {
		lines = append(lines, "rewake 2.0 does not read the mail of rewake 1.x, so end those sessions and launch again")
	}
	return "Rewake: " + strings.Join(lines, ".\n") + "."
}
