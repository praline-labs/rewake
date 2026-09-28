package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/inbox"
)

// editOK replaces a message and answers the replacement's id.
func editOK(t *testing.T, id, text string) string {
	t.Helper()
	code, out, errOut := run("edit", id, text)
	if code != ExitOK {
		t.Fatalf("edit %s: %d %q %q", id, code, out, errOut)
	}
	return idLine.FindStringSubmatch(out)[1]
}

// Withdrawing by the id an edit replaced withdraws the replacement, with its
// addenda, and says so: the old id is still main's name for the task.
func TestWithdrawingByAnEditedIDWithdrawsTheReplacement(t *testing.T) {
	dir, _ := sender(t)
	task := sendOK(t, "api", "rerun the smoke")
	addendum := sendOK(t, "api", "also rerun the lint", "--to", task)
	second := editOK(t, task, "rerun the smoke on staging")
	third := editOK(t, second, "rerun the smoke on staging and prod")

	code, out, errOut := run("withdraw", task, "--json")
	var model withdrawModel
	if code != ExitOK || json.Unmarshal([]byte(out), &model) != nil {
		t.Fatalf("withdraw by the first id: %d %q %q", code, out, errOut)
	}
	if model.ID != third || model.Named != task || model.Result != "unseen" {
		t.Fatalf("withdrew %+v, want %s named by %s", model, third, task)
	}
	for _, id := range []string{third, addendum} {
		if !withdrawn(t, dir, id) {
			t.Errorf("%s is not withdrawn", id)
		}
	}

	_, out, _ = run("withdraw", second)
	if !strings.HasPrefix(out, "Rewake: "+second+" was replaced by "+third+"; withdrawing "+third+".\n") ||
		!strings.Contains(out, "your task "+third+" to api was already withdrawn") {
		t.Fatalf("withdraw by the second id: %q", out)
	}
}

// Editing by the id an edit replaced edits the replacement: the new letter
// replaces the one that stands, not a tombstone.
func TestEditingByAnEditedIDEditsTheReplacement(t *testing.T) {
	dir, _ := sender(t)
	task := sendOK(t, "api", "rerun the smoke")
	second := editOK(t, task, "rerun the smoke on staging")

	code, out, errOut := run("edit", task, "rerun the smoke on prod")
	if code != ExitOK || !strings.HasPrefix(out, "Rewake: "+task+" was replaced by "+second+"; editing "+second+".\n") {
		t.Fatalf("edit by the first id: %d %q %q", code, out, errOut)
	}
	third := idLine.FindStringSubmatch(out)[1]
	if replacement := stored(t, dir, third); replacement.Replaces != second || replacement.Text != "rerun the smoke on prod" || !withdrawn(t, dir, second) {
		t.Fatalf("the third letter %+v does not replace %s", replacement, second)
	}

	code, out, _ = run("edit", task, "rerun the smoke nowhere", "--json")
	var model sendModel
	if code != ExitOK || json.Unmarshal([]byte(out), &model) != nil || model.Replaces != third || model.Named != task {
		t.Fatalf("edit --json by the first id: %d %q", code, out)
	}
}

// Withdrawn again, a task whose addendum was read says the recall went out
// the first time rather than claiming a new one.
func TestWithdrawingAgainSendsNoSecondRecall(t *testing.T) {
	dir, _ := sender(t)
	task := sendOK(t, "api", "rerun the smoke")
	addendum := sendOK(t, "api", "also rerun the lint", "--to", task)
	if err := inbox.MarkRead(dir, "api", epochOf(t, dir, "api"), stored(t, dir, addendum), true); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := run("withdraw", task); code != ExitOK {
		t.Fatalf("withdraw: %d %q", code, errOut)
	}
	_, out, _ := run("withdraw", task)
	if strings.Contains(out, "is told the task was withdrawn") || !strings.Contains(out, "was told the task was withdrawn when it was") {
		t.Fatalf("withdraw again: %q", out)
	}
	if told := recalls(t, dir); len(told) != 1 {
		t.Fatalf("recalls %+v", told)
	}
}

// Two edits of one letter at once: the second finds, under the lock, that the
// first has replaced it, and replaces that replacement instead of refusing
// with advice to send the letter again.
func TestAnEditBesideAnotherReplacesItsReplacement(t *testing.T) {
	dir, _ := sender(t)
	task := sendOK(t, "api", "rerun the smoke")
	addendum := sendOK(t, "api", "also rerun the lint", "--to", task)
	var second string
	old := beforeAddendumLock
	t.Cleanup(func() { beforeAddendumLock = old })
	beforeAddendumLock = func() {
		beforeAddendumLock = old
		second = editOK(t, addendum, "also rerun the vet")
	}

	code, out, errOut := run("edit", addendum, "also rerun the tests")
	if code != ExitOK || !strings.HasPrefix(out, "Rewake: "+addendum+" was replaced by "+second+"; editing "+second+".\n") {
		t.Fatalf("the edit beside another: %d %q %q", code, out, errOut)
	}
	third := idLine.FindStringSubmatch(out)[1]
	if replacement := stored(t, dir, third); replacement.Replaces != second || replacement.AddendumTo != task || !withdrawn(t, dir, second) {
		t.Fatalf("the third letter %+v does not replace %s", replacement, second)
	}
}
