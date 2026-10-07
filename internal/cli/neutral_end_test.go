package cli

import (
	"context"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// The end-identity and failure oracles on neutral completions: an adapter's
// end as the contract carries it, confirmed through ConfirmCompletion. They
// stand beside the same oracles on the hook and gateway paths, which leave
// with those paths (docs/v2/stage3-steps.md#s5).

// confirm hands one end to the core as an adapter that can hold it does.
func confirm(t *testing.T, dir string, self registry.Session, completion harness.Completion) {
	t.Helper()
	if reason, err := ConfirmCompletion(context.Background(), dir, self, completion); err != nil || reason != "" {
		t.Fatalf("confirming %s: reason %q, %v", completion.ID, reason, err)
	}
}

func selfOf(t *testing.T, dir string) registry.Session {
	t.Helper()
	self, err := registry.Lookup(dir, "api")
	if err != nil {
		t.Fatal(err)
	}
	return self
}

// Two ends of one conversation with distinct ids, the same in everything
// else, are two ends: each reports.
func TestNeutralEndsWithDistinctIDsReportTwice(t *testing.T) {
	dir := liveSession(t, "api")
	peer := otherRun(t, dir, "web")
	self := selfOf(t, dir)
	for _, id := range []string{"run/A/1", "run/A/2"} {
		readFrom(t, dir, peer)
		confirm(t, dir, self, harness.Completion{ID: id, Thread: "A", Kind: inbox.Finished, Text: "done", Boundary: boundaryNow(t, dir, self)})
	}
	if reports := finishedFor(t, dir, peer.Name); len(reports) != 2 {
		t.Fatalf("two ends with distinct ids made %d reports", len(reports))
	}
	if waiters := inbox.Waiters(dir, self.Name, self.Epoch()); len(waiters) != 0 {
		t.Fatalf("waits remain: %v", waiters)
	}
}

// A stopped end and then a finished one of the same turn report twice, and
// the finished one settles: the stopped one is advisory and does not stand
// for the turn's final word.
func TestNeutralStoppedThenFinishedEndsOfOneTurnReportTwice(t *testing.T) {
	dir := liveSession(t, "api")
	peer := otherRun(t, dir, "web")
	self := selfOf(t, dir)
	readFrom(t, dir, peer)
	stopped := harness.Completion{ID: "run/A/T", Thread: "A", Kind: inbox.Stopped, Text: "stopped", Boundary: boundaryNow(t, dir, self)}
	confirm(t, dir, self, stopped)
	if waiters := inbox.Waiters(dir, self.Name, self.Epoch()); len(waiters) != 1 {
		t.Fatalf("the stopped end settled the wait: %v", waiters)
	}
	final := stopped
	final.Kind, final.Text = inbox.Finished, "continued result"
	confirm(t, dir, self, final)
	if reports := finishedFor(t, dir, peer.Name); len(reports) != 2 {
		t.Fatalf("a stopped and a finished end made %d reports", len(reports))
	}
	if waiters := inbox.Waiters(dir, self.Name, self.Epoch()); len(waiters) != 0 {
		t.Fatalf("the finished end did not settle: %v", waiters)
	}
}

// Two confirmations of one end at once — an answer lost and the end sent
// again while the first is still being taken — tell the waiter once.
func TestNeutralTwoEndsAtOnceReportOnce(t *testing.T) {
	dir := liveSession(t, "api")
	peer := otherRun(t, dir, "web")
	self := selfOf(t, dir)
	readFrom(t, dir, peer)
	end := harness.Completion{ID: "run/A/1", Thread: "A", Kind: inbox.Finished, Text: "done", Boundary: boundaryNow(t, dir, self)}

	second := make(chan error, 1)
	first := true
	previous := beforeReports
	beforeReports = func() {
		if !first {
			return
		}
		first = false
		go func() {
			_, err := ConfirmCompletion(context.Background(), dir, self, end)
			second <- err
		}()
		// Give the second call every chance to get through first.
		select {
		case err := <-second:
			second <- err
		case <-time.After(300 * time.Millisecond):
		}
	}
	t.Cleanup(func() { beforeReports = previous })

	confirm(t, dir, self, end)
	if err := <-second; err != nil {
		t.Fatal(err)
	}
	if reports := finishedFor(t, dir, peer.Name); len(reports) != 1 {
		t.Errorf("web holds %d reports, want one", len(reports))
	}
}

// A failed end reaches every waiting sender, verbatim.
func TestNeutralFailedEndsReachEveryWaitingSender(t *testing.T) {
	dir := failedWaiters(t)
	self := selfOf(t, dir)
	confirm(t, dir, self, harness.Completion{ID: "run/A/failed", Thread: "A", Kind: inbox.Error, Text: "verbatim failure", Boundary: boundaryNow(t, dir, self)})
	for _, peer := range []string{"one", "two"} {
		report := reportObject(t, dir, peer)
		if report["kind"] != "error" || report["text"] != "verbatim failure" {
			t.Fatalf("report to %s: %v", peer, report)
		}
	}
}

// An end without its read boundary has no known scope: it is refused, reports
// nothing, and its waits stay owed for the next end. One without its event's
// id cannot be known again, and is refused too.
func TestNeutralEndsWithoutTheirScopeReportNothing(t *testing.T) {
	for name, completion := range map[string]func(*inbox.ReadBoundary) harness.Completion{
		"no boundary": func(*inbox.ReadBoundary) harness.Completion {
			return harness.Completion{ID: "run/A/failed", Thread: "A", Kind: inbox.Error, Text: "verbatim failure"}
		},
		"no id": func(boundary *inbox.ReadBoundary) harness.Completion {
			return harness.Completion{Thread: "A", Kind: inbox.Error, Text: "verbatim failure", Boundary: boundary}
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := failedWaiters(t)
			self := selfOf(t, dir)
			if _, err := ConfirmCompletion(context.Background(), dir, self, completion(boundaryNow(t, dir, self))); err == nil {
				t.Fatal("an end without its scope was taken")
			}
			for _, peer := range []string{"one", "two"} {
				if files := finishedFor(t, dir, peer); len(files) != 0 {
					t.Fatalf("reports to %s: %v", peer, files)
				}
			}
			if len(inbox.Waiters(dir, "api", self.Epoch())) != 2 {
				t.Fatal("the refused end cleared its waits")
			}
		})
	}
}

