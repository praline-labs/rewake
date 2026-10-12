package legacy_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// failRemoval fails, once, the removal of the room of a validated tree.
func failRemoval(t *testing.T, room string) {
	t.Helper()
	failed := false
	faults(t, func(op, path string) error {
		if op == state.OpRemove && !failed && strings.Contains(path, "/.rooms-1x-validated-") && filepath.Base(path) == room {
			failed = true
			return &os.PathError{Op: op, Path: path, Err: syscall.EIO}
		}
		return nil
	})
}

// A deletion that fails after a validation passed defers only the deletion:
// the launch goes on and says what is left, the tree stays validated, and
// the next launch finishes it without checking it again.
func TestValidatedDeletionFailureDefersOnlyDeletion(t *testing.T) {
	s := newScene(t)
	for _, room := range []string{"a", "b"} {
		s.letter(s.room(s.rooms(), room), "web", "m-"+room)
	}
	failRemoval(t, "b")
	s.went()
	if !strings.Contains(s.said.String(), "1 validated tree of rewake 1.x could not be deleted yet; the next launch deletes it") {
		t.Fatalf("the launch said %q", s.said.String())
	}
	if strings.Contains(s.said.String(), "removed") {
		t.Fatalf("a deferred deletion was reported removed: %q", s.said.String())
	}
	s.wantTrees("validated")
	state.Fault = nil
	s.said.Reset()
	// A wrapper registered in a validated tree is no evidence: the tree is
	// out of every 1.x path, and it is never checked again.
	s.liveSession(filepath.Join(s.only("validated"), "b"), "web")
	s.went()
	s.wantTrees()
	if got := s.said.String(); got != "rewake: removed the state of rewake 1.x (2 rooms, 2 unread letters, 0 undelivered reports).\n" {
		t.Fatalf("the finishing launch said %q", got)
	}
}

// The counts are recorded under v2/ before the deletion, so a launch
// finishing a partial deletion reports what the tree held, not what is left;
// counts that were never recorded or cannot be read are unknown, and so is
// each count the record leaves absent or null or that is no whole number
// from zero: nobody recorded it, and it is not zero.
func TestLegacySummarySurvivesPartialDeletion(t *testing.T) {
	t.Run("recorded", func(t *testing.T) {
		s := newScene(t)
		for _, room := range []string{"a", "b", "c"} {
			s.letter(s.room(s.rooms(), room), "web", "m-"+room)
		}
		failRemoval(t, "b")
		s.went()
		tree := s.only("validated")
		if exists(filepath.Join(tree, "a")) || !exists(filepath.Join(tree, "b")) {
			t.Fatal("the deletion did not stop part way")
		}
		id := strings.TrimPrefix(filepath.Base(tree), ".rooms-1x-validated-")
		if !exists(s.summary(id)) {
			t.Fatal("no counts recorded for the tree")
		}
		state.Fault = nil
		s.said.Reset()
		s.went()
		if got := s.said.String(); got != "rewake: removed the state of rewake 1.x (3 rooms, 3 unread letters, 0 undelivered reports).\n" {
			t.Fatalf("the finishing launch said %q", got)
		}
		if exists(s.summary(id)) {
			t.Fatal("the counts outlived their tree")
		}
	})
	const unknownAll = "unknown rooms, unknown unread letters, unknown undelivered reports"
	cases := []struct{ name, summary, says string }{
		{"never recorded", "", unknownAll},
		{"unreadable", "{", unknownAll},
		{"an empty object", "{}", unknownAll},
		{"null", "null", unknownAll},
		{"a count absent", `{"rooms":2,"unread":1}`, "2 rooms, 1 unread letter, unknown undelivered reports"},
		{"a count null", `{"rooms":null,"unread":1,"undelivered":0}`, "unknown rooms, 1 unread letter, 0 undelivered reports"},
		{"counts no whole number from zero", `{"rooms":-5,"unread":1.5,"undelivered":"1"}`, unknownAll},
		{"counts recorded unknown", `{"rooms":2,"unread":-1,"undelivered":-1}`, "2 rooms, unknown unread letters, unknown undelivered reports"},
		{"zeros recorded", `{"rooms":0,"unread":0,"undelivered":0}`, "0 rooms, 0 unread letters, 0 undelivered reports"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newScene(t)
			s.room(s.validated(leftID), "a")
			if c.summary != "" {
				mustWrite(t, s.summary(leftID), c.summary)
			}
			s.went()
			if got, want := s.said.String(), "rewake: removed the state of rewake 1.x ("+c.says+").\n"; got != want {
				t.Fatalf("the launch said %q, want %q", got, want)
			}
		})
	}
}

