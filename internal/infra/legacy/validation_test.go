package legacy_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/legacy"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// A tree is deleted only after a validation run after its rename: a wrapper
// that appeared in the set-aside tree after admission refuses the launch and
// puts the tree back; a candidate a clearer left is validated on its own
// contents; and in a clean clearing nothing is removed before the rename to
// validated and its flush.
func TestCandidateRequiresPostRenameValidation(t *testing.T) {
	t.Run("a wrapper after admission", func(t *testing.T) {
		s := newScene(t)
		s.letter(s.room(s.rooms(), "old"), "web", "m1")
		onStep(t, "legacy set aside", func() { s.liveSession(filepath.Join(s.only("candidate"), "old"), "late") })
		s.refused("a rewake 1.x session runs: late in room old")
		s.wantTrees("rooms")
		if !exists(filepath.Join(s.rooms(), "old", "inbox", "web", "m1.json")) {
			t.Fatal("the tree put back lost its letter")
		}
	})
	t.Run("a candidate left alive", func(t *testing.T) {
		s := newScene(t)
		room := s.room(s.candidate(leftID), "old")
		onStep(t, "legacy admitted", func() { s.liveSession(room, "late") })
		s.refused("a rewake 1.x session runs: late in room old")
		s.wantTrees("rooms")
	})
	t.Run("a candidate left quiet", func(t *testing.T) {
		s := newScene(t)
		s.room(s.candidate(leftID), "old")
		s.went()
		s.wantTrees()
	})
	t.Run("the order of a clean clearing", func(t *testing.T) {
		s := newScene(t)
		for _, room := range []string{"a", "b"} {
			s.letter(s.room(s.rooms(), room), "web", "m-"+room)
		}
		var mu sync.Mutex
		var log []string
		faults(t, func(op, path string) error {
			mu.Lock()
			defer mu.Unlock()
			if op != state.OpRead && op != state.OpNote {
				log = append(log, op+" "+strings.TrimPrefix(path, s.base))
			}
			return nil
		})
		s.went()
		s.wantTrees()
		validated, synced := -1, -1
		for i, entry := range log {
			switch {
			case strings.HasPrefix(entry, "rename /.rooms-1x-validated-"):
				validated = i
			case validated >= 0 && synced < 0 && entry == "sync ":
				synced = i
			case strings.HasPrefix(entry, "remove /.rooms-1x-") && (validated < 0 || synced < 0):
				t.Fatalf("removed before the validated rename was flushed: %v", log)
			}
		}
		if validated < 0 || synced < 0 || !contains(log[:validated], "step legacy validated") || !contains(log[:validated], "write /v2/.cleared-1x-") {
			t.Fatalf("no validation, record and flushed rename before the deletion: %v", log)
		}
	})
}

// contains says whether an entry of log starts with prefix.
func contains(log []string, prefix string) bool {
	for _, entry := range log {
		if strings.HasPrefix(entry, prefix) {
			return true
		}
	}
	return false
}

// Validation of the launch's own candidate checks the launch locks admission
// took at their renamed paths rather than locking them again — which would
// contend with itself — and refuses a lock file replaced meanwhile or a
// room gone from the tree; a candidate another clearer left gets fresh locks,
// taken without waiting.
func TestValidationReusesOwnedLocks(t *testing.T) {
	t.Run("its own locks", func(t *testing.T) {
		s := newScene(t)
		for _, room := range []string{"a", "b", "c"} {
			s.room(s.rooms(), room)
		}
		s.went()
		s.wantTrees()
	})
	t.Run("a lock file replaced", func(t *testing.T) {
		s := newScene(t)
		s.room(s.rooms(), "old")
		onStep(t, "legacy set aside", func() {
			lock := filepath.Join(s.only("candidate"), "old", ".launch.lock")
			if err := os.Remove(lock); err != nil {
				t.Fatal(err)
			}
			mustWrite(t, lock, "")
		})
		s.refused(".launch.lock cannot be checked", "is not the file this launch locked")
		s.wantTrees("rooms")
	})
	t.Run("a room that went", func(t *testing.T) {
		s := newScene(t)
		s.room(s.rooms(), "a")
		s.room(s.rooms(), "b")
		onStep(t, "legacy set aside", func() {
			if err := os.RemoveAll(filepath.Join(s.only("candidate"), "b")); err != nil {
				t.Fatal(err)
			}
		})
		s.refused("cannot be checked: it went while the tree was set aside")
		s.wantTrees("rooms")
	})
	t.Run("a left candidate's lock", func(t *testing.T) {
		s := newScene(t)
		legacy.SetWaits(t, time.Second, 2*time.Second)
		room := s.room(s.candidate(leftID), "old")
		var admitted time.Time
		onStep(t, "legacy admitted", func() {
			admitted = time.Now()
			holdLock(t, filepath.Join(room, ".launch.lock"))
		})
		s.refused("a rewake 1.x command runs in room old", ".launch.lock")
		if waited := time.Since(admitted); waited > time.Second {
			t.Fatalf("validation waited %s for a left candidate's lock", waited)
		}
		s.wantTrees("rooms")
	})
}

