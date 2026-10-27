package reviewcheckout

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/core/taskbrief"
	"github.com/praline-labs/rewake/internal/host/worktree"
	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
)

var main = taskbrief.Author{Name: "main-one", Epoch: "100.1.boot"}

// scene is a private room holding brief 1, and a real repository with two
// commits to review.
type scene struct {
	home     taskbrief.Home
	repo     string
	first    string
	second   string
	reviewer Reviewer
}

func newScene(t *testing.T) scene {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	for name, value := range map[string]string{
		"GIT_CONFIG_GLOBAL": os.DevNull, "GIT_CONFIG_SYSTEM": os.DevNull,
		"GIT_AUTHOR_NAME": "Test", "GIT_AUTHOR_EMAIL": "test@example.invalid",
		"GIT_COMMITTER_NAME": "Test", "GIT_COMMITTER_EMAIL": "test@example.invalid",
		worktree.RootEnv: t.TempDir(),
	} {
		t.Setenv(name, value)
	}
	base := t.TempDir()
	if err := os.Chmod(base, 0o700); err != nil {
		t.Fatal(err)
	}
	dir := buildtest.Room(t, base, "room")
	home := taskbrief.Home{Room: dir}
	if _, err := taskbrief.New(home, main, "# Review me\n\n- [ ] one\n", ""); err != nil {
		t.Fatal(err)
	}
	repo := t.TempDir()
	git := func(args ...string) string {
		out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q", "-b", "v2")
	git("commit", "-q", "--allow-empty", "-m", "First")
	first := git("rev-parse", "HEAD")
	git("commit", "-q", "--allow-empty", "-m", "Second")
	return scene{
		home: home, repo: repo, first: first, second: git("rev-parse", "HEAD"),
		reviewer: Reviewer{Name: "review-one", Room: "room", Epoch: "300.3.boot", Harness: "fixture", Dir: dir},
	}
}

func (s scene) bound(t *testing.T) taskbrief.Binding {
	t.Helper()
	brief, err := taskbrief.Load(s.home, 1)
	if err != nil {
		t.Fatal(err)
	}
	binding, ok := brief.BindingOf(taskbrief.Author{Name: s.reviewer.Name, Epoch: s.reviewer.Epoch})
	if !ok {
		t.Fatal("the reviewer is bound to nothing")
	}
	return binding
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// Giving a brief makes the checkout, detached at the commit, and binds the
// reviewer to it.
func TestGiveMakesTheCheckoutAndBindsIt(t *testing.T) {
	s := newScene(t)
	kept, err := Give(s.home, 1, s.repo, s.first, s.reviewer)
	if err != nil || kept.Err != nil || len(kept.Kept) != 0 {
		t.Fatalf("Give: %+v %v", kept, err)
	}
	binding := s.bound(t)
	if binding.Kind.Of() != taskbrief.Review || binding.Commit != s.first || !exists(binding.Checkout) {
		t.Errorf("binding %+v", binding)
	}
}

// A second commit replaces the first: the new checkout is bound, the old one,
// holding nothing, is removed.
func TestGiveReplacesThePreviousCheckout(t *testing.T) {
	s := newScene(t)
	if _, err := Give(s.home, 1, s.repo, s.first, s.reviewer); err != nil {
		t.Fatal(err)
	}
	old := s.bound(t).Checkout
	if _, err := Give(s.home, 1, s.repo, s.second, s.reviewer); err != nil {
		t.Fatal(err)
	}
	now := s.bound(t)
	if now.Checkout == old || now.Commit != s.second || !exists(now.Checkout) || exists(old) {
		t.Errorf("old %s (there: %v), now %+v", old, exists(old), now)
	}
}

// A previous checkout holding changes stays, and the outcome says why.
func TestGiveKeepsAPreviousCheckoutHoldingChanges(t *testing.T) {
	s := newScene(t)
	if _, err := Give(s.home, 1, s.repo, s.first, s.reviewer); err != nil {
		t.Fatal(err)
	}
	old := s.bound(t).Checkout
	if err := os.WriteFile(filepath.Join(old, "notes.txt"), []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	outcome, err := Give(s.home, 1, s.repo, s.second, s.reviewer)
	if err != nil || outcome.Kept[old] == "" || !exists(old) {
		t.Fatalf("Give: %+v %v", outcome, err)
	}
	if paths := outcome.Paths(); len(paths) != 1 || paths[0] != old {
		t.Errorf("paths %v", paths)
	}
}

// A brief that is closed makes no checkout.
func TestGiveOfAClosedBriefMakesNoCheckout(t *testing.T) {
	s := newScene(t)
	if _, err := taskbrief.Close(s.home, 1); err != nil {
		t.Fatal(err)
	}
	_, err := Give(s.home, 1, s.repo, s.first, s.reviewer)
	var closed *taskbrief.ClosedError
	if !errors.As(err, &closed) {
		t.Fatalf("Give: %v", err)
	}
	root, _ := worktree.Root()
	if records, _ := worktree.List(root); len(records) != 0 {
		t.Errorf("checkouts %+v", records)
	}
}

// The step that failed is named, for a caller that words each differently.
func TestGiveNamesTheFailedStep(t *testing.T) {
	s := newScene(t)
	_, err := Give(s.home, 1, s.repo, "no-such", s.reviewer)
	var step *StepError
	if !errors.As(err, &step) || step.Step != Commit {
		t.Fatalf("Give: %v", err)
	}
}

// A bind that fails takes the checkout just made back: no checkout is left
// bound to nothing.
func TestGiveTakesTheCheckoutBackWhenTheBindFails(t *testing.T) {
	s := newScene(t)
	var dirs []string
	_ = filepath.WalkDir(filepath.Join(s.home.Room, "briefs"), func(path string, entry fs.DirEntry, err error) error {
		if err == nil && entry.IsDir() {
			dirs = append(dirs, path)
		}
		return nil
	})
	for _, dir := range dirs {
		if err := os.Chmod(dir, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	}
	if _, err := os.OpenFile(filepath.Join(dirs[len(dirs)-1], "probe"), os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
		t.Skip("directories stay writable here (running as root)")
	}
	_, err := Give(s.home, 1, s.repo, s.first, s.reviewer)
	if err == nil {
		t.Fatal("Give succeeded with the brief unwritable")
	}
	root, _ := worktree.Root()
	if records, _ := worktree.List(root); len(records) != 0 {
		t.Errorf("a checkout was left after the failed bind: %+v", records)
	}
}

// A brief's close releases the checkouts given for it, and only those of the
// brief in the room.
func TestReleaseBriefReleasesThatBriefsCheckouts(t *testing.T) {
	s := newScene(t)
	if _, err := Give(s.home, 1, s.repo, s.first, s.reviewer); err != nil {
		t.Fatal(err)
	}
	path := s.bound(t).Checkout
	if other := ReleaseBrief(filepath.Join(s.home.Room, "elsewhere"), 1); len(other.Kept) != 0 || !exists(path) {
		t.Fatalf("another room's release took it: %+v", other)
	}
	if other := ReleaseBrief(s.home.Room, 2); !exists(path) || other.Err != nil {
		t.Fatalf("another brief's release took it: %+v", other)
	}
	if outcome := ReleaseBrief(s.home.Room, 1); outcome.Err != nil || len(outcome.Kept) != 0 || exists(path) {
		t.Errorf("outcome %+v, checkout there: %v", outcome, exists(path))
	}
}

// A run's end releases the checkouts given to that run, not another's, and
// reports one that holds changes as staying.
func TestReleaseRunReleasesThatRunsCheckouts(t *testing.T) {
	s := newScene(t)
	if _, err := Give(s.home, 1, s.repo, s.first, s.reviewer); err != nil {
		t.Fatal(err)
	}
	path := s.bound(t).Checkout
	if outcome := ReleaseRun(s.home.Room, "999.9.boot"); !exists(path) || len(outcome.Kept) != 0 {
		t.Fatalf("another run's end took it: %+v", outcome)
	}
	if outcome := ReleaseRun(s.home.Room, ""); !exists(path) || outcome.Err != nil {
		t.Fatalf("an empty run took it: %+v", outcome)
	}
	if err := os.WriteFile(filepath.Join(path, "notes.txt"), []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	if outcome := ReleaseRun(s.home.Room, s.reviewer.Epoch); outcome.Kept[path] == "" || !exists(path) {
		t.Fatalf("a checkout holding changes: %+v", outcome)
	}
	if err := os.Remove(filepath.Join(path, "notes.txt")); err != nil {
		t.Fatal(err)
	}
	if outcome := ReleaseRun(s.home.Room, s.reviewer.Epoch); len(outcome.Kept) != 0 || exists(path) {
		t.Errorf("outcome %+v, checkout there: %v", outcome, exists(path))
	}
}

// A root that cannot be read reaches the caller instead of being dropped.
func TestAReleaseWithNoRootSaysSo(t *testing.T) {
	t.Setenv(worktree.RootEnv, "relative/root")
	if outcome := ReleaseBrief("/room", 1); outcome.Err == nil {
		t.Errorf("outcome %+v", outcome)
	}
}
