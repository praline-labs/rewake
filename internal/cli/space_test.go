package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// spaceRecord is a record as a writer leaves it, written here by hand so the
// listing is read against the file's shape and not only against the writer.
func spaceRecord(id, name, spaceState, main, namespace, checkout, end string) string {
	return `{"id": "` + id + `", "name": "` + name + `", "state": "` + spaceState + `",
	"main": {"name": "lead-one", "run": "` + main + `", "pidNamespace": "` + namespace + `"},
	"worker": {"harness": "worker-harness", "name": "` + name + `", "role": "write", "sessionId": "0f0e0d0c-0b0a-4908-8706-050403020100", "run": "5151.1800"},
	"checkout": {"path": "` + checkout + `", "branch": "worktree-agent-` + name + `"},
	"host": "6161.1750", "openedAt": "2026-10-13T12:00:00Z"` + end + `}`
}

// rewake space list reads the room's records as a view: open, closed and
// kept spaces with their workers and session ids, a main that is no longer
// running said beside its open space, the kept checkout reminded with the
// commands that clear it, a record that does not read named apart; and a
// room with none says so.
func TestSpaceListShowsOpenClosedAndKept(t *testing.T) {
	t.Setenv(state.DirEnv, filepath.Join(t.TempDir(), "state"))
	t.Setenv(state.RoomEnv, "")
	t.Setenv(state.SessionEnv, "")
	t.Setenv(epochEnv, "")
	if code, out, _ := run("space", "list"); code != ExitOK || !strings.Contains(out, "No spaces in room "+currentRoom(t)+".") {
		t.Fatalf("a base nobody made: %d %q", code, out)
	}
	dir := buildtest.Current(t)
	start, err := proc.Default.StartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	boot, err := registry.CurrentBoot()
	if err != nil {
		t.Fatal(err)
	}
	here := proc.Namespace()
	running := registry.RunEpoch(os.Getpid(), start, boot)
	kept := t.TempDir()
	ids := map[string]string{"open": strings.Repeat("a1", 16), "orphan": strings.Repeat("b2", 16), "closed": strings.Repeat("c3", 16), "kept": strings.Repeat("d4", 16)}
	records := map[string]string{
		"open":   spaceRecord(ids["open"], "open", "open", running, here, "/work/open", ""),
		"orphan": spaceRecord(ids["orphan"], "orphan", "open", registry.RunEpoch(os.Getpid(), start+1, boot), here, "/work/orphan", ""),
		"closed": spaceRecord(ids["closed"], "closed", "closed", running, here, "/work/closed",
			`, "end": {"at": "2026-10-13T13:00:00Z", "why": "closed by main", "worker": "ended", "checkout": {"presence": "gone"}}`),
		"kept": spaceRecord(ids["kept"], "kept", "kept", running, here, kept,
			`, "end": {"at": "2026-10-13T13:00:00Z", "why": "main ended without closing", "worker": "ended", "checkout": {"presence": "there", "commits": true, "locked": true}}`),
	}
	if err := os.MkdirAll(state.SpacesPath(dir), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, text := range records {
		if err := os.WriteFile(state.SpacePath(dir, ids[name]), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	broken := state.SpacePath(dir, strings.Repeat("e5", 16))
	if err := os.WriteFile(broken, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	code, out, errOut := run("space", "list")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	for _, want := range []string{
		"SPACE", "SESSION ID", "0f0e0d0c-0b0a-4908-8706-050403020100",
		"open    ", "orphan  open (main ended)", "closed  closed", "kept    kept",
		"unreadable: " + broken,
		"Rewake: 1 kept checkout stays until removed; rewake removes none.",
		"  kept: " + kept + " on worktree-agent-kept: commits, locked",
		"    git worktree unlock " + kept,
		"    git worktree remove " + kept,
		"    git branch -d worktree-agent-kept",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the listing lacks %q:\n%s", want, out)
		}
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "open ") && strings.Contains(line, "(main") {
			t.Errorf("the running main's space reads as not running: %q", line)
		}
	}

	code, out, _ = run("space", "list", "--json")
	var listing spaceListing
	if err := json.Unmarshal([]byte(out), &listing); code != ExitOK || err != nil {
		t.Fatalf("--json: %d %v %s", code, err, out)
	}
	got := map[string]string{}
	for _, view := range listing.Spaces {
		got[view.Name] = string(view.State) + "/" + view.MainRun
	}
	want := map[string]string{"open": "open/running", "orphan": "open/ended", "closed": "closed/", "kept": "kept/"}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("%s: %q, want %q", name, got[name], w)
		}
	}
	if len(listing.Kept) != 1 || listing.Kept[0].ID != ids["kept"] || len(listing.Unreadable) != 1 {
		t.Fatalf("kept %+v, unreadable %+v", listing.Kept, listing.Unreadable)
	}

	// Removed by hand, the checkout is no longer reminded, though its record
	// still reads kept until a change records it cleared.
	if err := os.Remove(kept); err != nil {
		t.Fatal(err)
	}
	if _, out, _ := run("space", "list"); strings.Contains(out, "kept checkout") || !strings.Contains(out, "kept    kept") {
		t.Fatalf("a kept checkout removed by hand:\n%s", out)
	}
	if code, _, errOut := run("space", "close"); code != ExitUsage || !strings.Contains(errOut, "space needs list") {
		t.Fatalf("a verb list does not know: %d %q", code, errOut)
	}
}
