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

// commitIn adds a file to a checkout and commits it, returning the new HEAD.
func (lab worktreeLab) commitIn(t *testing.T, dir, file string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, file), []byte(file+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lab.git(t, dir, "add", file)
	lab.git(t, dir, "commit", "-q", "-m", "Add "+file)
	return lab.git(t, dir, "rev-parse", "HEAD")
}

// land takes the worktree's commits into the source's branch as they are and
// says how many; with nothing new it says so. A target that moved on is the
// worktree's state, exit 1, with the rebase to do; a branch that does not
// resolve is a wrong call, exit 2.
func TestWorktreeLandReportsWhatMoved(t *testing.T) {
	lab := newWorktreeLab(t)
	record := lab.made(t, "fix")
	lab.commitIn(t, record.Path, "one")
	tip := lab.commitIn(t, record.Path, "two")

	code, out, errOut := run("worktree", "land", "fix")
	if code != ExitOK || !strings.Contains(out, "landed 2 commits of fix into main") || !strings.Contains(out, "fast-forwarded in "+lab.repo) {
		t.Fatalf("land: %d %q %s", code, out, errOut)
	}
	if head := lab.git(t, lab.repo, "rev-parse", "main"); head != tip {
		t.Errorf("main at %s, want %s", head, tip)
	}
	code, out, _ = run("worktree", "land", "fix", "--json")
	var landing worktreeLanding
	if code != ExitOK || json.Unmarshal([]byte(out), &landing) != nil || landing.Commits != 0 || landing.New != tip || landing.Worktree != record.Ref() {
		t.Errorf("land again: %d %s", code, out)
	}
	if code, out, _ := run("worktree", "land", "fix"); code != ExitOK || !strings.Contains(out, "nothing to land") {
		t.Errorf("land again, lines: %d %s", code, out)
	}

	lab.commitIn(t, record.Path, "three")
	lab.commitIn(t, lab.repo, "theirs")
	code, _, errOut = run("worktree", "land", "fix")
	if code != ExitFailed || !strings.Contains(errOut, "rebase") || !strings.Contains(errOut, "git -C "+record.Path+" rebase main") {
		t.Errorf("diverged: %d %s", code, errOut)
	}
	if code, _, errOut := run("worktree", "land", "fix", "--into", "missing"); code != ExitFailed || !strings.Contains(errOut, "no branch missing") {
		t.Errorf("--into missing: %d %s", code, errOut)
	}
}

// finish lands once more and takes the worktree and its branch away; whatever
// would stop it is asked first, and a refused finish moves nothing.
func TestWorktreeFinishLandsAndRemoves(t *testing.T) {
	lab := newWorktreeLab(t)
	record := lab.made(t, "fix")
	tip := lab.commitIn(t, record.Path, "one")

	roomDir, _ := state.RoomDir(lab.state, "trees")
	session := otherRun(t, roomDir, "busy-codex")
	if _, err := worktree.Claim(record, worktree.Owner{Name: session.Name, Room: "trees", Epoch: session.Epoch(), Harness: "codex", Dir: roomDir}); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := run("worktree", "finish", "fix"); code != ExitFailed || !strings.Contains(errOut, "still run in it: busy-codex") || !strings.Contains(errOut, "Nothing was landed") {
		t.Errorf("finish while running: %d %s", code, errOut)
	}
	if head := lab.git(t, lab.repo, "rev-parse", "main"); head != record.Commit {
		t.Errorf("a refused finish moved main to %s", head)
	}
	if _, err := worktree.Claim(record, worktree.Owner{Name: session.Name, Room: "trees", Epoch: "1.1", Harness: "codex", Dir: roomDir}); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(record.Path, "loose"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := run("worktree", "finish", "fix"); code != ExitFailed || !strings.Contains(errOut, "has changes") {
		t.Errorf("finish with changes: %d %s", code, errOut)
	}
	if err := os.Remove(filepath.Join(record.Path, "loose")); err != nil {
		t.Fatal(err)
	}
	lab.git(t, record.Path, "switch", "-q", "-c", "elsewhere")
	if code, _, errOut := run("worktree", "finish", "fix"); code != ExitFailed || !strings.Contains(errOut, "not on its branch fix") {
		t.Errorf("finish off its branch: %d %s", code, errOut)
	}
	lab.git(t, record.Path, "switch", "-q", "fix")
	lab.git(t, record.Path, "branch", "-q", "-D", "elsewhere")
	if _, err := os.Stat(record.Path); err != nil {
		t.Fatalf("a refused finish removed the checkout: %v", err)
	}

	code, out, errOut := run("worktree", "finish", "fix", "--json")
	var finished worktreeFinish
	if code != ExitOK || json.Unmarshal([]byte(out), &finished) != nil || finished.Landed.Commits != 1 || !finished.BranchDropped {
		t.Fatalf("finish: %d %s %s", code, out, errOut)
	}
	if head := lab.git(t, lab.repo, "rev-parse", "main"); head != tip {
		t.Errorf("main at %s, want %s", head, tip)
	}
	if _, err := os.Stat(record.Path); !os.IsNotExist(err) {
		t.Errorf("the checkout stayed: %v", err)
	}
	if branches := lab.git(t, lab.repo, "branch", "--list", "fix"); branches != "" {
		t.Errorf("the branch stayed: %s", branches)
	}
	if records := lab.records(t); len(records) != 0 {
		t.Errorf("records left: %+v", records)
	}
}

// A finish whose fast-forward is not possible removes nothing.
func TestWorktreeFinishRefusesADivergedTarget(t *testing.T) {
	lab := newWorktreeLab(t)
	record := lab.made(t, "fix")
	lab.commitIn(t, record.Path, "mine")
	lab.commitIn(t, lab.repo, "theirs")
	if code, _, errOut := run("worktree", "finish", "fix"); code != ExitFailed || !strings.Contains(errOut, "rebase") {
		t.Errorf("finish diverged: %d %s", code, errOut)
	}
	if records := lab.records(t); len(records) != 1 {
		t.Errorf("a refused finish removed the record: %+v", records)
	}
	if branches := lab.git(t, lab.repo, "branch", "--list", "fix"); branches == "" {
		t.Error("a refused finish removed the branch")
	}
}

// rm takes the branch along when another ref holds its commits, and keeps one
// with commits only it holds, saying how to take or drop them.
func TestWorktreeRmDropsOnlyABranchNothingIsLostWith(t *testing.T) {
	lab := newWorktreeLab(t)
	clean := lab.made(t, "clean")
	worked := lab.made(t, "worked")
	lab.commitIn(t, worked.Path, "mine")
	if code, out, errOut := run("worktree", "rm", clean.Name); code != ExitOK || !strings.Contains(out, "deleted the branch clean") {
		t.Errorf("rm clean: %d %s %s", code, out, errOut)
	}
	code, out, errOut := run("worktree", "rm", worked.Name)
	if code != ExitOK || !strings.Contains(out, "kept the branch worked") || !strings.Contains(out, "git merge --ff-only worked") {
		t.Errorf("rm worked: %d %s %s", code, out, errOut)
	}
	if branches := lab.git(t, lab.repo, "branch", "--list", "clean", "worked"); strings.Contains(branches, "clean") || !strings.Contains(branches, "worked") {
		t.Errorf("branches left: %q", branches)
	}
}
