package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// readFrom has api read a message from the given run of web.
func readFrom(t *testing.T, dir string, web registry.Session) {
	t.Helper()
	current, _ := registry.Lookup(dir, "api")
	t.Setenv(state.SessionEnv, "api")
	t.Setenv(epochEnv, current.Epoch())
	rawUnread(t, dir, "api", map[string]any{"from": "web", "fromEpoch": web.Epoch(), "toEpoch": current.Epoch(), "text": "rerun"})
	if code, _, errOut := run("inbox"); code != ExitOK {
		t.Fatalf("inbox: %d %s", code, errOut)
	}
}

func finishedFor(t *testing.T, dir, name string) []string {
	t.Helper()
	found, _ := filepath.Glob(filepath.Join(state.InboxPath(dir, name), "*.json"))
	return found
}

// The report goes to the run that wrote, not to whoever holds the name when the
// turn ends.
func TestTurnEndSkipsANameThatChangedHands(t *testing.T) {
	dir := liveSession(t, "api")
	first := otherRun(t, dir, "web")
	readFrom(t, dir, first)
	otherRun(t, dir, "web")

	run("turn-ended", turnPayload)
	if found := finishedFor(t, dir, "web"); len(found) != 0 {
		t.Errorf("web holds %v; the report went to a run that never wrote", found)
	}
}

// What an earlier run of this name read is not this run's to report.
func TestTurnEndIgnoresAnEarlierRunsReading(t *testing.T) {
	dir := liveSession(t, "api")
	web := otherRun(t, dir, "web")
	readFrom(t, dir, web)

	t.Setenv(epochEnv, "7.7")
	run("turn-ended", turnPayload)
	if found := finishedFor(t, dir, "web"); len(found) != 0 {
		t.Errorf("web holds %v; a later run reported a turn it did not have", found)
	}
}

// A report that could not be written must still be owed.
func TestTurnEndKeepsTheWaitWhenTheReportFails(t *testing.T) {
	dir := liveSession(t, "api")
	web := otherRun(t, dir, "web")
	readFrom(t, dir, web)

	mailbox := state.InboxPath(dir, "web")
	_ = os.RemoveAll(mailbox)
	if err := os.WriteFile(mailbox, nil, 0o600); err != nil {
		t.Fatalf("block: %v", err)
	}
	run("turn-ended", turnPayload)
	if err := os.Remove(mailbox); err != nil {
		t.Fatalf("unblock: %v", err)
	}

	run("turn-ended", turnPayload)
	if found := finishedFor(t, dir, "web"); len(found) != 1 {
		t.Errorf("web holds %v after the mailbox came back, want the owed report", found)
	}
}

// A hook that gets no payload must not wait for one forever.
func TestTurnEndDoesNotWaitOnAnOpenPipe(t *testing.T) {
	liveSession(t, "api")
	t.Setenv(state.SessionEnv, "api")
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer writer.Close()
	previous := os.Stdin
	os.Stdin = reader
	defer func() { os.Stdin = previous }()

	done := make(chan struct{})
	go func() {
		run("turn-ended")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(8 * time.Second):
		t.Fatal("turn-ended is still waiting on a pipe that never closes")
	}
}

// Two ends of a turn reported at once — a hook run twice — tell each waiter once.
func TestTwoTurnEndsAtOnceReportOnce(t *testing.T) {
	dir := liveSession(t, "api")
	web := otherRun(t, dir, "web")
	readFrom(t, dir, web)

	second := make(chan struct{})
	first := true
	previous := beforeReports
	beforeReports = func() {
		if !first {
			return
		}
		first = false
		go func() {
			run("turn-ended", turnPayload)
			close(second)
		}()
		// Give the second call every chance to get through first.
		select {
		case <-second:
		case <-time.After(300 * time.Millisecond):
		}
	}
	t.Cleanup(func() { beforeReports = previous })

	run("turn-ended", turnPayload)
	<-second
	if found := finishedFor(t, dir, "web"); len(found) != 1 {
		t.Errorf("web holds %d reports, want one", len(found))
	}
}

