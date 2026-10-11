package receipt

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func journalDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

var scoped = Key{Epoch: "run1", Conversation: "c", Turn: "t", Digest: "d"}

// Calls racing with one key end in one operation: exactly one creates it,
// every other joins, and no second record is left behind.
func TestRacingCallsWithOneKeyJoinOneRecord(t *testing.T) {
	dir := journalDir(t)
	const racers = 16
	tokens := make([]string, racers)
	created := make([]bool, racers)
	var group sync.WaitGroup
	for index := range racers {
		group.Add(1)
		go func() {
			defer group.Done()
			record, joined, err := Begin(dir, "api", scoped, Record{Words: []string{"inbox"}})
			if err != nil {
				t.Errorf("begin: %v", err)
				return
			}
			tokens[index], created[index] = record.Token, !joined
		}()
	}
	group.Wait()
	owners := 0
	for index := range racers {
		if tokens[index] != tokens[0] {
			t.Fatalf("two operations for one key: %s and %s", tokens[0], tokens[index])
		}
		if created[index] {
			owners++
		}
	}
	if owners != 1 {
		t.Fatalf("%d calls created the operation", owners)
	}
	journal, _ := Path(dir, "api", "run1")
	records, _ := filepath.Glob(filepath.Join(journal, "*.json"))
	if len(records) != 1 {
		t.Fatalf("records left: %v", records)
	}
}

// A shell call has no turn, so the same words are two operations.
func TestAnUnscopedKeyNeverJoins(t *testing.T) {
	dir := journalDir(t)
	key := Key{Epoch: "run1", Digest: "d"}
	first, joined, err := Begin(dir, "api", key, Record{})
	if err != nil || joined {
		t.Fatalf("first: %v %v", joined, err)
	}
	second, joined, err := Begin(dir, "api", key, Record{})
	if err != nil || joined || second.Token == first.Token {
		t.Fatalf("second joined the first: %v %v", joined, err)
	}
}

// Another conversation, turn or run with the same words is another operation.
func TestTheKeyScopesByRunConversationAndTurn(t *testing.T) {
	dir := journalDir(t)
	first, _, _ := Begin(dir, "api", scoped, Record{})
	for _, other := range []Key{
		{Epoch: "run1", Conversation: "c2", Turn: "t", Digest: "d"},
		{Epoch: "run1", Conversation: "c", Turn: "t2", Digest: "d"},
		{Epoch: "run1", Conversation: "c", Turn: "t", Digest: "d2"},
		{Epoch: "run2", Conversation: "c", Turn: "t", Digest: "d"},
	} {
		record, joined, err := Begin(dir, "api", other, Record{})
		if err != nil || joined || record.Token == first.Token {
			t.Errorf("%+v joined %+v", other, scoped)
		}
	}
}

// A token names a record of its own run and nothing else.
func TestATokenBelongsToItsRun(t *testing.T) {
	dir := journalDir(t)
	record, _, _ := Begin(dir, "api", scoped, Record{})
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
	record, _, _ := Begin(dir, "api", scoped, Record{})
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
	tool, _, _ := Begin(dir, "api", scoped, Record{Transport: "test"})
	tool.Created = time.Now().Add(-48 * time.Hour)
	_ = Save(dir, "api", tool)
	shell, _, _ := Begin(dir, "api", Key{Epoch: "run1", Digest: "d"}, Record{Transport: Shell})
	finished, _, _ := Begin(dir, "api", Key{Epoch: "run1", Digest: "d"}, Record{Transport: Shell})
	finished.Phase = Done
	_ = Save(dir, "api", finished)
	uncertain, _, _ := Begin(dir, "api", Key{Epoch: "run1", Digest: "d"}, Record{Transport: Shell})
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
	if len(found) != 3 || !found[tool.Token] || !found[shell.Token] || !found[uncertain.Token] {
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
// keeps what a recovery still reads: an open record, and a read left in
// progress.
func TestSweepKeepsWhatARecoveryReads(t *testing.T) {
	dir := journalDir(t)
	done, _, _ := Begin(dir, "api", scoped, Record{})
	done.Phase = Done
	_ = Save(dir, "api", done)
	open, _, _ := Begin(dir, "api", Key{Epoch: "run1", Conversation: "c", Turn: "t", Digest: "open"}, Record{})
	reading, _, _ := Begin(dir, "api", Key{Epoch: "run1", Conversation: "c", Turn: "t", Digest: "read"}, Record{})
	reading.Phase = Done
	reading.Read = &ReadBatch{Letters: []Letter{{ID: "m", Parts: []Part{{Start: 0, End: 1, Shown: []Shown{{CallID: "x"}}}, {Start: 1, End: 2}}}}}
	_ = Save(dir, "api", reading)
	other, _, _ := Begin(dir, "api", Key{Epoch: "run0", Digest: "d"}, Record{})

	unread := func(string) bool { return true }
	uncertain, _, _ := Begin(dir, "api", Key{Epoch: "run1", Conversation: "c", Turn: "t", Digest: "unknown"}, Record{})
	uncertain.Phase, uncertain.Uncertain = Done, true
	_ = Save(dir, "api", uncertain)
	Sweep(dir, "api", "run1", time.Now().Add(-time.Hour), unread)
	for _, token := range []string{done.Token, open.Token, reading.Token} {
		if _, err := Load(dir, "api", "run1", token); err != nil {
			t.Fatalf("a fresh record went: %v", err)
		}
	}
	Sweep(dir, "api", "run1", time.Now().Add(time.Hour), unread)
	if _, err := Load(dir, "api", "run1", done.Token); !errors.Is(err, ErrUnknown) {
		t.Errorf("an old finished record stays: %v", err)
	}
	if again, joined, _ := Begin(dir, "api", scoped, Record{}); joined || again.Token == done.Token {
		t.Error("the swept record's key still joins")
	}
	if _, err := Load(dir, "api", "run1", open.Token); err != nil {
		t.Errorf("an open record went: %v", err)
	}
	if _, err := Load(dir, "api", "run1", reading.Token); err != nil {
		t.Errorf("a read in progress went: %v", err)
	}
	if _, err := Load(dir, "api", "run1", uncertain.Token); err != nil {
		t.Errorf("a record whose effect is unknown went: %v", err)
	}
	// A letter in progress that was read elsewhere is no longer in progress.
	Sweep(dir, "api", "run1", time.Now().Add(time.Hour), func(string) bool { return false })
	if _, err := Load(dir, "api", "run1", reading.Token); !errors.Is(err, ErrUnknown) {
		t.Errorf("a read whose letter was read elsewhere stays: %v", err)
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
	record, _, err := Begin(dir, "api", scoped, Record{})
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
	Sweep(dir, "api", "run1", time.Now().Add(time.Hour), func(string) bool { return true })
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
