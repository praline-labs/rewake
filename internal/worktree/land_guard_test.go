package worktree

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// change writes a file in a checkout and commits it, returning the new HEAD.
func change(t *testing.T, dir, file, text string) string {
	t.Helper()
	write(t, filepath.Join(dir, file), text)
	must(t, dir, "add", file)
	must(t, dir, "commit", "-q", "-m", "Change "+file)
	return must(t, dir, "rev-parse", "HEAD")
}

// A target checked out anywhere but the source is refused and nothing moves:
// the worktree itself after its worker switched to the target, and another
// checkout, whoever works there.
func TestLandRefusesATargetAnotherCheckoutHolds(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	record, err := Create(t.TempDir(), source, "work")
	if err != nil {
		t.Fatal(err)
	}
	change(t, record.Path, "mine", "mine\n")
	must(t, record.Path, "switch", "-q", "-c", "other", record.Commit)
	var blocked *StateError
	if _, err := Land(record, "other"); !errors.As(err, &blocked) || !strings.Contains(err.Error(), "worktree itself") {
		t.Errorf("--into the branch the worktree is on: %v", err)
	}
	if head := must(t, record.Path, "rev-parse", "HEAD"); head != record.Commit {
		t.Errorf("the worktree's HEAD moved to %s", head)
	}
	must(t, record.Path, "switch", "-q", "work")

	side := filepath.Join(t.TempDir(), "side")
	must(t, source, "worktree", "add", "-q", "-b", "side", side, record.Commit)
	if _, err := Land(record, "side"); !errors.As(err, &blocked) || !strings.Contains(err.Error(), side) {
		t.Errorf("--into a branch another checkout holds: %v", err)
	}
	if head := must(t, side, "rev-parse", "HEAD"); head != record.Commit {
		t.Errorf("the other checkout's HEAD moved to %s", head)
	}
	if _, err := os.Stat(filepath.Join(side, "mine")); !os.IsNotExist(err) {
		t.Errorf("the other checkout got the file: %v", err)
	}
}

// The source is the source whichever path names it: a record that reached it
// through a symbolic link still lands there.
func TestLandFindsTheSourceThroughALink(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	record, err := Create(t.TempDir(), source, "work")
	if err != nil {
		t.Fatal(err)
	}
	tip := change(t, record.Path, "mine", "mine\n")
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(source, link); err != nil {
		t.Fatal(err)
	}
	record.Source = link
	if landing, err := Land(record, ""); err != nil || landing.New != tip {
		t.Fatalf("land through a link: %+v, %v", landing, err)
	}
}

// A branch a rebase is working on is not moved: git lists that checkout as
// detached, and the rebase's last step would fail.
func TestLandRefusesABranchUnderRebase(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	record, err := Create(t.TempDir(), source, "work")
	if err != nil {
		t.Fatal(err)
	}
	rebasing := filepath.Join(t.TempDir(), "rebasing")
	must(t, source, "worktree", "add", "-q", "-b", "side", rebasing, record.Commit)
	sideTip := change(t, rebasing, "src/nested/file", "side\n")
	must(t, record.Path, "merge", "-q", "--ff-only", "side")
	change(t, record.Path, "mine", "mine\n")
	other := filepath.Join(t.TempDir(), "other")
	must(t, source, "worktree", "add", "-q", "--detach", other, record.Commit)
	change(t, other, "src/nested/file", "theirs\n")
	must(t, source, "branch", "-f", "onto", must(t, other, "rev-parse", "HEAD"))
	if _, err := gitOutput(rebasing, "rebase", "onto"); err == nil {
		t.Fatal("the rebase did not stop on its conflict")
	}
	var blocked *StateError
	if _, err := Land(record, "side"); !errors.As(err, &blocked) || !strings.Contains(err.Error(), "rebase") {
		t.Errorf("--into a branch under rebase: %v", err)
	}
	if tip := must(t, source, "rev-parse", "side"); tip != sideTip {
		t.Errorf("side moved to %s under its rebase", tip)
	}
}

// A branch a bisect started on is not moved either: the bisect goes back to it
// when it ends.
func TestLandRefusesABranchUnderBisect(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	for _, file := range []string{"a", "b", "c"} {
		change(t, source, file, file+"\n")
	}
	record, err := Create(t.TempDir(), source, "work")
	if err != nil {
		t.Fatal(err)
	}
	change(t, record.Path, "mine", "mine\n")
	must(t, source, "bisect", "start", "HEAD", "HEAD~3")
	var blocked *StateError
	if _, err := Land(record, "main"); !errors.As(err, &blocked) || !strings.Contains(err.Error(), "bisect") {
		t.Errorf("--into a branch under bisect: %v", err)
	}
	if tip := must(t, source, "rev-parse", "main"); tip != record.Commit {
		t.Errorf("main moved to %s under its bisect", tip)
	}
}

// A ref's tip is the ref's own, never one below its name.
func TestRefTipIsExact(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	must(t, source, "branch", "foo/bar")
	must(t, source, "branch", "refs/heads/zed")
	commonDir := filepath.Join(source, ".git")
	for _, ref := range []string{"refs/heads/foo", "refs/heads/zed"} {
		if tip, err := refTip(commonDir, ref); err != nil || tip != "" {
			t.Errorf("%s: %q, %v", ref, tip, err)
		}
	}
	if tip, err := refTip(commonDir, "refs/heads/foo/bar"); err != nil || tip == "" {
		t.Errorf("refs/heads/foo/bar: %q, %v", tip, err)
	}
}
