package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/state"
	"github.com/iiiokojiadbi/rewake/internal/worktree"
)

// made is one checkout of the lab's repository, owned by nobody yet.
func (lab worktreeLab) made(t *testing.T, name string) worktree.Record {
	t.Helper()
	record, err := worktree.Create(lab.root, lab.repo, name)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

// ls reports whose each checkout is, whether that session still runs and
// what removing it would lose, in lines and in the same model under --json.
func TestWorktreeLsReportsOwnerAndState(t *testing.T) {
	lab := newWorktreeLab(t)
	if code, out, _ := run("worktree", "ls"); code != ExitOK || !strings.Contains(out, "No worktrees under "+lab.root) {
		t.Fatalf("empty: %d %q", code, out)
	}
	if err := lab.launch(t, codexProbe(t), lab.repo, "--worktree=ended"); err != nil {
		t.Fatal(err)
	}
	live := lab.made(t, "live")
	roomDir, _ := state.RoomDir(lab.state, "trees")
	session := otherRun(t, roomDir, "busy-codex")
	if _, err := worktree.Claim(live, worktree.Owner{Name: session.Name, Room: "trees", Epoch: session.Epoch(), Harness: "codex", Dir: roomDir}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live.Path, "new"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, out, errOut := run("worktree", "ls")
	if code != ExitOK {
		t.Fatalf("ls: %d %s", code, errOut)
	}
	for _, want := range []string{"root: " + lab.root, "WORKTREE", live.Ref(), "busy-codex (running)", "changes", "tree-codex (ended)", "clean"} {
		if !strings.Contains(out, want) {
			t.Errorf("ls lacks %q:\n%s", want, out)
		}
	}

	code, out, _ = run("worktree", "ls", "--json")
	var listing worktreeListing
	if code != ExitOK || json.Unmarshal([]byte(out), &listing) != nil || len(listing.Worktrees) != 2 {
		t.Fatalf("ls --json: %d %s", code, out)
	}
	byName := map[string]worktreeView{}
	for _, view := range listing.Worktrees {
		byName[view.Name] = view
	}
	if view := byName["live"]; !view.Running || !view.Check.Changes || view.Session == nil || view.Session.Name != "busy-codex" {
		t.Errorf("live: %+v", view)
	}
	if view := byName["ended"]; view.Running || view.Check.Dirty() || view.Commit == "" || view.Path == "" {
		t.Errorf("ended: %+v", view)
	}
}

// rm keeps what would lose work or cut a running session off, says why and
// how to override, and removes it under --force.
func TestWorktreeRmRefusesWhatHoldsWork(t *testing.T) {
	lab := newWorktreeLab(t)
	changed := lab.made(t, "changed")
	if err := os.WriteFile(filepath.Join(changed.Path, "src", "nested", "file"), []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	detached := lab.made(t, "detached")
	lab.git(t, detached.Path, "commit", "-q", "--allow-empty", "-m", "Only here")
	running := lab.made(t, "running")
	roomDir, _ := state.RoomDir(lab.state, "trees")
	session := otherRun(t, roomDir, "busy-codex")
	if _, err := worktree.Claim(running, worktree.Owner{Name: session.Name, Room: "trees", Epoch: session.Epoch(), Harness: "codex", Dir: roomDir}); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"changed":  "has changes",
		"detached": "on no branch",
		"running":  "still run in it: busy-codex in room trees",
	} {
		code, _, errOut := run("worktree", "rm", name)
		if code != ExitUsage || !strings.Contains(errOut, want) || !strings.Contains(errOut, "--force") {
			t.Errorf("rm %s: %d %s", name, code, errOut)
		}
	}
	if records := lab.records(t); len(records) != 3 {
		t.Fatalf("a refused rm removed something: %+v", records)
	}
	for _, name := range []string{"changed", "detached", "running"} {
		if code, out, errOut := run("worktree", "rm", name, "--force"); code != ExitOK || !strings.Contains(out, "removed") {
			t.Errorf("rm %s --force: %d %s %s", name, code, out, errOut)
		}
	}
	if records := lab.records(t); len(records) != 0 {
		t.Errorf("left: %+v", records)
	}
}

// A clean checkout of a session that ended goes without --force, and git
// forgets it; a session of another run of the same name does not hold it.
func TestWorktreeRmRemovesAnEndedSessionsCheckout(t *testing.T) {
	lab := newWorktreeLab(t)
	record := lab.made(t, "done")
	roomDir, _ := state.RoomDir(lab.state, "trees")
	otherRun(t, roomDir, "busy-codex")
	if _, err := worktree.Claim(record, worktree.Owner{Name: "busy-codex", Room: "trees", Epoch: "1.1", Harness: "codex", Dir: roomDir}); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := run("worktree", "rm", record.Ref(), "--json")
	var removal worktreeRemoval
	if code != ExitOK || json.Unmarshal([]byte(out), &removal) != nil || removal.Removed.Path != record.Path || removal.Forced {
		t.Fatalf("rm: %d %s %s", code, out, errOut)
	}
	if _, err := os.Stat(record.Path); !os.IsNotExist(err) {
		t.Errorf("the checkout stayed: %v", err)
	}
	if listed := lab.git(t, lab.repo, "worktree", "list", "--porcelain"); strings.Contains(listed, record.Path) {
		t.Errorf("git still knows it:\n%s", listed)
	}
}

// Wrong calls are refused with what to do instead; a bare name in two
// repositories asks for the repository.
func TestWorktreeCommandRefusals(t *testing.T) {
	lab := newWorktreeLab(t)
	lab.made(t, "twin")
	other := filepath.Join(filepath.Dir(lab.repo), "copy", "project")
	lab.git(t, filepath.Dir(lab.repo), "clone", "-q", lab.repo, other)
	if _, err := worktree.Create(lab.root, other, "twin"); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"worktree"}, "needs ls or rm"},
		{[]string{"worktree", "list"}, "needs ls or rm"},
		{[]string{"worktree", "ls", "x"}, "takes no name"},
		{[]string{"worktree", "ls", "--force"}, "--force is for rm"},
		{[]string{"worktree", "rm"}, "needs the name"},
		{[]string{"worktree", "rm", "nothing"}, "No worktree"},
		{[]string{"worktree", "rm", "twin"}, "in 2 repositories"},
		{[]string{"worktree", "rm", "a", "b"}, "at most 2"},
	} {
		if code, _, errOut := run(c.args...); code != ExitUsage || !strings.Contains(errOut, c.want) {
			t.Errorf("%q: %d %s", c.args, code, errOut)
		}
	}
	if records := lab.records(t); len(records) != 2 {
		t.Errorf("a refusal removed something: %+v", records)
	}
}
