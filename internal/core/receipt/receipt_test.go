package receipt

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
)

func journalDir(t *testing.T) string {
	t.Helper()
	return buildtest.Dir(t)
}

var key = Key{Epoch: "run1", Digest: "d"}

// A shell call has no turn, so the same words are two operations.
func TestTheSameWordsAreTwoOperations(t *testing.T) {
	dir := journalDir(t)
	first, err := Begin(dir, "api", key, Record{})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := Begin(dir, "api", key, Record{})
	if err != nil || second.Token == first.Token {
		t.Fatalf("second joined the first: %v", err)
	}
}

// A token names a record of its own run and nothing else.
func TestATokenBelongsToItsRun(t *testing.T) {
	dir := journalDir(t)
	record, _ := Begin(dir, "api", key, Record{})
	if _, err := Load(dir, "api", "run2", record.Token); !errors.Is(err, ErrUnknown) {
		t.Fatalf("another run loads it: %v", err)
	}
	for _, token := range []string{"../x", "", record.Token + "0", "ZZZZZZZZZZZZZZZZZZZZZZZZ"} {
		if _, err := Load(dir, "api", "run1", token); !errors.Is(err, ErrUnknown) {
			t.Errorf("%q: %v", token, err)
		}
	}
	if _, err := Path(dir, "api", "../run"); err == nil {
		t.Error("a run name that is a path is taken")
	}
}

// A second holder waits and gives up as busy; the first one's release lets
// the next in.
func TestTheLockHoldsOneCallAtATime(t *testing.T) {
	dir := journalDir(t)
	record, _ := Begin(dir, "api", key, Record{})
	release, err := Lock(context.Background(), dir, "api", "run1", record.Token)
	if err != nil {
		t.Fatal(err)
	}
	wait, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := Lock(wait, dir, "api", "run1", record.Token); !errors.Is(err, ErrBusy) {
		t.Fatalf("a second holder got %v", err)
	}
	release()
	again, err := Lock(context.Background(), dir, "api", "run1", record.Token)
	if err != nil {
		t.Fatalf("after release: %v", err)
	}
	again()
}

// Unresolved finds the operations of these words whose effect is not settled,
// whatever their age and transport: the open ones, and a finished one whose
// effect is uncertain. A journal it cannot read is an error, not an empty
// answer.
func TestUnresolvedFindsOpenOperations(t *testing.T) {
	dir := journalDir(t)
	old, _ := Begin(dir, "api", key, Record{Transport: "test"})
	old.Created = time.Now().Add(-48 * time.Hour)
	_ = Save(dir, "api", old)
	shell, _ := Begin(dir, "api", key, Record{Transport: Shell})
	finished, _ := Begin(dir, "api", key, Record{Transport: Shell})
	finished.Phase = Done
	_ = Save(dir, "api", finished)
	uncertain, _ := Begin(dir, "api", key, Record{Transport: Shell})
	uncertain.Phase, uncertain.Uncertain = Done, true
	_ = Save(dir, "api", uncertain)
	records, err := Unresolved(dir, "api", "run1", "d")
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, record := range records {
		found[record.Token] = true
	}
	if len(found) != 3 || !found[old.Token] || !found[shell.Token] || !found[uncertain.Token] {
		t.Fatalf("found %v", found)
	}
	journal, _ := Path(dir, "api", "run1")
	if err := os.WriteFile(recordPath(journal, finished.Token), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if records, err := Unresolved(dir, "api", "run1", "d"); err == nil {
		t.Fatalf("an unreadable record answered %d operations and no error", len(records))
	}
}

// The sweep drops finished records and other runs' journals once old, and
// keeps what a recovery still reads: an open record, and one whose effect is
// unknown.
func TestSweepKeepsWhatARecoveryReads(t *testing.T) {
	dir := journalDir(t)
	done, _ := Begin(dir, "api", key, Record{})
	done.Phase = Done
	_ = Save(dir, "api", done)
	open, _ := Begin(dir, "api", Key{Epoch: "run1", Digest: "open"}, Record{})
	other, _ := Begin(dir, "api", Key{Epoch: "run0", Digest: "d"}, Record{})
	uncertain, _ := Begin(dir, "api", Key{Epoch: "run1", Digest: "unknown"}, Record{})
	uncertain.Phase, uncertain.Uncertain = Done, true
	_ = Save(dir, "api", uncertain)
	Sweep(dir, "api", "run1", time.Now().Add(-time.Hour))
	for _, token := range []string{done.Token, open.Token} {
		if _, err := Load(dir, "api", "run1", token); err != nil {
			t.Fatalf("a fresh record went: %v", err)
		}
	}
	Sweep(dir, "api", "run1", time.Now().Add(time.Hour))
	if _, err := Load(dir, "api", "run1", done.Token); !errors.Is(err, ErrUnknown) {
		t.Errorf("an old finished record stays: %v", err)
	}
	if _, err := Load(dir, "api", "run1", open.Token); err != nil {
		t.Errorf("an open record went: %v", err)
	}
	if _, err := Load(dir, "api", "run1", uncertain.Token); err != nil {
		t.Errorf("a record whose effect is unknown went: %v", err)
	}
	if _, err := Load(dir, "api", "run0", other.Token); !errors.Is(err, ErrUnknown) {
		t.Errorf("another run's old journal stays: %v", err)
	}
}

// The sweep passes over a record whose lock another call holds, and removing
// a lock file never lets two calls hold one operation: a call that got the
// lock on a file removed under it takes the new one instead.
func TestTheSweepRespectsAHeldLock(t *testing.T) {
	dir := journalDir(t)
	record, err := Begin(dir, "api", key, Record{})
	if err != nil {
		t.Fatal(err)
	}
	record.Phase = Done
	if err := Save(dir, "api", record); err != nil {
		t.Fatal(err)
	}
	release, err := Lock(context.Background(), dir, "api", "run1", record.Token)
	if err != nil {
		t.Fatal(err)
	}
	Sweep(dir, "api", "run1", time.Now().Add(time.Hour))
	if _, err := Load(dir, "api", "run1", record.Token); err != nil {
		t.Fatalf("the sweep removed a held record: %v", err)
	}
	busy, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if second, err := Lock(busy, dir, "api", "run1", record.Token); err == nil {
		second()
		t.Fatal("a second call took a held lock")
	}

	// A waiter on the file, which its holder then removes.
	waited := make(chan func())
	go func() {
		got, err := Lock(context.Background(), dir, "api", "run1", record.Token)
		if err != nil {
			close(waited)
			return
		}
		waited <- got
	}()
	time.Sleep(50 * time.Millisecond)
	journal, _ := Path(dir, "api", "run1")
	_ = os.Remove(lockPath(journal, record.Token))
	release()
	first, ok := <-waited
	if !ok {
		t.Fatal("the waiter failed")
	}
	defer first()
	busy, cancel = context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if third, err := Lock(busy, dir, "api", "run1", record.Token); err == nil {
		third()
		t.Fatal("two calls hold one operation after its lock file was removed")
	}
}
