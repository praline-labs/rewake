package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/proc"
	"github.com/praline-labs/rewake/internal/worktree"
)

// launchedElsewhere rewrites a checkout's record as if a rewake launch still
// running in another process had made it and not yet claimed it.
func (worktreeLab) launchedElsewhere(t *testing.T, record worktree.Record) {
	t.Helper()
	command := exec.Command("sleep", "60")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = command.Process.Kill()
		_ = command.Wait()
	})
	start, err := proc.StartTime(command.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(filepath.Dir(record.Path), filepath.Base(record.Path)+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	fields["launcher"] = strconv.Itoa(command.Process.Pid) + "." + strconv.FormatUint(start, 10)
	if raw, err = json.Marshal(fields); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

// A checkout whose launch has made it and not yet registered its session is
// in use: ls says it is launching, and rm keeps it without --force.
func TestWorktreeRmKeepsACheckoutItsLaunchIsStillStarting(t *testing.T) {
	lab := newWorktreeLab(t)
	record := lab.made(t, "starting")
	lab.launchedElsewhere(t, record)
	if code, out, _ := run("worktree", "ls"); code != ExitOK || !strings.Contains(out, "(launching)") {
		t.Errorf("ls: %d %s", code, out)
	}
	if code, _, errOut := run("worktree", "rm", "starting"); code != ExitFailed || !strings.Contains(errOut, "still starting its session") {
		t.Errorf("rm: %d %s", code, errOut)
	}
	if code, out, errOut := run("worktree", "rm", "starting", "--force"); code != ExitOK || !strings.Contains(out, "removed") {
		t.Errorf("rm --force: %d %s %s", code, out, errOut)
	}
}

// A checkout git worktree remove would refuse — locked, or holding a
// submodule checked out — keeps finish from landing anything, as its refusal
// promises; rm names it, and rm --force passes the lock too.
func TestWorktreeFinishRefusesWhatGitWouldNotRemoveBeforeLanding(t *testing.T) {
	lab := newWorktreeLab(t)
	old := lab.git(t, lab.repo, "rev-parse", "main")

	locked := lab.made(t, "locked")
	lab.commitIn(t, locked.Path, "mine")
	lab.git(t, lab.repo, "worktree", "lock", "--reason", "kept by hand", locked.Path)
	if code, _, errOut := run("worktree", "finish", "locked"); code != ExitFailed || !strings.Contains(errOut, "locked with git worktree lock (kept by hand)") || !strings.Contains(errOut, "Nothing was landed") {
		t.Errorf("finish locked: %d %s", code, errOut)
	}
	if main := lab.git(t, lab.repo, "rev-parse", "main"); main != old {
		t.Errorf("a refused finish landed: main at %s", main)
	}
	if code, _, errOut := run("worktree", "rm", "locked"); code != ExitFailed || !strings.Contains(errOut, "git worktree unlock") {
		t.Errorf("rm locked: %d %s", code, errOut)
	}
	if code, out, errOut := run("worktree", "rm", "locked", "--force"); code != ExitOK || !strings.Contains(out, "removed") {
		t.Errorf("rm --force locked: %d %s %s", code, out, errOut)
	}

	library := filepath.Join(filepath.Dir(lab.repo), "library")
	lab.git(t, filepath.Dir(lab.repo), "init", "-q", "-b", "main", library)
	lab.git(t, library, "commit", "-q", "--allow-empty", "-m", "Library")
	lab.git(t, lab.repo, "-c", "protocol.file.allow=always", "submodule", "add", "-q", library, "library")
	lab.git(t, lab.repo, "commit", "-q", "-m", "Library")
	old = lab.git(t, lab.repo, "rev-parse", "main")
	nested := lab.made(t, "nested")
	lab.git(t, nested.Path, "-c", "protocol.file.allow=always", "submodule", "update", "-q", "--init")
	lab.commitIn(t, nested.Path, "mine")
	if code, _, errOut := run("worktree", "finish", "nested"); code != ExitFailed || !strings.Contains(errOut, "submodule") {
		t.Errorf("finish nested: %d %s", code, errOut)
	}
	if main := lab.git(t, lab.repo, "rev-parse", "main"); main != old {
		t.Errorf("a refused finish landed: main at %s", main)
	}
}

// After rm the branch line says what is true of the branch: kept because
// another checkout has it out, or gone already — never advice to merge a
// branch that is not there.
func TestWorktreeRmSaysWhyTheBranchStays(t *testing.T) {
	lab := newWorktreeLab(t)
	held := lab.made(t, "held")
	lab.git(t, held.Path, "switch", "-q", "--detach")
	elsewhere := filepath.Join(filepath.Dir(lab.repo), "elsewhere")
	lab.git(t, lab.repo, "worktree", "add", "-q", elsewhere, "held")
	code, out, errOut := run("worktree", "rm", "held")
	if code != ExitOK || !strings.Contains(out, "kept the branch held: it is checked out in "+elsewhere) || strings.Contains(out, "merge --ff-only") {
		t.Errorf("rm held: %d %s %s", code, out, errOut)
	}

	gone := lab.made(t, "gone")
	lab.git(t, gone.Path, "switch", "-q", "--detach")
	lab.git(t, lab.repo, "branch", "-q", "-D", "gone")
	code, out, errOut = run("worktree", "rm", "gone")
	if code != ExitOK || !strings.Contains(out, "no branch gone to delete: it was deleted already") || strings.Contains(out, "merge --ff-only") {
		t.Errorf("rm gone: %d %s %s", code, out, errOut)
	}
}

// A source that has the worktree's own branch checked out is a state to
// change, exit 1; --into= with no branch is a wrong call, exit 2, and lands
// nothing rather than taking the default.
func TestWorktreeLandTellsTheStateFromTheCall(t *testing.T) {
	lab := newWorktreeLab(t)
	record := lab.made(t, "mine")
	lab.commitIn(t, record.Path, "mine")
	old := lab.git(t, lab.repo, "rev-parse", "main")
	if code, _, errOut := run("worktree", "land", "mine", "--into="); code != ExitUsage || !strings.Contains(errOut, "--into= needs a branch name") {
		t.Errorf("--into=: %d %s", code, errOut)
	}
	if main := lab.git(t, lab.repo, "rev-parse", "main"); main != old {
		t.Errorf("--into= landed into main: %s", main)
	}
	lab.git(t, record.Path, "switch", "-q", "--detach")
	lab.git(t, lab.repo, "switch", "-q", "mine")
	if code, _, errOut := run("worktree", "land", "mine"); code != ExitFailed || !strings.Contains(errOut, "has the worktree's own branch mine checked out") {
		t.Errorf("source on the worktree's branch: %d %s", code, errOut)
	}
}
