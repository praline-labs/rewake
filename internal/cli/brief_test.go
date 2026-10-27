package cli

import (
	"encoding/json"
	"slices"
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

func keys(t *testing.T, raw any) []string {
	t.Helper()
	object, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("%v is no object", raw)
	}
	var names []string
	for name := range object {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// The --json of list and show is what rewake's shell reads: its fields are
// held here by name, so a rename fails before the shell draws nothing.
func TestTheShellsJSONOfBriefs(t *testing.T) {
	dir, worker := briefWorld(t)
	lead, _ := registry.Lookup(dir, "lead")
	if code, out, errOut := run("brief", "new", "# Auth refactor\n\n- [ ] move the login\n- [ ] run the tests\n"); code != ExitOK || !strings.Contains(out, "brief 1 made") {
		t.Fatalf("new: %d %q %q", code, out, errOut)
	}
	run("brief", "new", "# Old work\n")
	if code, _, errOut := run("brief", "close", "2"); code != ExitOK {
		t.Fatalf("close: %d %q", code, errOut)
	}
	author := taskbrief.Author{Name: worker.Name, Epoch: worker.Epoch()}
	if err := taskbrief.Bind(taskbrief.Home{Room: dir}, 1, author); err != nil {
		t.Fatal(err)
	}
	if _, err := taskbrief.Tick(taskbrief.Home{Room: dir}, 1, author, 1, 2, true); err != nil {
		t.Fatal(err)
	}
	if _, _, err := taskbrief.Comment(taskbrief.Home{Room: dir}, 1, author, "the tests pass"); err != nil {
		t.Fatal(err)
	}
	as(t, lead)

	code, out, errOut := run("brief", "list", "--closed", "--json")
	if code != ExitOK {
		t.Fatalf("list: %d %q", code, errOut)
	}
	var listing map[string]any
	if err := json.Unmarshal([]byte(out), &listing); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(keys(t, listing), ","); got != "briefs,room,unreadable" {
		t.Fatalf("list's fields: %s", got)
	}
	briefs := listing["briefs"].([]any)
	if len(briefs) != 2 {
		t.Fatalf("briefs = %v", briefs)
	}
	summaryFields := "bound,closed,commentCount,created,done,edition,id,revision,title,total"
	if got := strings.Join(keys(t, briefs[0]), ","); got != summaryFields {
		t.Fatalf("an open brief's fields: %s", got)
	}
	if got := strings.Join(keys(t, briefs[1]), ","); got != "bound,closed,closedAt,commentCount,created,done,edition,id,revision,title,total" {
		t.Fatalf("a closed brief's fields: %s", got)
	}
	first := briefs[0].(map[string]any)
	if first["id"] != 1.0 || first["title"] != "Auth refactor" || first["done"] != 1.0 || first["total"] != 2.0 || first["commentCount"] != 1.0 || first["edition"] != 1.0 || first["revision"] != 2.0 {
		t.Fatalf("brief 1 = %v", first)
	}
	bound := first["bound"].([]any)
	if len(bound) != 1 || strings.Join(keys(t, bound[0]), ",") != "epoch,name,run,since" || bound[0].(map[string]any)["run"] != "running" {
		t.Fatalf("bound = %v", bound)
	}

	code, out, errOut = run("brief", "show", "1", "--json")
	if code != ExitOK {
		t.Fatalf("show: %d %q", code, errOut)
	}
	var shown map[string]any
	if err := json.Unmarshal([]byte(out), &shown); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(keys(t, shown), ","); got != "bound,closed,commentCount,comments,created,done,edition,id,items,revision,text,title,total" {
		t.Fatalf("show's fields: %s", got)
	}
	if shown["text"] != "# Auth refactor\n\n- [ ] move the login\n- [x] run the tests\n" {
		t.Fatalf("text = %q", shown["text"])
	}
	items := shown["items"].([]any)
	if len(items) != 2 || strings.Join(keys(t, items[1]), ",") != "done,n,text" || items[1].(map[string]any)["done"] != true {
		t.Fatalf("items = %v", items)
	}
	comments := shown["comments"].([]any)
	if len(comments) != 1 || strings.Join(keys(t, comments[0]), ",") != "at,edition,from,n,text" || comments[0].(map[string]any)["from"] != "worker" {
		t.Fatalf("comments = %v", comments)
	}
}

// A worker reads the numbered text and ticks by its edition; main edits the
// text with the ticks applied, and the worker's old number is refused.
func TestAWorkerTicksByTheEditionItRead(t *testing.T) {
	dir, worker := briefWorld(t)
	lead, _ := registry.Lookup(dir, "lead")
	run("brief", "new", "# Two steps\n- [ ] one\n- [ ] two\n")
	if err := taskbrief.Bind(taskbrief.Home{Room: dir}, 1, taskbrief.Author{Name: worker.Name, Epoch: worker.Epoch()}); err != nil {
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