// A rooms/ recreated after the tree was set aside fails nothing by being
// there: bare, or with a bare room, the clearing goes on and leaves it. Live
// state in it — a wrapper, a lock held, a record that cannot be read — is
// evidence against the admission, and the launch refuses, the candidate
// kept beside it.
func TestRecreatedRoomsRevokesAdmissionOnlyOnEvidence(t *testing.T) {
	cases := []struct {
		name string
		make func(s *scene)
		want string
	}{
		{"bare", func(s *scene) { mustMkdir(s.t, s.rooms()) }, ""},
		{"with a bare room", func(s *scene) { s.room(s.rooms(), "new") }, ""},
		{"with a live wrapper", func(s *scene) { s.liveSession(s.room(s.rooms(), "new"), "web") }, "a rewake 1.x session runs: web in room new"},
		{"with a lock held", func(s *scene) { holdLock(s.t, filepath.Join(s.room(s.rooms(), "new"), "sessions", ".web.lock")) }, "sessions/.web.lock"},
		{"with a record that cannot be read", func(s *scene) { mustWrite(s.t, filepath.Join(s.room(s.rooms(), "new"), "sessions", "web.json"), "x") }, "web.json cannot be checked"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newScene(t)
			s.room(s.rooms(), "old")
			onStep(t, "legacy set aside", func() { c.make(s) })
			if c.want == "" {
				s.went()
				s.wantTrees("rooms")
				return
			}
			s.refused(c.want)
			s.wantTrees("candidate", "rooms")
			if !exists(filepath.Join(s.only("candidate"), "old")) {
				t.Fatal("the candidate kept lost its room")
			}
		})
	}
}

// A candidate that fails validation goes back to rooms/ only by a rename that
// never replaces: a rooms/ there before the restore, or one that appears
// between the look and the rename, keeps the candidate where it is.
func TestCandidateRestoreNeverReplacesRooms(t *testing.T) {
	for _, when := range []string{"before the restore", "at the rename"} {
		t.Run(when, func(t *testing.T) {
			s := newScene(t)
			s.room(s.rooms(), "old")
			faults(t, func(op, path string) error {
				switch {
				case op == state.OpStep && path == "legacy set aside":
					s.liveSession(filepath.Join(s.only("candidate"), "old"), "late")
					if when == "before the restore" {
						mustMkdir(t, s.rooms())
					}
				case op == state.OpRename && path == s.rooms() && when == "at the rename":
					mustMkdir(t, s.rooms())
				}
				return nil
			})
			err := s.launch()
			if err == nil || !strings.Contains(err.Error(), "late in room old") || strings.Contains(err.Error(), "could not put") {
				t.Fatalf("the launch: %v", err)
			}
			s.wantTrees("candidate", "rooms")
			if entries, _ := os.ReadDir(s.rooms()); len(entries) != 0 {
				t.Fatalf("the restore wrote into the rooms/ that appeared: %v", entries)
			}
		})
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

// A candidate's name is drawn, never assumed free: a name another clearer's
// tree holds is passed by for the next draw, and both trees go through whole.
// The draws differ within a process, so a pid and a start recurring after a
// boot do not make a name recur.
func TestASetAsideDrawsAgainOnATakenName(t *testing.T) {
	s := newScene(t)
	s.letter(s.room(s.rooms(), "old"), "web", "m1")
	s.letter(s.room(s.candidate(leftID), "left"), "web", "m-left")
	legacy.SetDraws(t, leftID, "5.6.0123456789abcdef")
	s.went()
	s.wantTrees()
	if want := "rewake: removed the state of rewake 1.x (2 rooms, 2 unread letters, 0 undelivered reports).\n"; s.said.String() != want {
		t.Fatalf("it said %q, want %q", s.said.String(), want)
	}
	if first, second := legacy.NewTreeID(), legacy.NewTreeID(); first == second || !state.ValidTreeID(first) {
		t.Fatalf("two draws %q and %q", first, second)
	}
}
