package legacy_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
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
