package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// runningAt publishes a live session of another run whose work is in cwd.
func runningAt(t *testing.T, dir, name, cwd string) {
	t.Helper()
	session := otherRun(t, dir, name)
	session.CWD = cwd
	_ = os.Remove(state.SessionPath(dir, name))
	if err := registry.Publish(dir, session); err != nil {
		t.Fatal(err)
	}
}

// A session started in a checkout after the one it was made for — by hand,
// in any room — holds it as its owner would; one next to it does not.
func TestASessionStartedInACheckoutHoldsIt(t *testing.T) {
	lab := newWorktreeLab(t)
	record := lab.made(t, "visited")
	elsewhere, _ := state.RoomDir(lab.state, "elsewhere")
	runningAt(t, elsewhere, "visitor", filepath.Join(record.Path, "src"))
	trees, _ := state.RoomDir(lab.state, "trees")
	runningAt(t, trees, "neighbor", record.Path+"-copy")

	code, _, errOut := run("worktree", "rm", "visited")
	if code != ExitFailed || !strings.Contains(errOut, "still run in it: visitor in room elsewhere") || strings.Contains(errOut, "neighbor") {
		t.Fatalf("rm with a visitor: %d %s", code, errOut)
	}
	if _, out, _ := run("worktree", "ls"); !strings.Contains(out, "visitor (running)") {
		t.Errorf("ls does not show the visitor:\n%s", out)
	}
	if records := lab.records(t); len(records) != 1 {
		t.Errorf("a refused rm removed it: %+v", records)
	}
}

// A checkout whose directory went missing may have been moved with its work;
// rm keeps git's entry for git worktree repair unless forced, and forced it
// takes out that entry alone.
func TestWorktreeRmKeepsAMissingCheckoutUnlessForced(t *testing.T) {
	lab := newWorktreeLab(t)
	moved := lab.made(t, "moved")
	other := lab.made(t, "other")
	for _, path := range []string{moved.Path, other.Path} {
		if err := os.Rename(path, path+".away"); err != nil {
			t.Fatal(err)
		}
	}
	code, _, errOut := run("worktree", "rm", "moved")
	if code != ExitFailed || !strings.Contains(errOut, "git worktree repair") || !strings.Contains(errOut, "--force") {
		t.Fatalf("rm of a missing checkout: %d %s", code, errOut)
	}
	if _, out, _ := run("worktree", "ls"); !strings.Contains(out, "missing") {
		t.Errorf("ls does not say missing:\n%s", out)
	}
	if code, _, errOut := run("worktree", "rm", "moved", "--force"); code != ExitOK {
		t.Fatalf("rm --force: %d %s", code, errOut)
	}
	listed := lab.git(t, lab.repo, "worktree", "list", "--porcelain")
	if strings.Contains(listed, "worktree "+moved.Path+"\n") || !strings.Contains(listed, "worktree "+other.Path+"\n") {
		t.Errorf("git lists:\n%s", listed)
	}
}

// A deleted repository leaves git nothing to say of its checkout; rm keeps
// the directory unless forced, as its refusal says, and forced removes it.
func TestWorktreeRmOfACheckoutWhoseRepositoryIsGone(t *testing.T) {
	lab := newWorktreeLab(t)
	record := lab.made(t, "orphan")
	if err := os.RemoveAll(lab.repo); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := run("worktree", "rm", "orphan")
	if code != ExitFailed || !strings.Contains(errOut, "is gone, so git cannot tell") || !strings.Contains(errOut, "--force") {
		t.Fatalf("rm without its repository: %d %s", code, errOut)
	}
	if code, _, errOut := run("worktree", "rm", "orphan", "--force"); code != ExitOK {
		t.Fatalf("rm --force: %d %s", code, errOut)
	}
	if _, err := os.Stat(record.Path); !os.IsNotExist(err) {
		t.Errorf("the directory stayed: %v", err)
	}
	if records := lab.records(t); len(records) != 0 {
		t.Errorf("records left: %+v", records)
	}
}

// Files git ignores are deleted by git worktree remove without a word, so rm
// keeps a checkout holding them.
func TestWorktreeRmKeepsIgnoredFiles(t *testing.T) {
	lab := newWorktreeLab(t)
	if err := os.WriteFile(filepath.Join(lab.repo, ".gitignore"), []byte(".env\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lab.git(t, lab.repo, "add", ".gitignore")
	lab.git(t, lab.repo, "commit", "-q", "-m", "Ignore .env")
	record := lab.made(t, "env")
	if err := os.WriteFile(filepath.Join(record.Path, ".env"), []byte("TOKEN=x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := run("worktree", "rm", "env"); code != ExitFailed || !strings.Contains(errOut, "files git ignores") {
		t.Fatalf("rm with a .env: %d %s", code, errOut)
	}
	if _, err := os.Stat(filepath.Join(record.Path, ".env")); err != nil {
		t.Errorf("the .env is gone: %v", err)
	}
}

// A harness that exits with an error — a resume of a conversation that does
// not exist, say — leaves a checkout it never used, and it is taken back;
// one it wrote in stays.
func TestAnErrorExitTakesAnUntouchedCheckoutBack(t *testing.T) {
	lab := newWorktreeLab(t)
	probe := codexProbe(t)
	probe.command = "/bin/false"
	told, err := lab.launchTold(t, probe, lab.repo, "--worktree=short")
	var exit *ExitCodeError
	if !errors.As(err, &exit) {
		t.Fatalf("the launch ended with %v", err)
	}
	if records := lab.records(t); len(records) != 0 {
		t.Errorf("the checkout stayed: %+v; told %q", records, told)
	}

	probe = codexProbe(t)
	probe.command = filepath.Join(t.TempDir(), "writes")
	if err := os.WriteFile(probe.command, []byte("#!/bin/sh\necho x > made\nexit 3\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := lab.launch(t, probe, lab.repo, "--worktree=wrote"); !errors.As(err, &exit) {
		t.Fatalf("the writing launch ended with %v", err)
	}
	if records := lab.records(t); len(records) != 1 || records[0].Name != "wrote" {
		t.Errorf("records: %+v", records)
	}
}

// A launch the registration refuses — its name taken — made a checkout for
// nothing: it is taken back, and the person is not told the session works
// there.
func TestATakenSessionNameLeavesNoCheckout(t *testing.T) {
	lab := newWorktreeLab(t)
	trees, _ := state.RoomDir(lab.state, "trees")
	otherRun(t, trees, "tree-codex")
	told, err := lab.launchTold(t, codexProbe(t), lab.repo, "--worktree=late")
	var usage *UsageError
	if !errors.As(err, &usage) || !strings.Contains(usage.Message, "tree-codex") {
		t.Fatalf("the launch ended with %v", err)
	}
	if records := lab.records(t); len(records) != 0 {
		t.Errorf("the checkout stayed: %+v", records)
	}
	if strings.Contains(told, "working in the worktree") {
		t.Errorf("told of a session that never ran: %q", told)
	}
}
