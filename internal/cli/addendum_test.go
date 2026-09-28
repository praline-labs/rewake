package cli

import (
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/state"
)

// reportLands makes api's report on a task arrive between the first look at
// it and the one under the lock, as a turn end would.
func reportLands(t *testing.T, dir, task string) {
	t.Helper()
	web := epochOf(t, dir, "web")
	old := beforeAddendumLock
	t.Cleanup(func() { beforeAddendumLock = old })
	beforeAddendumLock = func() {
		rawUnread(t, dir, "web", map[string]any{"from": "api", "fromEpoch": epochOf(t, dir, "api"), "kind": "finished", "toEpoch": web, "inReplyTo": []string{task}, "text": "done"})
	}
}

func withdrawn(t *testing.T, dir, id string) bool {
	t.Helper()
	status, _ := inbox.ReadStatus(dir, "api", id)
	return status.Withdrawn
}

// A report that lands after send --to looked at the task is seen by the look
// under the lock: the addendum is refused, not sent to work already reported.
func TestAnAddendumIsAskedAgainUnderTheLock(t *testing.T) {
	dir, _ := sender(t)
	task := sendOK(t, "api", "rerun the smoke")
	reportLands(t, dir, task)
	code, _, errOut := run("send", "api", "also rerun the lint", "--to", task)
	if code != ExitFailed || !strings.Contains(errOut, "already reported on") {
		t.Fatalf("send --to across a report: %d %q", code, errOut)
	}
}

// And so is an edit of an addendum: its replacement would add to a task
// reported on, so the addendum stays as it was.
func TestAnAddendumsEditIsAskedAgainUnderTheLock(t *testing.T) {
	dir, _ := sender(t)
	task := sendOK(t, "api", "rerun the smoke")
	addendum := sendOK(t, "api", "also rerun the lint", "--to", task)
	reportLands(t, dir, task)
	code, _, errOut := run("edit", addendum, "also rerun the vet")
	if code != ExitFailed || !strings.Contains(errOut, "already reported on") || withdrawn(t, dir, addendum) {
		t.Fatalf("edit across a report: %d %q, withdrawn %v", code, errOut, withdrawn(t, dir, addendum))
	}
}

// Withdrawing a task takes its unread addenda along — an addendum left behind
// would be read on its own and owe a report on work taken back — each told
// as its notice calls for.
func TestWithdrawingATaskTakesItsAddendaAlong(t *testing.T) {
	dir, _ := sender(t)
	task := sendOK(t, "api", "rerun the smoke")
	loud := sendOK(t, "api", "also rerun the lint", "--to", task)
	quiet := sendOK(t, "api", "and the vet", "--to", loud)
	other := sendOK(t, "api", "an unrelated task")
	announce(t, dir, loud)
	code, out, errOut := run("withdraw", task)
	if code != ExitOK ||
		!strings.Contains(out, "withdrew its addendum "+loud+" too; its notice may have gone out") ||
		!strings.Contains(out, "withdrew its addendum "+quiet+" too, before its notice went out") {
		t.Fatalf("withdraw: %d %q %q", code, out, errOut)
	}
	for _, id := range []string{task, loud, quiet} {
		if !withdrawn(t, dir, id) {
			t.Errorf("%s is not withdrawn", id)
		}
	}
	if withdrawn(t, dir, other) {
		t.Error("a task that is no addendum went with it")
	}
	// The task's notice never went out, the loud addendum's may have: one
	// recall, for the addendum.
	if told := recalls(t, dir); len(told) != 1 || told[0].Recall.ID != loud {
		t.Fatalf("recalls %+v", told)
	}
}

// A read addendum is final and stays owed; the task it adds to is recalled
// even though its own notice never went out, since the addendum named it.
func TestAReadAddendumStaysAndItsTaskIsRecalled(t *testing.T) {
	dir, _ := sender(t)
	task := sendOK(t, "api", "rerun the smoke")
	addendum := sendOK(t, "api", "also rerun the lint", "--to", task)
	if err := inbox.MarkRead(dir, "api", epochOf(t, dir, "api"), stored(t, dir, addendum), true); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := run("withdraw", task)
	if code != ExitOK || !strings.Contains(out, "api has read its addendum "+addendum+" already") ||
		strings.Contains(out, "saw nothing") || !strings.Contains(out, "api has read an addendum to it, so it is told not to act on it") {
		t.Fatalf("withdraw: %d %q %q", code, out, errOut)
	}
	if withdrawn(t, dir, addendum) {
		t.Error("a read addendum was withdrawn")
	}
	if told := recalls(t, dir); len(told) != 1 || told[0].Recall.ID != task {
		t.Fatalf("recalls %+v", told)
	}
}

// An edit leaves the addenda where they are and they add to the replacement:
// the sender is told, --to by either id adds to the replacement, and both
// sides list them under it.
func TestAnEditsReplacementKeepsTheAddenda(t *testing.T) {
	dir, _ := sender(t)
	task := sendOK(t, "api", "rerun the smoke")
	addendum := sendOK(t, "api", "also rerun the lint", "--to", task)
	code, out, errOut := run("edit", task, "rerun the smoke on staging")
	if code != ExitOK {
		t.Fatalf("edit: %d %q %q", code, out, errOut)
	}
	replacement := idLine.FindStringSubmatch(out)[1]
	if !strings.Contains(out, "your addendum "+addendum+" now adds to "+replacement) {
		t.Fatalf("edit did not say where the addendum went: %q", out)
	}
	for _, to := range []string{task, addendum} {
		if message := stored(t, dir, sendOK(t, "api", "one more", "--to", to)); message.AddendumTo != replacement {
			t.Fatalf("--to %s added to %s, not the replacement %s", to, message.AddendumTo, replacement)
		}
	}

	_, out, _ = run("inbox", "--awaited")
	if at, under := strings.Index(out, replacement), strings.Index(out, "+ "+addendum+" · addendum to "+shortRef(replacement)); at < 0 || under < at {
		t.Fatalf("--awaited does not list the addendum under the replacement:\n%s", out)
	}

	api := epochOf(t, dir, "api")
	for _, id := range []string{replacement, addendum} {
		if err := inbox.MarkRead(dir, "api", api, stored(t, dir, id), true); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv(state.SessionEnv, "api")
	t.Setenv(epochEnv, api)
	_, out, _ = run("inbox", "--owed")
	if at, under := strings.Index(out, "rerun the smoke on staging"), strings.Index(out, "+ addendum from web"); at < 0 || under < at {
		t.Fatalf("--owed does not list the addendum under the replacement:\n%s", out)
	}
}

// An edit of a notify keeps its kind: the replacement is a notify too.
func TestAnEditOfANotifyKeepsItsKind(t *testing.T) {
	dir, _ := sender(t)
	old := sendOK(t, "api", "the build is red", "--notify")
	code, out, errOut := run("edit", old, "the build is green after all")
	if code != ExitOK {
		t.Fatalf("edit: %d %q %q", code, out, errOut)
	}
	if replacement := stored(t, dir, idLine.FindStringSubmatch(out)[1]); inbox.KindOf(replacement) != inbox.Note || replacement.Replaces != old {
		t.Fatalf("replacement %+v", replacement)
	}
}
