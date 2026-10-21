package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
)

func readEntry(t *testing.T, args ...string) worktreeEntry {
	t.Helper()
	code, out, errOut := run(append(append([]string{"worktree", "enter"}, args...), "--json")...)
	var entry worktreeEntry
	if code != ExitOK || json.Unmarshal([]byte(out), &entry) != nil {
		t.Fatalf("enter %v: %d %s %s", args, code, out, errOut)
	}
	return entry
}

// A worker that may enter a checkout gets one, claimed for its run so its
// space's close finds it, and the same one on every later call: one
// checkout per worker. Without its template's leave, nothing is made.
func TestWorktreeEnterMakesOneCheckoutPerRun(t *testing.T) {
	lab := newWorktreeLab(t)
	asStatusRun(t, lab, false, true)
	t.Chdir(lab.repo)
	// A worker that may enter a checkout exports it to the tests it runs.
	t.Setenv(adapter.WorktreeEnterEnv, "")
	if code, _, errOut := run("worktree", "enter", "--json"); code != ExitUsage || !strings.Contains(errOut, "enter_worktree = true") {
		t.Fatalf("without leave: %d %s", code, errOut)
	}
	t.Setenv(adapter.WorktreeEnterEnv, "1")
	first := readEntry(t, "fix")
	if first.Outcome != enterCreated || first.Checkout.Branch != "fix" {
		t.Fatalf("first: %+v", first)
	}
	if _, err := os.Stat(first.Checkout.Path); err != nil {
		t.Fatal(err)
	}
	if status := readStatus(t); status.Checkout == nil || status.Checkout.Path != first.Checkout.Path || status.Close.Outcome != closeRemoves {
		t.Fatalf("status after entry: %+v", status)
	}
	for _, args := range [][]string{nil, {"fix"}} {
		if again := readEntry(t, args...); again.Outcome != enterReused || again.Checkout.Path != first.Checkout.Path {
			t.Fatalf("again %v: %+v", args, again)
		}
	}
	if code, _, errOut := run("worktree", "enter", "other", "--json"); code != ExitUsage || !strings.Contains(errOut, "one checkout per worker") {
		t.Fatalf("another name: %d %s", code, errOut)
	}
}

// A run started in the checkout it owns is already there.
func TestWorktreeEnterOfARunStartedInItsCheckout(t *testing.T) {
	lab := newWorktreeLab(t)
	r := asStatusRun(t, lab, true, true)
	t.Setenv(adapter.WorktreeEnterEnv, "1")
	if entry := readEntry(t); entry.Outcome != enterAlready || entry.Checkout.Path != r.record.Path {
		t.Fatalf("%+v", entry)
	}
}
