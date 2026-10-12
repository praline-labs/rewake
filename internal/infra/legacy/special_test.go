package legacy_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// A file of the 1.x tree or of the clearing that is not a regular file — a
// FIFO, above all — is refused without waiting on it: opening a FIFO waits
// for a writer, and the clearing would hold the clear lock and every room
// lock it took until one came. A record, a lock and the clear lock refuse,
// naming the path; a letter the counter cannot read is an unknown count.
func TestASpecialFileIsRefusedWithoutWaiting(t *testing.T) {
	cases := []struct {
		name, at, want string
	}{
		{"a session record", "rooms/old/sessions/web.json", "web.json cannot be checked: it is not a regular file"},
		{"a name lock", "rooms/old/sessions/.web.lock", ".web.lock is not a regular file"},
		{"a mailbox lock", "rooms/old/inbox/web/.lock", "inbox/web/.lock is not a regular file"},
		{"a launch lock", "rooms/old/.launch.lock", ".launch.lock is not a regular file"},
		{"a launch lock of a left candidate", ".rooms-1x-candidate-" + leftID + "/old/.launch.lock", ".launch.lock is not a regular file"},
		{"the clear lock", "v2/.clear.lock", ".clear.lock is not a regular file"},
		{"a letter", "rooms/old/inbox/web/m2.json", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newScene(t)
			tree, _, found := strings.Cut(c.at, "/old/")
			if !found {
				tree = "rooms"
			}
			s.letter(s.room(filepath.Join(s.base, tree), "old"), "web", "m1")
			fifo := filepath.Join(s.base, c.at)
			if err := os.MkdirAll(filepath.Dir(fifo), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := syscall.Mkfifo(fifo, 0o600); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- s.launch() }()
			var err error
			select {
			case err = <-done:
			case <-time.After(3 * time.Second):
				// Let every blocked open go, so no worker outlives the test.
				for released := false; !released; {
					releaseFIFOs(s.base)
					select {
					case <-done:
						released = true
					case <-time.After(20 * time.Millisecond):
					}
				}
				t.Fatalf("the clearing waited on %s for a writer", c.at)
			}
			if c.want == "" {
				if err != nil || !strings.Contains(s.said.String(), "unknown unread letters") {
					t.Fatalf("a letter that is no file: %v, said %q", err, s.said.String())
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("the launch: %v", err)
			}
			if !exists(filepath.Join(s.base, tree, "old", "inbox", "web", "m1.json")) {
				t.Fatal("a refused launch lost a letter of 1.x")
			}
		})
	}
}

// releaseFIFOs opens for a moment the write end of every FIFO in base,
// wherever a rename carried it, letting a reader waiting on one go.
func releaseFIFOs(base string) {
	_ = filepath.WalkDir(base, func(path string, entry fs.DirEntry, err error) error {
		if err == nil && entry.Type()&fs.ModeNamedPipe != 0 {
			if writer, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
				_, _ = writer.Write([]byte("x"))
				_ = writer.Close()
			}
		}
		return nil
	})
}

