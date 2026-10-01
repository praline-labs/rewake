package inbox

import (
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// Rule 6 of docs/mail-bridge-cli.md: a lookup that failed proves nothing is
// absent. These are the lookups on the paths of the mail tool; the mailbox
// that cannot be searched is in claims_test.go.

// unreadable leaves a file in place that cannot be read.
func unreadable(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	// Written, then closed to reads: a file already there keeps its mode
	// through a write.
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
}

// A claim that cannot be read may stand: the letter cannot be taken back.
func TestAClaimThatCannotBeReadHoldsTheLetter(t *testing.T) {
	dir := stateDir(t)
	letter := announcedUnread(t, dir, Message{ID: NewID(), From: "web", FromEpoch: "w1", To: "api", ToEpoch: "e1", Kind: Task, Text: "long", CreatedAt: time.Now()})
	unreadable(t, filepath.Join(claimsPath(dir, "api"), letter.ID), []byte("token"))
	if _, err := withdrawNow(t, dir, letter, nil); !errors.Is(err, ErrReadInProgress) {
		t.Fatalf("withdrawing past a claim that cannot be read = %v", err)
	}
}

// A status that cannot be read may say withdrawn: the letter is not handed
// out as it stands.
func TestAStatusThatCannotBeReadStopsTheLookup(t *testing.T) {
	dir := stateDir(t)
	letter := announcedUnread(t, dir, Message{ID: NewID(), From: "web", To: "api", ToEpoch: "e1", Kind: Task, Text: "taken back", CreatedAt: time.Now()})
	if _, found, err := UnreadCopy(dir, "api", "e1", letter.ID); err != nil || !found {
		t.Fatalf("a readable letter: %v %v", found, err)
	}
	unreadable(t, statusPath(dir, "api", letter.ID), []byte(`{"withdrawn":true}`))
	if held, found, err := UnreadCopy(dir, "api", "e1", letter.ID); err == nil {
		t.Fatalf("looked up past a status it could not read: %v %q", found, held.Text)
	}
	if _, found, err := UnreadCopy(dir, "api", "e1", NewID()); err != nil || found {
		t.Fatalf("a letter that is not there: %v %v", found, err)
	}
}

// A pending mark that cannot be read may be the newer one, and a waiter that
// cannot be read may be owed: neither is taken for none.
func TestAMarkOrAWaiterThatCannotBeReadIsNotNone(t *testing.T) {
	dir := stateDir(t)
	marks, _ := marksPath(dir, "api", "e1")
	if err := os.MkdirAll(filepath.Dir(marks), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(marks, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(marks, 0o700) })
	if _, marked, err := MarkWithin(dir, "api", "e1", 0, 1); err == nil {
		t.Fatalf("marks that cannot be listed answered marked=%v", marked)
	}
	path, _ := awaitingPath(dir, "api", "e1")
	unreadable(t, filepath.Join(path, "web"), []byte("w1"))
	if waiters, err := ReadWaiters(dir, "api", "e1"); err == nil {
		t.Fatalf("an unreadable waiter answered %v", waiters)
	}
	claim := claimsPath(dir, "api")
	if err := os.MkdirAll(claim, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(claim, 0o700) })
	if senders, err := ClaimedOwedBy(dir, "api", "e1"); err == nil {
		t.Fatalf("unlisted claims answered %v", senders)
	}
}

// must is a lookup's answer in a test that set up nothing it could not read.
func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

// failStatusWrites makes writing the status of these messages fail, while
// what is on disk stays readable, until the returned function is called.
func failStatusWrites(t *testing.T, dir, name string, ids ...string) func() {
	t.Helper()
	var blocked atomic.Bool
	blocked.Store(true)
	paths := map[string]bool{}
	for _, id := range ids {
		paths[statusPath(dir, name, id)] = true
	}
	previous := writeStatusFile
	writeStatusFile = func(path string, data []byte) error {
		if blocked.Load() && paths[path] {
			return os.ErrPermission
		}
		return previous(path, data)
	}
	t.Cleanup(func() { writeStatusFile = previous })
	return func() { blocked.Store(false) }
}
