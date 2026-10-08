package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/state"
	"github.com/praline-labs/rewake/internal/worktree"
)

// worktreeProbe is a harness that takes its worktree flag as Codex does, and
// remembers where and with what it was launched.
type worktreeProbe struct {
	roleLaunchProbe
	cwd  string
	fail bool
	// command replaces the harness program: /bin/false for one that exits
	// with an error.
	command string
}

func (p *worktreeProbe) Launch(request harness.LaunchRequest) (harness.LaunchPlan, error) {
	p.cwd, _ = os.Getwd()
	if p.fail {
		return harness.LaunchPlan{}, errors.New("the harness would not start")
	}
	plan, err := p.roleLaunchProbe.Launch(request)
	if p.command != "" {
		plan.Command = p.command
	}
	return plan, err
}

func (p *worktreeProbe) WorktreeFlag() string { return p.taker().WorktreeFlag() }

func (p *worktreeProbe) WorktreeRefusal(args []string) error {
	return p.taker().WorktreeRefusal(args)
}

func (p *worktreeProbe) WorktreeNameSpaced() bool { return p.taker().WorktreeNameSpaced() }

func (p *worktreeProbe) LaunchDirectory(args []string) (string, []string, error) {
	return p.taker().LaunchDirectory(args)
}

func (p *worktreeProbe) taker() harness.WorktreeHarness {
	return p.Harness.(harness.WorktreeHarness)
}

// worktreeLab is a repository with src/nested/file at one commit, a state
// directory and a worktree root of this test's own.
type worktreeLab struct {
	repo, state, root string
}

