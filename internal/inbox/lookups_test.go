package inbox

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/state"
)

// The shared lookups answer found, proven absent, or an error: only a file
// that is not there is absence (rule 6 of
// docs/mail-bridge-cli.md#the-rules-the-code-holds). These pin the primitives
// and the effects nearest to them.

// A letter that cannot be read, or does not parse, may be the one asked for:
// the listing fails rather than leave it out.
func TestALetterThatCannotBeReadIsNotLeftOut(t *testing.T) {
	for _, fault := range []string{"unreadable", "unparsable"} {
		t.Run(fault, func(t *testing.T) {
			dir := stateDir(t)
			letter := announcedUnread(t, dir, Message{ID: NewID(), From: "web", FromEpoch: "w1", To: "api", ToEpoch: "e1", Kind: Task, Text: "listed", CreatedAt: time.Now()})
			path := filepath.Join(state.UnreadPath(dir, "api"), letter.ID+".json")
			if fault == "unreadable" {
				unreadable(t, path, []byte(`{}`))
			} else if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
				t.Fatal(err)
			}
			if unread, err := PeekUnread(dir, "api", "e1"); err == nil {
				t.Fatalf("listed %d letters past one it could not read", len(unread))
			}
			if available, err := AvailableUnread(dir, "api", "e1"); err == nil {
				t.Fatalf("offered %d letters past one it could not read", len(available))
			}
		})
	}
}

// A status that cannot be read, or does not parse, may say read or withdrawn;
// only a missing one is none written.
func TestAStatusIsFoundAbsentOrUnknown(t *testing.T) {
	dir := stateDir(t)
	id := NewID()
	if err := state.EnsureSubdir(state.InboxPath(dir, "api")); err != nil {
		t.Fatal(err)
	}
	if _, known, err := ReadStatus(dir, "api", id); known || err != nil {
		t.Fatalf("a status never written: %v %v", known, err)
	}
	if err := os.WriteFile(statusPath(dir, "api", id), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, known, err := ReadStatus(dir, "api", id); err == nil {
		t.Fatalf("a status that does not parse answered known=%v", known)
	}
	unreadable(t, statusPath(dir, "api", id), []byte(`{"state":"read"}`))
	if _, known, err := ReadStatus(dir, "api", id); err == nil {
		t.Fatalf("a status that cannot be read answered known=%v", known)
	}
}

// A mailbox that cannot be searched does not prove a message has left it.
func TestAnsweredNeedsAMailboxItCanSearch(t *testing.T) {
	dir := stateDir(t)
	inbox := state.InboxPath(dir, "api")
	if err := state.EnsureSubdir(inbox); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(inbox, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(inbox, 0o700) })
	if answered, err := Answered(dir, "api", NewID()); err == nil {
		t.Fatalf("an unsearchable mailbox answered answered=%v", answered)
	}
}

// A waiter that cannot be read may name what is owed already: recording a
// read on top of it would write that over.
func TestAWaiterThatCannotBeReadIsNotWrittenOver(t *testing.T) {
	dir := stateDir(t)
	path, _ := awaitingPath(dir, "api", "e1")
	earlier := []byte(`{"name":"web","epoch":"w1","messages":["earlier"]}`)
	unreadable(t, filepath.Join(path, "web"), earlier)
	if err := markAwaitingSequence(dir, "api", "e1", "web", "w1", NewID(), 0, 0); err == nil {
		t.Fatal("recorded a read over a waiter it could not read")
	}
	if err := os.Chmod(filepath.Join(path, "web"), 0o600); err != nil {
		t.Fatal(err)
	}
	if kept, err := os.ReadFile(filepath.Join(path, "web")); err != nil || string(kept) != string(earlier) {
		t.Fatalf("the waiter now holds %q (%v)", kept, err)
	}
}

// A read whose status cannot be read may be a retry of one that recorded its
// waiter already: it stops, the letter still unread and nothing owed twice.
func TestAReadStopsOnAStatusItCannotRead(t *testing.T) {
	dir := stateDir(t)
	letter := announcedUnread(t, dir, Message{ID: NewID(), From: "web", FromEpoch: "w1", To: "api", ToEpoch: "e1", Kind: Task, Text: "read once", CreatedAt: time.Now()})
	unreadable(t, statusPath(dir, "api", letter.ID), []byte(`{"state":"read"}`))
	if err := MarkRead(dir, "api", "e1", letter, true); err == nil {
		t.Fatal("read past a status it could not read")
	}
	if waiters, err := ReadWaiters(dir, "api", "e1"); err != nil || len(waiters) != 0 {
		t.Fatalf("waiters after the stopped read: %+v (%v)", waiters, err)
	}
	if _, err := os.Stat(filepath.Join(state.UnreadPath(dir, "api"), letter.ID+".json")); err != nil {
		t.Fatalf("the letter left unread/: %v", err)
	}
}

