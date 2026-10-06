package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/receipt"
	"github.com/praline-labs/rewake/internal/state"
)

// The effects that rule 6 stops when a shared lookup cannot tell
// (docs/mail-bridge-cli.md#the-rules-the-code-holds); the lookups themselves
// are pinned in the inbox package.

// closed makes a file unreadable until the returned function opens it again.
func closed(t *testing.T, path string) func() {
	t.Helper()
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
	return func() {
		if err := os.Chmod(path, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// A first read that could not see the letter freezes nothing: once the letter
// reads, the same words show it.
func TestAReadThatCouldNotSeeTheLetterFreezesNothing(t *testing.T) {
	dir, self, web := toolSession(t)
	id := rawUnread(t, dir, "api", map[string]any{"from": "web", "fromEpoch": web.Epoch(), "toEpoch": self.Epoch(), "kind": "task", "text": "READ_ONCE_IT_READS"})
	reopen := closed(t, filepath.Join(state.UnreadPath(dir, "api"), id+".json"))
	tool := newToolCaller(t)
	if blind := tool.run("inbox", "--message", id); blind.code == ExitOK {
		t.Fatalf("read a letter it could not read: %+v", blind)
	}
	reopen()
	if again := tool.run("inbox", "--message", id); again.code != ExitOK || !strings.Contains(again.out, "READ_ONCE_IT_READS") {
		t.Fatalf("the same words once the letter reads: %+v", again)
	}
}

// A letter being read in parts that cannot be read may be owed: pending is
// not refused as nothing owed, and stays open for rewake retry.
func TestAClaimedLetterThatCannotBeReadLeavesTheMarkOpen(t *testing.T) {
	dir, self, web := toolSession(t)
	id := rawUnread(t, dir, "api", map[string]any{"from": "web", "fromEpoch": web.Epoch(), "toEpoch": self.Epoch(), "kind": "task", "text": "in parts"})
	tool := newToolCaller(t)
	if first := tool.run("inbox", "--message", id); first.code != ExitOK {
		t.Fatal(first)
	}
	if _, claimed := inbox.ReadClaimed(dir, "api", id); !claimed {
		t.Fatal("the read claimed nothing")
	}
	turnStarted(t, dir, self, markAt-1)
	closed(t, filepath.Join(state.UnreadPath(dir, "api"), id+".json"))
	if blind := tool.run("pending", "the check is running"); blind.code != ExitFailed || !strings.Contains(blind.errOut, "rewake retry ") {
		t.Fatalf("a mark past a claimed letter it could not read: %+v", blind)
	}
}

// ackLab is api with a part of web's task shown and its acknowledgment due.
func ackLab(t *testing.T, id string, tool *toolCaller) (readPartModel, toolRun) {
	t.Helper()
	got := tool.run("inbox", "--message", id, "--json")
	var part readPartModel
	if got.code != ExitOK || json.Unmarshal([]byte(got.out), &part) != nil {
		t.Fatalf("the read: %+v", got)
	}
	return part, got
}

// An acknowledgment that cannot read the waiter records nothing over it: the
// waiter may name a task still owed.
func TestAnAcknowledgmentDoesNotWriteOverAWaiterItCannotRead(t *testing.T) {
	dir, self, web := toolSession(t)
	earlier := readFrom(t, dir, web)
	id := rawUnread(t, dir, "api", map[string]any{"from": "web", "fromEpoch": web.Epoch(), "toEpoch": self.Epoch(), "kind": "task", "text": "the next task"})
	part, got := ackLab(t, id, newToolCaller(t))
	waiter := filepath.Join(state.AwaitingPath(dir, "api"), self.Epoch(), "web")
	reopen := closed(t, waiter)
	err := AcknowledgeRead(dir, "api", self.Epoch(), part.Receipt, whole(got), nil)
	reopen()
	if kept, readErr := os.ReadFile(waiter); err == nil || readErr != nil || !strings.Contains(string(kept), earlier) {
		t.Fatalf("acknowledged over an unreadable waiter: %v, the waiter now %q", err, kept)
	}
}

// An acknowledgment retried after its move did not finish, over a status it
// cannot read, stops: the status may say the read was recorded, and
// recording it again would owe the report twice.
func TestARetriedAcknowledgmentStopsOnAStatusItCannotRead(t *testing.T) {
	dir, self, web := toolSession(t)
	id := rawUnread(t, dir, "api", map[string]any{"from": "web", "fromEpoch": web.Epoch(), "toEpoch": self.Epoch(), "kind": "task", "text": "one task"})
	part, got := ackLab(t, id, newToolCaller(t))
	// A closed archive rather than a file in its place: the whole reading
	// before the acknowledgment stops on a file of no kind, and the move
	// would not be reached.
	done := state.DonePath(dir, "api")
	if err := os.MkdirAll(done, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(done, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(done, 0o700) })
	if err := AcknowledgeRead(dir, "api", self.Epoch(), part.Receipt, whole(got), nil); err == nil {
		t.Fatal("the archive was not in the way")
	}
	// Reported on since: the waiter is gone, the letter still unread.
	for _, waiter := range must(inbox.ReadWaiters(dir, "api", self.Epoch())) {
		if err := inbox.ClearAwaiting(dir, "api", self.Epoch(), waiter); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(done, 0o700); err != nil {
		t.Fatal(err)
	}
	closed(t, filepath.Join(state.InboxPath(dir, "api"), id+".status"))
	err := AcknowledgeRead(dir, "api", self.Epoch(), part.Receipt, whole(got), nil)
	record, loadErr := receipt.Load(dir, "api", self.Epoch(), part.Receipt)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if waiters := must(inbox.ReadWaiters(dir, "api", self.Epoch())); err == nil || len(waiters) != 0 || record.Read.Letters[0].Read {
		t.Fatalf("acknowledged past an unreadable status: %v, waiters %+v", err, waiters)
	}
}

// A held answer that cannot be read stops the turn end rather than publish a
// report without it and then drop it.
func TestAKeptAnswerThatCannotBeReadIsNotDropped(t *testing.T) {
	lab := newInterimLab(t)
	lab.held(t, "the suite is green: 40 pass")
	kept := filepath.Join(state.InboxPath(lab.dir, "api"), "pending", "kept.json")
	closed(t, kept)
	run("turn-ended", stopPayload(t, "Stop", "nothing more to add", true))
	for _, report := range reportsTo(t, lab.dir, "web") {
		if inbox.KindOf(report) == inbox.Finished {
			t.Fatalf("published without the held answer: %q", report.Text)
		}
	}
	if _, err := os.Stat(kept); err != nil {
		t.Fatalf("the held answer was dropped: %v", err)
	}
}

// A result that cannot be read stays unknown whoever holds the name now: a
// letter read already, or delivered with its waiting copy left behind,
// reported as failed before delivery, would be sent again.
func TestAnUnreadableResultIsNotFailedWhenTheNameMovesOn(t *testing.T) {
	for _, where := range []string{"archived", "waiting"} {
		t.Run(where, func(t *testing.T) {
			dir, self, web := toolSession(t)
			message := inbox.Message{ID: inbox.NewID(), From: self.Name, FromEpoch: self.Epoch(), To: web.Name, ToEpoch: web.Epoch(), Kind: inbox.Note, Text: "taken already", CreatedAt: time.Now()}
			directory := state.DonePath(dir, "web")
			recorded := `{"state":"read"}`
			if where == "waiting" {
				directory, recorded = state.InboxPath(dir, "web"), `{"state":"delivered"}`
			}
			if err := state.EnsureSubdir(directory); err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(message)
			if err := os.WriteFile(filepath.Join(directory, message.ID+".json"), raw, 0o600); err != nil {
				t.Fatal(err)
			}
			status := filepath.Join(state.InboxPath(dir, "web"), message.ID+".status")
			if err := os.WriteFile(status, []byte(recorded), 0o600); err != nil {
				t.Fatal(err)
			}
			closed(t, status)
			otherRun(t, dir, "web")
			var out, errOut bytes.Buffer
			ctx := &Context{Stdout: &out, Stderr: &errOut, JSON: true}
			failure := reportSent(ctx, sent{dir: dir, self: self, epoch: self.Epoch(), target: web}, message, noteKind, 0)
			var model sendModel
			if err := json.Unmarshal(out.Bytes(), &model); err != nil {
				t.Fatalf("%s (%v): %v", out.String(), failure, err)
			}
			if model.State != string(inbox.Pending) || !strings.Contains(model.Detail, "could not be read") {
				t.Fatalf("an unreadable result after the name moved on: %+v", model)
			}
		})
	}
}

// A turn end whose clearing fails says so, and the next turn end finishes
// it before it reads a waiter: a task read since from the same sender joins
// the old record, and the old task is reported once.
func TestAClearingThatFailedIsFinishedBeforeTheNextReport(t *testing.T) {
	dir, self, web := toolSession(t)
	earlier := readFrom(t, dir, web)
	waiter := filepath.Join(state.AwaitingPath(dir, "api"), self.Epoch(), "web")
	previous := beforeReports
	beforeReports = func() { _ = os.Chmod(waiter, 0) }
	t.Cleanup(func() { beforeReports = previous; _ = os.Chmod(waiter, 0o600) })
	failed := completeTurn(dir, self, inbox.TurnEnd{Boundary: boundaryNow(t, dir, self), ID: "first", Text: "the first is done"}, "")
	beforeReports = previous
	if err := os.Chmod(waiter, 0o600); err != nil {
		t.Fatal(err)
	}
	if failed == nil {
		t.Fatal("a turn end whose clearing failed answered success")
	}
	if left := unfinishedJournals(t, dir); left != 1 {
		t.Fatalf("the unfinished clearing left %d records", left)
	}
	later := readFrom(t, dir, web)
	if err := completeTurn(dir, self, inbox.TurnEnd{Boundary: boundaryNow(t, dir, self), ID: "second", Text: "the second is done"}, ""); err != nil {
		t.Fatal(err)
	}
	answered := map[string]int{}
	for _, report := range reportsTo(t, dir, "web") {
		for _, id := range report.InReplyTo {
			answered[id]++
		}
	}
	if answered[earlier] != 1 || answered[later] != 1 {
		t.Fatalf("reports per task: %v", answered)
	}
	if left := unfinishedJournals(t, dir); left != 0 {
		t.Fatalf("a finished clearing left %d records", left)
	}
}