func newWorktreeLab(t *testing.T) worktreeLab {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	for key, value := range map[string]string{
		"GIT_CONFIG_GLOBAL": os.DevNull, "GIT_CONFIG_SYSTEM": os.DevNull,
		"GIT_AUTHOR_NAME": "Test", "GIT_AUTHOR_EMAIL": "test@example.invalid",
		"GIT_COMMITTER_NAME": "Test", "GIT_COMMITTER_EMAIL": "test@example.invalid",
	} {
		t.Setenv(key, value)
	}
	isolateAliases(t)
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	lab := worktreeLab{repo: filepath.Join(base, "project"), state: filepath.Join(base, "state"), root: filepath.Join(base, "trees")}
	if err := os.MkdirAll(filepath.Join(lab.repo, "src", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lab.repo, "src", "nested", "file"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "."}, {"commit", "-q", "-m", "First"}} {
		lab.git(t, lab.repo, args...)
	}
	if err := os.Mkdir(lab.state, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(state.DirEnv, lab.state)
	t.Setenv(worktree.RootEnv, lab.root)
	return lab
}

func (worktreeLab) git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	// Started in dir, not in the test's own directory, which a failed
	// launch may have removed: a shell wrapper in front of git complains
	// about a missing working directory into the output read here.
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %q: %v: %s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func (lab worktreeLab) records(t *testing.T) []worktree.Record {
	t.Helper()
	records, err := worktree.List(lab.root)
	if err != nil {
		t.Fatal(err)
	}
	return records
}

// launch runs one launch of the probe from dir with the arguments after the
// harness word.
func (lab worktreeLab) launch(t *testing.T, probe *worktreeProbe, dir string, raw ...string) error {
	t.Helper()
	_, err := lab.launchTold(t, probe, dir, raw...)
	return err
}

// launchTold is launch with what rewake told the person on stderr.
func (worktreeLab) launchTold(t *testing.T, probe *worktreeProbe, dir string, raw ...string) (string, error) {
	t.Helper()
	t.Chdir(dir)
	parsed, err := parse(append([]string{"--room", "trees", "--name", "tree", probe.ID()}, raw...))
	if err != nil {
		t.Fatal(err)
	}
	var stderr strings.Builder
	err = handleLaunch(probe)(&Context{Stdout: io.Discard, Stderr: &stderr}, parsed.Call)
	return stderr.String(), err
}

func codexProbe(t *testing.T) *worktreeProbe { return harnessProbe(t, "codex") }

func harnessProbe(t *testing.T, id string) *worktreeProbe {
	t.Helper()
	found, ok := harness.Find(id)
	if !ok {
		t.Fatalf("%s is not in the catalog", id)
	}
	return &worktreeProbe{roleLaunchProbe: roleLaunchProbe{Harness: found}}
}

// The launch starts in the checkout, at the place within the repository it
// was made from; the flag is rewake's and does not reach the harness, while
// the same word after "--" is the harness's text and stays.
func TestAWorktreeLaunchStartsInTheCheckout(t *testing.T) {
	lab := newWorktreeLab(t)
	probe := codexProbe(t)
	if err := lab.launch(t, probe, filepath.Join(lab.repo, "src", "nested"), "--worktree=fix", "--model", "m", "--", "--worktree"); err != nil {
		t.Fatal(err)
	}
	records := lab.records(t)
	if len(records) != 1 || records[0].Name != "fix" {
		t.Fatalf("records: %+v", records)
	}
	record := records[0]
	if probe.cwd != filepath.Join(record.Path, "src", "nested") {
		t.Errorf("launched in %s, want the checkout's src/nested under %s", probe.cwd, record.Path)
	}
	if want := []string{"--model", "m", "--", "--worktree"}; !reflect.DeepEqual(probe.request.Args, want) {
		t.Errorf("harness got %q, want %q", probe.request.Args, want)
	}
	owner := record.Session
	roomDir, _ := state.RoomDir(lab.state, "trees")
	if owner == nil || owner.Name != "tree-codex" || owner.Room != "trees" || owner.Dir != roomDir || owner.Epoch == "" || owner.Harness != "codex" {
		t.Errorf("owner %+v", owner)
	}
	if record.Commit != lab.git(t, lab.repo, "rev-parse", "HEAD") {
		t.Errorf("made at %s", record.Commit)
	}
}

// -C chooses the directory the checkout is made from and where in it the
// launch starts; it is taken out, since it would lead back to the source.
func TestAWorktreeLaunchFollowsTheDirectoryFlag(t *testing.T) {
	lab := newWorktreeLab(t)
	probe := codexProbe(t)
	if err := lab.launch(t, probe, lab.repo, "--worktree", "-C", "src/nested", "prompt"); err != nil {
		t.Fatal(err)
	}
	records := lab.records(t)
	if len(records) != 1 || probe.cwd != filepath.Join(records[0].Path, "src", "nested") {
		t.Fatalf("launched in %s, records %+v", probe.cwd, records)
	}
	if want := []string{"prompt"}; !reflect.DeepEqual(probe.request.Args, want) {
		t.Errorf("harness got %q, want %q", probe.request.Args, want)
	}
}

// Every refusal is a wrong call, exit 2, and leaves no checkout behind; a
// taken name says how to go on.
func TestWorktreeLaunchRefusals(t *testing.T) {
	lab := newWorktreeLab(t)
	if err := lab.launch(t, codexProbe(t), lab.repo, "--worktree=taken"); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	for _, c := range []struct {
		dir  string
		args []string
		want string
	}{
		{lab.repo, []string{"--worktree="}, "needs a name"},
		{lab.repo, []string{"--worktree", "--worktree=a"}, "given twice"},
		{lab.repo, []string{"--worktree=bad+name"}, "not usable"},
		{lab.repo, []string{"--worktree=taken"}, "rewake worktree rm"},
		{lab.repo, []string{"--worktree=main"}, "already has a branch main"},
		{lab.repo, []string{"--worktree=HEAD"}, "not usable"},
		{outside, []string{"--worktree"}, "not in a Git working tree"},
		{lab.repo, []string{"--worktree", "-C", "missing"}, "missing"},
		{lab.repo, []string{"--worktree", "resume", "--last"}, "Start a new conversation with --worktree"},
		{lab.repo, []string{"--worktree", "resume", "--last"}, "rewake worktree ls names the worktree's path; cd there and run rewake codex resume without --worktree"},
		{lab.repo, []string{"--worktree=a", "fork", "0199"}, "fork continues one in the directory it was started in"},
		{lab.repo, []string{"--worktree", "--remote", "ws://h"}, "--remote"},
		{lab.repo, []string{"--worktree", "--profile", "p"}, "--profile"},
	} {
		err := lab.launch(t, codexProbe(t), c.dir, c.args...)
		var usage *UsageError
		if !errors.As(err, &usage) || !strings.Contains(usage.Message, c.want) {
			t.Errorf("%q: %v, want a refusal saying %q", c.args, err, c.want)
		}
		if records := lab.records(t); len(records) != 1 {
			t.Errorf("%q left %d checkouts", c.args, len(records))
		}
	}
}

// A launch that fails takes its fresh checkout back: it was made for this
// launch alone and holds nothing.
func TestAFailedLaunchTakesItsCheckoutBack(t *testing.T) {
	lab := newWorktreeLab(t)
	probe := codexProbe(t)
	probe.fail = true
	if err := lab.launch(t, probe, lab.repo, "--worktree=short"); err == nil {
		t.Fatal("the failing launch succeeded")
	}
	if records := lab.records(t); len(records) != 0 {
		t.Errorf("the checkout of a failed launch stayed: %+v", records)
	}
	if listed := lab.git(t, lab.repo, "worktree", "list", "--porcelain"); strings.Contains(listed, lab.root) {
		t.Errorf("git still knows it:\n%s", listed)
	}
	if branches := lab.git(t, lab.repo, "branch", "--list", "short"); branches != "" {
		t.Errorf("its branch stayed: %s", branches)
	}
}

// A failed launch whose harness left something in the checkout keeps it, and
// says why, where it is and how to go on there.
func TestAFailedLaunchKeepsATouchedCheckoutAndSaysSo(t *testing.T) {
	lab := newWorktreeLab(t)
	probe := codexProbe(t)
	probe.command = filepath.Join(t.TempDir(), "harness")
	if err := os.WriteFile(probe.command, []byte("#!/bin/sh\ntouch left-behind\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	told, _ := lab.launchTold(t, probe, lab.repo, "--worktree=touched")
	records := lab.records(t)
	if len(records) != 1 {
		t.Fatalf("records: %+v", records)
	}
	path := records[0].Path
	for _, want := range []string{"the launch failed", "has changes", "cd " + path + " and run rewake codex without --worktree", "rewake worktree land"} {
		if !strings.Contains(told, want) {
			t.Errorf("told %q, want %q in it", told, want)
		}
	}
}

// Both harnesses hand --worktree to rewake. Claude Code's -w stays its own:
// it reaches the harness untouched and makes no checkout of rewake's, and the
// two together are refused.
//
// The launches in this file prove both do. Here, every harness that takes a
// worktree flag spells it --worktree; a harness may take none, as the fixture
// does.
func TestClaudeHandsItsLongWorktreeFlagToRewake(t *testing.T) {
	for _, h := range harness.All() {
		if taker, ok := h.(harness.WorktreeHarness); ok && taker.WorktreeFlag() != "--worktree" {
			t.Errorf("%s takes %s, not --worktree", h.ID(), taker.WorktreeFlag())
		}
	}
	lab := newWorktreeLab(t)
	probe := harnessProbe(t, "claude")
	if err := lab.launch(t, probe, filepath.Join(lab.repo, "src", "nested"), "--worktree=fix", "--model", "m"); err != nil {
		t.Fatal(err)
	}
	records := lab.records(t)
	if len(records) != 1 || records[0].Branch != "fix" || probe.cwd != filepath.Join(records[0].Path, "src", "nested") {
		t.Fatalf("launched in %s, records %+v", probe.cwd, records)
	}
	if want := []string{"--model", "m"}; !reflect.DeepEqual(probe.request.Args, want) {
		t.Errorf("harness got %q, want %q", probe.request.Args, want)
	}
	native := harnessProbe(t, "claude")
	if err := lab.launch(t, native, lab.repo, "-w", "own"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"-w", "own"}; !reflect.DeepEqual(native.request.Args, want) || native.cwd != lab.repo || len(lab.records(t)) != 1 {
		t.Errorf("-w: harness got %q in %s", native.request.Args, native.cwd)
	}
	for _, args := range [][]string{
		{"--worktree", "-w"},
		{"--worktree", "--tmux"},
		{"--worktree", "--continue"},
		{"--worktree", "-c"},
		{"--worktree", "-pc"},
		{"--worktree", "--resume", "0199"},
		{"--worktree", "-r0199"},
		{"--worktree=a", "--resume=0199", "--fork-session"},
		{"--worktree", "--from-pr", "12"},
		{"--worktree", "--teleport"},
		{"--worktree", "-pw"},
		{"--worktree", "-prID"},
		{"--worktree", "--cloud"},
		{"--worktree", "--cloud=0199"},
		{"--worktree", "--environment", "env_01"},
		{"--worktree", "--environment=env_01"},
		{"--worktree", "attach", "a1b2"},
		{"--worktree", "--model", "m", "respawn"},
	} {
		err := lab.launch(t, harnessProbe(t, "claude"), lab.repo, args...)
		var usage *UsageError
		if !errors.As(err, &usage) {
			t.Errorf("%q: %v, want a refusal", args, err)
		}
		if records := lab.records(t); len(records) != 1 {
			t.Errorf("%q left %d checkouts", args, len(records))
		}
	}
	// A prompt after --, a value of -d or -n that holds c, and the id of a new
	// conversation are not continuations.
	for index, args := range [][]string{
		{"--", "--continue"},
		{"--", "attach"},
		{"-dcache"},
		{"-ncircle"},
		{"--session-id", "0199"},
	} {
		named := append([]string{fmt.Sprintf("--worktree=kept-%d", index)}, args...)
		if err := lab.launch(t, harnessProbe(t, "claude"), lab.repo, named...); err != nil {
			t.Errorf("%q: %v", named, err)
		}
	}
}

// Claude Code's own flag is --worktree [name], so a word after rewake's is the
// name the person meant: refused with the spelling that names it, where it
// once made a checkout of a generated name and sent the word as the prompt.
// Codex's own flag is a switch, and there the word stays the prompt.
func TestClaudeWorktreeNameAfterASpaceIsRefused(t *testing.T) {
	lab := newWorktreeLab(t)
	err := lab.launch(t, harnessProbe(t, "claude"), lab.repo, "--worktree", "fix-login")
	var usage *UsageError
	if !errors.As(err, &usage) || !strings.Contains(usage.Message, "--worktree=fix-login") || !strings.Contains(usage.Message, `--worktree -- "fix-login"`) {
		t.Fatalf("got %v, want a refusal naming --worktree=fix-login", err)
	}
	if records := lab.records(t); len(records) != 0 {
		t.Fatalf("a refused launch left %d checkouts", len(records))
	}
	for _, args := range [][]string{{"--worktree", "--", "fix the login"}, {"--worktree", "--model", "m"}} {
		if err := lab.launch(t, harnessProbe(t, "claude"), lab.repo, args...); err != nil {
			t.Errorf("%q: %v", args, err)
		}
	}
	if err := lab.launch(t, codexProbe(t), lab.repo, "--worktree", "fix the login"); err != nil {
		t.Errorf("codex took its prompt for a name: %v", err)
	}
}