// A failure nobody waits for goes to main; main's own stays in its mailbox.
func TestNeutralUnclaimedFailuresReachMainAndMainKeepsItsOwn(t *testing.T) {
	for _, own := range []bool{false, true} {
		t.Run(map[bool]string{true: "self", false: "leader"}[own], func(t *testing.T) {
			dir := liveSession(t, "api")
			t.Setenv(state.SessionEnv, "api")
			if own {
				markMain(t, dir, "api")
			} else {
				otherRun(t, dir, "leader")
				markMain(t, dir, "leader")
			}
			self := selfOf(t, dir)
			confirm(t, dir, self, harness.Completion{ID: "run/A/failed", Thread: "A", Kind: inbox.Error, Text: "failure", Boundary: boundaryNow(t, dir, self)})
			if own {
				messages, err := inbox.PeekUnread(dir, "api", self.Epoch())
				if err != nil || len(messages) != 1 || messages[0].Kind != inbox.Error {
					t.Fatalf("main's own failure: %+v %v", messages, err)
				}
				if len(finishedFor(t, dir, "api")) != 0 {
					t.Fatal("main woke itself with its failure")
				}
			} else if reportObject(t, dir, "leader")["kind"] != "error" {
				t.Fatal("the failure did not reach main as an error")
			}
		})
	}
}

// An end that says nothing after work is an error without text, not a
// finished report of nothing.
func TestNeutralAnEmptyEndAfterWorkReportsAnError(t *testing.T) {
	dir := liveSession(t, "api")
	peer := otherRun(t, dir, "web")
	self := selfOf(t, dir)
	readFrom(t, dir, peer)
	confirm(t, dir, self, harness.Completion{ID: "run/A/1", Thread: "A", Kind: inbox.Finished, Boundary: boundaryNow(t, dir, self)})
	report := reportObject(t, dir, peer.Name)
	if report["kind"] != "error" || report["text"] != "" {
		t.Fatalf("report=%v", report)
	}
}