// A withdrawal that cannot tell where the letter stands stops before it writes
// anything. Here the letter is being read in parts while its waiting copy is
// still there, as between the notice and its settling: taken for not in
// unread/, it would be withdrawn from the waiting copy past the claim.
func TestAWithdrawalThatCannotLookStopsBeforeWriting(t *testing.T) {
	dir := stateDir(t)
	letter := announcedUnread(t, dir, Message{ID: NewID(), From: "web", FromEpoch: "w1", To: "api", ToEpoch: "e1", Kind: Task, Text: "being read", CreatedAt: time.Now()})
	unread := state.UnreadPath(dir, "api")
	if err := os.Link(filepath.Join(unread, letter.ID+".json"), filepath.Join(state.InboxPath(dir, "api"), letter.ID+".json")); err != nil {
		t.Fatal(err)
	}
	if err := ClaimRead(dir, "api", letter.ID, "receipt"); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(unread, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(unread, 0o700) })
	if _, err := withdrawNow(t, dir, letter, nil); err == nil {
		t.Fatal("withdrew a letter it could not find")
	}
	if status, known, err := ReadStatus(dir, "api", letter.ID); err != nil || known {
		t.Fatalf("the stopped withdrawal wrote %+v (%v)", status, err)
	}
}

// A claimed letter that cannot be read may be owed: pending cannot say
// nothing is.
func TestAClaimedLetterThatCannotBeReadIsNotOwedByNobody(t *testing.T) {
	dir := stateDir(t)
	letter := announcedUnread(t, dir, Message{ID: NewID(), From: "web", FromEpoch: "w1", To: "api", ToEpoch: "e1", Kind: Task, Text: "in parts", CreatedAt: time.Now()})
	if err := ClaimRead(dir, "api", letter.ID, "receipt"); err != nil {
		t.Fatal(err)
	}
	if senders := must(ClaimedOwedBy(dir, "api", "e1")); len(senders) != 1 || senders[0] != "web" {
		t.Fatalf("a readable claimed letter is owed to %v", senders)
	}
	unreadable(t, filepath.Join(state.UnreadPath(dir, "api"), letter.ID+".json"), []byte(`{}`))
	if senders, err := ClaimedOwedBy(dir, "api", "e1"); err == nil {
		t.Fatalf("an unreadable claimed letter is owed to %v", senders)
	}
}

// The records of a turn end that cannot be read are not taken for none: a
// kept answer would be published without, and a mark would publish a pending
// turn as the report and be removed.
func TestATurnRecordThatCannotBeReadIsNotNone(t *testing.T) {
	dir := stateDir(t)
	if err := state.EnsureSubdir(pendingDir(dir, "api")); err != nil {
		t.Fatal(err)
	}
	unreadable(t, keptPath(dir, "api"), []byte(`{"epoch":"e1","text":"the answer"}`))
	if _, held, err := KeptAnswer(dir, "api", "e1"); err == nil {
		t.Fatalf("an unreadable kept answer answered held=%v", held)
	}
	unreadable(t, interimPath(dir, "api"), []byte(`{"epoch":"e1","text":"running"}`))
	if _, interim, err := LastInterim(dir, "api", "e1"); err == nil {
		t.Fatalf("an unreadable interim record answered interim=%v", interim)
	}
	file := MarkName(2, NewID())
	path, _ := marksPath(dir, "api", "e1")
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, file), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, marked, err := MarkWithin(dir, "api", "e1", 1, 2); err == nil {
		t.Fatalf("a mark that does not parse answered marked=%v", marked)
	}
	if _, err := os.Stat(filepath.Join(path, file)); err != nil {
		t.Fatalf("the mark it could not read was removed: %v", err)
	}
}