// The counts a validated tree's clearer recorded are read as every other file
// of the clearing: anything but a regular file in their place — a FIFO there
// from the start, or a FIFO or a link put there between the check and the open
// — is unknown counts, read without waiting and never followed; the tree it
// counted is deleted all the same.
func TestASpecialSummaryIsUnknownWithoutWaiting(t *testing.T) {
	const unknownAll = "unknown rooms, unknown unread letters, unknown undelivered reports"
	const counts = `{"rooms":7,"unread":7,"undelivered":7}`
	cases := []struct {
		name  string
		setup func(s *scene, summary string, hold func(*os.File))
	}{
		{"a FIFO", func(s *scene, summary string, _ func(*os.File)) {
			mustFIFO(s.t, summary)
		}},
		{"a FIFO put in its place after its check", func(s *scene, summary string, _ func(*os.File)) {
			mustWrite(s.t, summary, counts)
			atSummaryRead(s, summary, func() { mustFIFO(s.t, summary) })
		}},
		{"a FIFO holding counts put in its place after its check", func(s *scene, summary string, hold func(*os.File)) {
			mustWrite(s.t, summary, counts)
			atSummaryRead(s, summary, func() {
				mustFIFO(s.t, summary)
				// Both ends held by the test: the counts wait in the pipe,
				// for a reader that takes the FIFO for the record.
				writer, err := os.OpenFile(summary, os.O_RDWR|syscall.O_NONBLOCK, 0)
				if err != nil {
					s.t.Fatal(err)
				}
				hold(writer)
				if _, err := writer.WriteString(counts); err != nil {
					s.t.Fatal(err)
				}
			})
		}},
		{"a link put in its place after its check", func(s *scene, summary string, _ func(*os.File)) {
			mustWrite(s.t, summary, counts)
			elsewhere := filepath.Join(s.t.TempDir(), "counts.json")
			mustWrite(s.t, elsewhere, counts)
			atSummaryRead(s, summary, func() { mustSymlink(s.t, elsewhere, summary) })
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newScene(t)
			s.letter(s.room(s.validated(leftID), "old"), "web", "m1")
			var held []*os.File
			letGo := func() {
				for _, file := range held {
					_ = file.Close()
				}
				held = nil
			}
			t.Cleanup(letGo)
			c.setup(s, s.summary(leftID), func(file *os.File) { held = append(held, file) })
			done := make(chan error, 1)
			go func() { done <- s.launch() }()
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("a special summary refused the clearing: %v", err)
				}
			case <-time.After(3 * time.Second):
				letGo()
				for released := false; !released; {
					releaseFIFOs(s.base)
					select {
					case <-done:
						released = true
					case <-time.After(20 * time.Millisecond):
					}
				}
				t.Fatal("the clearing waited on its recorded counts for a writer")
			}
			if want := "rewake: removed the state of rewake 1.x (" + unknownAll + ").\n"; s.said.String() != want {
				t.Fatalf("the launch said %q, want %q", s.said.String(), want)
			}
			s.wantTrees()
		})
	}
}

// A mailbox put back as a link or a FIFO between its check and its listing is
// neither followed nor waited on: its letters are an unknown count, the
// letters a link names are left where they are, and the tree is deleted.
func TestAMailboxSwappedAfterItsCheckIsNeitherFollowedNorWaitedOn(t *testing.T) {
	for _, kind := range []string{"a link", "a FIFO"} {
		t.Run(kind, func(t *testing.T) {
			s := newScene(t)
			outside := t.TempDir()
			s.letter(s.room(outside, "x"), "web", "foreign")
			foreign := filepath.Join(outside, "x", "inbox", "web")
			s.letter(s.room(s.rooms(), "old"), "web", "m1")
			setAside, swapped := false, false
			faults(t, func(op, path string) error {
				switch {
				case op == state.OpStep && path == "legacy set aside":
					setAside = true
				case setAside && !swapped && op == state.OpRead && strings.HasSuffix(path, "/old/inbox/web"):
					swapped = true
					if err := os.Rename(path, filepath.Join(outside, "moved")); err != nil {
						t.Fatal(err)
					}
					if kind == "a link" {
						mustSymlink(t, foreign, path)
					} else {
						mustFIFO(t, path)
					}
				}
				return nil
			})
			done := make(chan error, 1)
			go func() { done <- s.launch() }()
			select {
			case err := <-done:
				if err != nil || !swapped {
					t.Fatalf("swapped %v: %v", swapped, err)
				}
			case <-time.After(3 * time.Second):
				for released := false; !released; {
					releaseFIFOs(s.base)
					select {
					case <-done:
						released = true
					case <-time.After(20 * time.Millisecond):
					}
				}
				t.Fatal("the clearing waited on a mailbox for a writer")
			}
			if !strings.Contains(s.said.String(), "unknown unread letters") {
				t.Fatalf("a mailbox swapped after its check was counted: %q", s.said.String())
			}
			if !exists(filepath.Join(foreign, "foreign.json")) {
				t.Fatal("the clearing removed what a link named")
			}
			s.wantTrees()
		})
	}
}

// atSummaryRead runs swap once, when the clearing asks to read summary: after
// the check before the open, before the open itself.
func atSummaryRead(s *scene, summary string, swap func()) {
	swapped := false
	faults(s.t, func(op, path string) error {
		if op == state.OpRead && path == summary && !swapped {
			swapped = true
			if err := os.Remove(path); err != nil {
				s.t.Fatal(err)
			}
			swap()
		}
		return nil
	})
}

func mustFIFO(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
}
