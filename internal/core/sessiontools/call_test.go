package sessiontools

import (
	"maps"
	"slices"
	"strings"
	"testing"
)

var worker = Run{
	Executable: "/r/rewake", Dir: "/s", Room: "room", Session: "w", Epoch: "7", Role: "worker",
}

func TestAPlanIsTheCommandTheRunsVariablesAndItsBound(t *testing.T) {
	plan, refused := worker.Plan("brief_tick", decode(t, `{"id":3,"item":1,"edition":2}`))
	if refused != "" {
		t.Fatal(refused)
	}
	if !slices.Equal(plan.Argv, []string{"/r/rewake", "brief", "tick", "3", "1", "--edition=2"}) {
		t.Fatalf("argv %q", plan.Argv)
	}
	want := map[string]string{"REWAKE_DIR": "/s", "REWAKE_ROOM": "room", "REWAKE_SESSION": "w", "REWAKE_EPOCH": "7"}
	if !maps.Equal(plan.Env, want) || plan.TimeoutMs != 60_000 {
		t.Fatalf("env %v, bound %d", plan.Env, plan.TimeoutMs)
	}
}

func TestOnlyAnEntryPlanSaysTheRunMayEnter(t *testing.T) {
	entering := worker
	entering.Enter = true
	plan, refused := entering.Plan("worktree_enter", decode(t, `{}`))
	if refused != "" || plan.Env["REWAKE_WORKTREE_ENTER"] != "1" || plan.TimeoutMs != 120_000 {
		t.Fatalf("%+v %q", plan, refused)
	}
	plan, _ = entering.Plan("worktree_status", decode(t, `{}`))
	if _, ok := plan.Env["REWAKE_WORKTREE_ENTER"]; ok {
		t.Fatalf("status says the run may enter: %v", plan.Env)
	}
}

// A dropped tool, or one of another role, may still be called by a model
// that read an older table: nothing runs for it.
func TestAToolTheRunWasNotOfferedIsRefused(t *testing.T) {
	for _, name := range []string{"worktree_land", "worktree_enter", "no_such_tool"} {
		if _, refused := worker.Plan(name, decode(t, `{"name":"x"}`)); refused != name+" is not a tool of this session" {
			t.Fatalf("%s: %q", name, refused)
		}
	}
	main := worker
	main.Role = "main"
	if _, refused := main.Plan("brief_read", decode(t, `{"id":1}`)); refused == "" {
		t.Fatal("main was planned a brief tool")
	}
	none := worker
	none.Executable = ""
	if _, refused := none.Plan("worktree_status", nil); !strings.Contains(refused, "REWAKE_EXECUTABLE") {
		t.Fatalf("%q", refused)
	}
}

func code(n int) *int { return &n }

func TestACommandThatRanAnswersItsOutputOrItsFailure(t *testing.T) {
	for _, c := range []struct {
		tool string
		ran  Ran
		want Outcome
	}{
		{"worktree_status", Ran{ExitCode: code(0), Stdout: " {\"a\":1}\n"}, Outcome{Text: `{"a":1}`}},
		{"brief_tick", Ran{ExitCode: code(1), Stdout: "out", Stderr: " edition 3, not 2 \n"}, Outcome{IsError: true, Text: "rewake brief tick failed (exit 1): edition 3, not 2"}},
		{"worktree_land", Ran{ExitCode: code(2), Stdout: " only out "}, Outcome{IsError: true, Text: "rewake worktree land failed (exit 2): only out"}},
		{"worktree_list", Ran{}, Outcome{IsError: true, Text: "rewake worktree ls failed (exit null): no output"}},
		// A failure says what the command said even when its output was cut.
		{"worktree_list", Ran{ExitCode: code(1), Stderr: "no room", StdoutCut: true}, Outcome{IsError: true, Text: "rewake worktree ls failed (exit 1): no room"}},
		{"worktree_status", Ran{ExitCode: code(0), Stdout: "{", StdoutCut: true}, Outcome{IsError: true, Text: "rewake worktree status answered more than the harness keeps; its answer was cut"}},
	} {
		if got := Answer(c.tool, c.ran); got != c.want {
			t.Fatalf("%s %+v: %+v, want %+v", c.tool, c.ran, got, c.want)
		}
	}
}

// pi reports a child it killed at the bound with code 0, so killed is read
// before the code.
func TestACommandThatDidNotRunToItsEndIsAnErrorSayingWhy(t *testing.T) {
	for _, c := range []struct {
		tool string
		ran  Ran
		want string
	}{
		{"brief_read", Ran{Error: "spawn ENOENT"}, "rewake brief show did not run to its end: spawn ENOENT"},
		{"brief_read", Ran{ExitCode: code(0), Killed: true}, "rewake brief show did not run to its end: stopped after 60000 ms or by an abort"},
		{"worktree_land", Ran{Killed: true}, "rewake worktree land did not run to its end: stopped after 120000 ms or by an abort"},
	} {
		if got := Answer(c.tool, c.ran); got != (Outcome{IsError: true, Text: c.want}) {
			t.Fatalf("%s: %+v", c.tool, got)
		}
	}
}

func TestEnteredReadsTheCheckoutsPath(t *testing.T) {
	if path, err := Entered(`{"checkout":{"path":"/w/fix"},"made":true}`); err != nil || path != "/w/fix" {
		t.Fatalf("%q %v", path, err)
	}
	for _, text := range []string{`{}`, `not json`, `{"checkout":{"path":""}}`} {
		if _, err := Entered(text); err == nil {
			t.Fatalf("%s read", text)
		}
	}
}