// Finished mail whose waiter cannot be read may still be owed: the sweep
// keeps it rather than guess.
func TestTheSweepKeepsWhatAnUnreadableWaiterMayOwe(t *testing.T) {
	dir := stateDir(t)
	run := liveRunOf(t, dir, "api")
	letter := Message{ID: NewID(), From: "web", FromEpoch: "w1", To: "api", ToEpoch: run, Kind: Task, Text: "owed", CreatedAt: time.Now()}
	if err := state.EnsureSubdir(state.DonePath(dir, "api")); err != nil {
		t.Fatal(err)
	}
	done := filepath.Join(state.DonePath(dir, "api"), letter.ID+".json")
	if err := os.WriteFile(done, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * keepFinished)
	if err := os.Chtimes(done, old, old); err != nil {
		t.Fatal(err)
	}
	path, _ := awaitingPath(dir, "api", run)
	unreadable(t, filepath.Join(path, "web"), []byte(`{"name":"web","epoch":"w1","messages":["`+letter.ID+`"]}`))
	(&Server{Dir: dir, Name: "api", Epoch: run, attempts: map[string]time.Time{}, outcomes: map[string]Result{}}).sweepFinished()
	if _, err := os.Stat(done); err != nil {
		t.Fatalf("the sweep removed a letter an unreadable waiter may owe: %v", err)
	}
}

// Waiters drops the error a waiter that cannot be read gives: no code outside
// tests may decide by it.
func TestNothingButTestsCallsTheLossyWaiters(t *testing.T) {
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != root && strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch fun := call.Fun.(type) {
			case *ast.Ident:
				if fun.Name == "Waiters" && file.Name.Name == "inbox" {
					t.Errorf("%s calls Waiters; ReadWaiters answers its error", path)
				}
			case *ast.SelectorExpr:
				if x, ok := fun.X.(*ast.Ident); ok && x.Name == "inbox" && fun.Sel.Name == "Waiters" {
					t.Errorf("%s calls inbox.Waiters; ReadWaiters answers its error", path)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// A wait record whose numbers do not parse is no record of zeros: read so, it
// would owe nothing and close the task it names, and a grant with it. The
// record from before times were kept, the run alone, still reads.
func TestADamagedWaitRecordIsNotReadAsZeros(t *testing.T) {
	dir := stateDir(t)
	letter := announcedUnread(t, dir, Message{ID: NewID(), From: "web", FromEpoch: "w1", To: "api", ToEpoch: "1.1", Kind: Task, Text: "owed", CreatedAt: time.Now()})
	if err := MarkRead(dir, "api", "1.1", letter, true); err != nil {
		t.Fatal(err)
	}
	path, _ := awaitingPath(dir, "api", "1.1")
	record := filepath.Join(path, "web")
	for _, damaged := range []string{
		"w1 broken " + letter.ID,
		"w1 1 " + letter.ID + " broken",
		"w1 1 " + letter.ID + " 1 broken",
		"w1 1 " + letter.ID + " 1 1 extra",
	} {
		if err := os.WriteFile(record, []byte(damaged), 0o600); err != nil {
			t.Fatal(err)
		}
		if waiters, err := ReadWaiters(dir, "api", "1.1"); err == nil {
			t.Fatalf("%q read as %+v", damaged, waiters)
		}
		if Settled(dir, "api", letter.ID) {
			t.Fatalf("%q settled the task", damaged)
		}
		if open, found := TaskOpen(dir, "api", letter.ID); !open || !found {
			t.Fatalf("%q closed the task: open=%v found=%v", damaged, open, found)
		}
		if err := markAwaitingSequence(dir, "api", "1.1", "web", "w1", NewID(), 0, 0); err == nil {
			t.Fatalf("recorded a read over %q", damaged)
		}
		if err := ClearAwaiting(dir, "api", "1.1", Waiter{Name: "web", Epoch: "w1", Messages: []string{letter.ID}}); err == nil {
			t.Fatalf("cleared %q", damaged)
		}
		if kept, _ := os.ReadFile(record); string(kept) != damaged {
			t.Fatalf("%q was written over with %q", damaged, kept)
		}
	}
	if err := os.WriteFile(record, []byte("w1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if waiters, err := ReadWaiters(dir, "api", "1.1"); err != nil || len(waiters) != 1 || waiters[0].Epoch != "w1" {
		t.Fatalf("the record of the run alone: %+v (%v)", waiters, err)
	}
}
