package cli

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/state"
	"github.com/iiiokojiadbi/rewake/internal/worktree"
)

// worktreeProbe is a harness that takes its worktree flag as Codex does, and
// remembers where and with what it was launched.
type worktreeProbe struct {
	roleLaunchProbe
	cwd  string
	fail bool
}

func (p *worktreeProbe) Launch(request harness.LaunchRequest) (harness.LaunchPlan, error) {
	p.cwd, _ = os.Getwd()
	if p.fail {
		return harness.LaunchPlan{}, errors.New("the harness would not start")
	}
	return p.roleLaunchProbe.Launch(request)
}

func (p *worktreeProbe) WorktreeFlag() string { return p.taker().WorktreeFlag() }

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
	output, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
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
func (worktreeLab) launch(t *testing.T, probe *worktreeProbe, dir string, raw ...string) error {
	t.Helper()
	t.Chdir(dir)
	parsed, err := parse(append([]string{"--room", "trees", "--name", "tree", "codex"}, raw...))
	if err != nil {
		t.Fatal(err)
	}
	return handleLaunch(probe)(&Context{Stdout: io.Discard, Stderr: io.Discard}, parsed.Call)
}

func codexProbe(t *testing.T) *worktreeProbe {
	t.Helper()
	codex, ok := harness.Find("codex")
	if !ok {
		t.Fatal("codex is not in the catalog")
	}
	return &worktreeProbe{roleLaunchProbe: roleLaunchProbe{Harness: codex}}
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
		{lab.repo, []string{"--worktree=bad.name"}, "not usable"},
		{lab.repo, []string{"--worktree=taken"}, "rewake worktree rm"},
		{outside, []string{"--worktree"}, "not in a Git working tree"},
		{lab.repo, []string{"--worktree", "-C", "missing"}, "missing"},
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
}

// Only a harness that cannot make its own worktree under rewake gives the flag
// to rewake; Claude Code keeps its own -w/--worktree.
func TestOnlyCodexHandsItsWorktreeFlagToRewake(t *testing.T) {
	for _, h := range harness.All() {
		_, takes := h.(harness.WorktreeHarness)
		if takes != (h.ID() == "codex") {
			t.Errorf("%s: takes the worktree flag %v", h.ID(), takes)
		}
	}
}
