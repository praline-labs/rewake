package cli

import (
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/taskbrief"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// briefWorld is a main "lead", this process, and a worker run "worker" of
// its room, with the commands run as main.
func briefWorld(t *testing.T) (string, registry.Session) {
	t.Helper()
	dir := liveSession(t, "lead")
	markMain(t, dir, "lead")
	t.Setenv(state.SessionEnv, "lead")
	return dir, otherRun(t, dir, "worker")
}

// as runs the next commands as the run of session.
func as(t *testing.T, session registry.Session) {
	t.Helper()
	t.Setenv(state.SessionEnv, session.Name)
	t.Setenv(state.EpochEnv, session.Epoch())
}

// A worker reads the numbered text and ticks by its edition; main edits the
// text with the ticks applied, and the worker's old number is refused.
func TestAWorkerTicksByTheEditionItRead(t *testing.T) {
	dir, worker := briefWorld(t)
	lead, _ := registry.Lookup(dir, "lead")
	run("brief", "new", "# Two steps\n- [ ] one\n- [ ] two\n")
	if err := taskbrief.Bind(taskbrief.Home{Room: dir}, 1, taskbrief.Author{Name: worker.Name, Epoch: worker.Epoch()}, taskbrief.Work); err != nil {
		t.Fatal(err)
	}
	as(t, worker)
	code, out, _ := run("brief", "show", "1")
	if code != ExitOK || !strings.Contains(out, "brief 1 · Two steps · edition 1 · revision 1 · 0 of 2 done") || !strings.Contains(out, "- [ ] #2 two") || !strings.Contains(out, "rewake brief tick 1 <n> --edition 1") {
		t.Fatalf("show: %d %q", code, out)
	}
	if code, out, errOut := run("brief", "tick", "1", "2", "--edition", "1"); code != ExitOK || out != "Rewake: brief 1 item #2 done: two.\n" {
		t.Fatalf("tick: %d %q %q", code, out, errOut)
	}
	if code, _, errOut := run("brief", "comment", "1", "--", "-- done with two"); code != ExitOK {
		t.Fatalf("comment: %d %q", code, errOut)
	}
	if code, _, errOut := run("brief", "list"); code != ExitFailed || !strings.Contains(errOut, "only main may list") {
		t.Fatalf("a worker's list: %d %q", code, errOut)
	}

	as(t, lead)
	_, markdown, _ := run("brief", "show", "1", "--markdown")
	if markdown != "# Two steps\n- [ ] one\n- [x] two\n" {
		t.Fatalf("markdown = %q", markdown)
	}
	if code, _, errOut := run("brief", "edit", "1", "--rev", "1", markdown+"- [ ] three\n"); code != ExitFailed || !strings.Contains(errOut, "revision 2, not 1") {
		t.Fatalf("a stale edit: %d %q", code, errOut)
	}
	if code, out, errOut := run("brief", "edit", "1", "--rev", "2", strings.Replace(markdown, "- [ ] one", "- [ ] zero\n- [ ] one", 1)); code != ExitOK || !strings.Contains(out, "edition 2") {
		t.Fatalf("edit: %d %q %q", code, out, errOut)
	}

	as(t, worker)
	if code, _, errOut := run("brief", "tick", "1", "1", "--edition", "1"); code != ExitFailed || !strings.Contains(errOut, "edition 2, not 1") || !strings.Contains(errOut, "re-read") {
		t.Fatalf("a stale tick: %d %q", code, errOut)
	}
	if code, out, _ := run("brief", "show", "1"); code != ExitOK || !strings.Contains(out, "- [x] #3 two") || !strings.Contains(out, "1 of 3 done") {
		t.Fatalf("after the edit: %q", out)
	}
}

func TestOnlyMainAndBoundRunsReachABrief(t *testing.T) {
	dir, worker := briefWorld(t)
	run("brief", "new", "- [ ] one\n")
	as(t, worker)
	for _, argv := range [][]string{
		{"brief", "show", "1"},
		{"brief", "tick", "1", "1", "--edition", "1"},
		{"brief", "comment", "1", "--", "hi"},
		{"brief", "new", "mine"},
		{"brief", "close", "1"},
	} {
		if code, _, errOut := run(argv...); code != ExitFailed {
			t.Errorf("%q by an unbound worker: %d %q", argv, code, errOut)
		}
	}
	t.Setenv(state.SessionEnv, "")
	t.Setenv(state.EpochEnv, "")
	if code, _, errOut := run("brief", "list"); code != ExitFailed || !strings.Contains(errOut, "not part of a rewake session") {
		t.Errorf("a plain shell's list: %d %q", code, errOut)
	}
	if _, err := taskbrief.Load(taskbrief.Home{Room: dir}, 2); err == nil {
		t.Error("a worker made a brief")
	}
}

func TestBriefCallsAreCheckedBeforeAnyEffect(t *testing.T) {
	briefWorld(t)
	for _, argv := range [][]string{
		{"brief"},
		{"brief", "drop"},
		{"brief", "new"},
		{"brief", "new", "  "},
		{"brief", "new", "x", "--rev", "1"},
		{"brief", "show", "01"},
		{"brief", "show", "9"},
		{"brief", "edit", "1", "x"},
		{"brief", "tick", "1", "0", "--edition", "1"},
		{"brief", "tick", "1", "1"},
	} {
		if code, _, errOut := run(argv...); code != ExitUsage {
			t.Errorf("%q: %d %q", argv, code, errOut)
		}
	}
}
