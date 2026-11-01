package cli

import (
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/infra/state"
)

func withdrawn(t *testing.T, dir, id string) bool {
	t.Helper()
	status, _, _ := mail.ReadStatus(dir, "api", id)
	return status.Withdrawn
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
	if err := mail.Open(dir, "api", epochOf(t, dir, "api")).MarkRead(stored(t, dir, addendum), true); err != nil {
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
		if err := mail.Open(dir, "api", api).MarkRead(stored(t, dir, id), true); err != nil {
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
	if replacement := stored(t, dir, idLine.FindStringSubmatch(out)[1]); mail.KindOf(replacement) != mail.Note || replacement.Replaces != old {
		t.Fatalf("replacement %+v", replacement)
	}
}