// Nothing a clearing does follows a link out of the base: a tree's name that
// is a link refuses, a link where a room, a mailbox or a record belongs
// refuses, and a link inside a tree that passes is removed, never what it
// names. What lies beside the trees stays as it was.
func TestLegacyClearNeverFollowsForeignPaths(t *testing.T) {
	cases := []struct {
		name  string
		setup func(s *scene, outside string)
		want  string
	}{
		{"rooms/ a link", func(s *scene, outside string) { mustSymlink(s.t, outside, s.rooms()) }, "rooms is not a directory"},
		{"a candidate a link", func(s *scene, outside string) { mustSymlink(s.t, outside, s.candidate(leftID)) }, "is not a directory"},
		{"a validated tree a link", func(s *scene, outside string) { mustSymlink(s.t, outside, s.validated(leftID)) }, "is not a directory"},
		{"a mailbox a link", func(s *scene, outside string) {
			mustSymlink(s.t, outside, filepath.Join(s.room(s.rooms(), "old"), "inbox", "web"))
		}, "inbox/web cannot be checked"},
		{"a record a link", func(s *scene, outside string) {
			room := s.room(s.rooms(), "old")
			record := s.liveSession(s.room(outside, "x"), "web")
			mustSymlink(s.t, record, filepath.Join(room, "sessions", "web.json"))
		}, "web.json cannot be checked"},
		{"a record swapped for a link after its check", func(s *scene, outside string) {
			// The record names a wrapper that ended, so admission passes; at
			// validation, between its check and its open, a link to another
			// record of an ended wrapper takes its place.
			pid, start, namespace := endedPid(s.t)
			s.session(s.room(s.rooms(), "old"), "web", pid, start, namespace)
			ended := s.session(s.room(outside, "x"), "web", pid, start, namespace)
			setAside := false
			faults(s.t, func(op, path string) error {
				switch {
				case op == state.OpStep && path == "legacy set aside":
					setAside = true
				case setAside && op == state.OpRead && strings.HasSuffix(path, "/old/sessions/web.json"):
					setAside = false
					if err := os.Remove(path); err != nil {
						s.t.Fatal(err)
					}
					mustSymlink(s.t, ended, path)
				}
				return nil
			})
		}, "web.json cannot be checked"},
		{"a link inside a tree that passes", func(s *scene, outside string) {
			mustSymlink(s.t, outside, filepath.Join(s.room(s.rooms(), "old"), "sock", "elsewhere"))
		}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newScene(t)
			outside := t.TempDir()
			s.room(outside, "foreign")
			s.letter(filepath.Join(outside, "foreign"), "web", "kept")
			c.setup(s, outside)
			before := s.unrelated()
			if c.want == "" {
				s.went()
				s.wantTrees()
			} else {
				s.refused(c.want)
			}
			if !exists(filepath.Join(outside, "foreign", "inbox", "web", "kept.json")) {
				t.Fatal("the clearing removed what a link named")
			}
			if s.unrelated() != before {
				t.Fatal("the clearing changed what lies beside the trees")
			}
		})
	}
}

// endedPid is a process that ran and was reaped: its pid, start and
// namespace, which /proc shows gone.
func endedPid(t *testing.T) (int, uint64, string) {
	t.Helper()
	child := exec.Command("/bin/true")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	start, err := proc.StartTime(child.Process.Pid)
	_ = child.Wait()
	if err != nil {
		t.Fatal(err)
	}
	return child.Process.Pid, start, proc.Namespace()
}

func mustSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}
