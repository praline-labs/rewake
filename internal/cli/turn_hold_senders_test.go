package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/state"
)

// senderLab is the interim lab with a second sender: web and second both wait
// for api's report.
func senderLab(t *testing.T) interimLab {
	t.Helper()
	lab := newInterimLab(t)
	second := otherRun(t, lab.dir, "second")
	rawUnread(t, lab.dir, "api", map[string]any{"from": second.Name, "fromEpoch": second.Epoch(), "toEpoch": lab.self.Epoch(), "text": "second task"})
	if code, _, errOut := run("inbox"); code != ExitOK {
		t.Fatalf("read the second task: %s", errOut)
	}
	return lab
}

// finishedTo reports whether a session holds the report text as a finished
// report.
func finishedTo(t *testing.T, dir, name, text string) bool {
	t.Helper()
	for _, report := range reportsTo(t, dir, name) {
		if inbox.KindOf(report) == inbox.Finished && report.Text == text {
			return true
		}
	}
	return false
}

// A sender whose record could not be read fails the whole check, whichever of
// the two it is: the hold would otherwise be asked for a set of senders the
// check never saw. The end is published to both.
func TestOneUnreadableSenderAbandonsTheHold(t *testing.T) {
	for _, failing := range []string{"web", "second"} {
		t.Run(failing, func(t *testing.T) {
			lab := senderLab(t)
			failed := false
			state.Fault = func(op, path string) error {
				if !failed && op == state.OpRead && path == state.SessionPath(lab.dir, failing) {
					failed = true
					return os.ErrPermission
				}
				return nil
			}
			t.Cleanup(func() { state.Fault = nil })
			reason := lab.confirm(t, lab.end("turn-2", 1, inbox.Finished, "the work is done"))
			state.Fault = nil
			if !failed {
				t.Fatalf("the read of %s never reached the fault seam", failing)
			}
			if reason != "" || lab.kept() {
				t.Fatalf("a failed read of %s still held the end: reason=%q kept=%v", failing, reason, lab.kept())
			}
			for _, name := range []string{"web", "second"} {
				if !finishedTo(t, lab.dir, name, "the work is done") {
					t.Errorf("the failed check did not fall through to publishing to %s", name)
				}
			}
		})
	}
}

// A sender proven gone is not a failure: the end is held for the one still
// running, and the reason names only it.
func TestAGoneSenderLeavesTheHoldToTheLiveOne(t *testing.T) {
	lab := senderLab(t)
	if err := os.Remove(state.SessionPath(lab.dir, "second")); err != nil {
		t.Fatal(err)
	}
	reason := lab.confirm(t, lab.end("turn-2", 1, inbox.Finished, "the work is done"))
	if !strings.Contains(reason, "web still wait") || !lab.kept() {
		t.Fatalf("the hold with one sender gone: reason=%q kept=%v", reason, lab.kept())
	}
}