// A read whose last step failed is retried. The retry must not owe the sender
// a second report for the same message.
func TestARetriedReadReportsOnce(t *testing.T) {
	dir := liveSession(t, "api")
	web := otherRun(t, dir, "web")
	t.Setenv(state.SessionEnv, "api")
	rawUnread(t, dir, "api", map[string]any{"from": "web", "fromEpoch": web.Epoch(), "toEpoch": epochOf(t, dir, "api"), "text": "execute once"})
	blocked := state.DonePath(dir, "api")
	if err := os.WriteFile(blocked, nil, 0o600); err != nil {
		t.Fatalf("block: %v", err)
	}
	run("inbox")
	run("turn-ended", turnPayload)
	if err := os.Remove(blocked); err != nil {
		t.Fatalf("unblock: %v", err)
	}
	run("inbox")
	run("turn-ended", turnPayload)

	if found := finishedFor(t, dir, "web"); len(found) != 1 {
		t.Errorf("one message produced %d reports", len(found))
	}
}

// A waiter that cannot be removed after its report was written must not be
// reported to again at every following turn.
func TestAStuckWaiterIsReportedOnce(t *testing.T) {
	dir := liveSession(t, "api")
	web := otherRun(t, dir, "web")
	readFrom(t, dir, web)
	waits := filepath.Join(state.AwaitingPath(dir, "api"), epochOf(t, dir, "api"))
	if err := os.Chmod(waits, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(waits, 0o700) })

	run("turn-ended", turnPayload)
	// The report is delivered and read before the next turn ends, as it would
	// be: it has left the waiting set by then.
	reports := finishedFor(t, dir, "web")
	if len(reports) != 1 {
		t.Fatalf("web holds %v after the first turn, want one report", reports)
	}
	done := state.DonePath(dir, "web")
	if err := os.MkdirAll(done, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Rename(reports[0], filepath.Join(done, filepath.Base(reports[0]))); err != nil {
		t.Fatalf("move: %v", err)
	}

	run("turn-ended", turnPayload)
	everywhere := 0
	for _, directory := range []string{state.InboxPath(dir, "web"), state.UnreadPath(dir, "web"), done} {
		found, _ := filepath.Glob(filepath.Join(directory, "*.json"))
		everywhere += len(found)
	}
	if everywhere != 1 {
		t.Errorf("one wait produced %d reports", everywhere)
	}
}

// A session can write to itself; its report goes into its own mailbox, which
// the hook already holds. Nothing may take that lock a second time.
func TestAReportToOneselfDoesNotDeadlock(t *testing.T) {
	dir := liveSession(t, "api")
	t.Setenv(state.SessionEnv, "api")
	epoch := epochOf(t, dir, "api")
	rawUnread(t, dir, "api", map[string]any{"from": "api", "fromEpoch": epoch, "toEpoch": epoch, "text": "note to self"})
	if code, _, errOut := run("inbox"); code != ExitOK {
		t.Fatalf("inbox: %s", errOut)
	}
	if code, _, errOut := run("turn-ended", turnPayload); code != ExitOK {
		t.Fatalf("turn-ended: %s", errOut)
	}
	if found := finishedFor(t, dir, "api"); len(found) != 1 {
		t.Errorf("the report to oneself was written %d times", len(found))
	}
}

// The report of a turn names what it answers.
func TestAReportNamesWhatItAnswers(t *testing.T) {
	dir := liveSession(t, "api")
	web := otherRun(t, dir, "web")
	readFrom(t, dir, web)
	run("turn-ended", turnPayload)

	found := finishedFor(t, dir, "web")
	if len(found) != 1 {
		t.Fatalf("web holds %v, want one report", found)
	}
	raw, _ := os.ReadFile(found[0])
	var report inbox.Message
	if err := json.Unmarshal(raw, &report); err != nil || len(report.InReplyTo) != 1 {
		t.Errorf("report = %s, want it to name the message it answers", raw)
	}
}

// A heads-up owes nothing: reading it leaves nobody waiting.
func TestANotifyOwesNoReport(t *testing.T) {
	dir := liveSession(t, "api")
	web := otherRun(t, dir, "web")
	t.Setenv(state.SessionEnv, "api")
	rawUnread(t, dir, "api", map[string]any{"from": "web", "fromEpoch": web.Epoch(), "toEpoch": epochOf(t, dir, "api"), "kind": "notify", "text": "merged"})
	run("inbox")
	run("turn-ended", turnPayload)
	if found := finishedFor(t, dir, "web"); len(found) != 0 {
		t.Errorf("web holds %v after a notify, want no report", found)
	}
}
