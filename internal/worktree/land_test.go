package worktree

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// commit adds a file to the checkout at dir and commits it, returning the new
// HEAD.
func commit(t *testing.T, dir, file string) string {
	t.Helper()
	write(t, filepath.Join(dir, file), file+"\n")
	must(t, dir, "add", file)
	must(t, dir, "commit", "-q", "-m", "Add "+file)
	return must(t, dir, "rev-parse", "HEAD")
}

// A name whose branch the repository has is refused before anything is made:
// the checkout would not start where the launch stands.
func TestANameWhoseBranchExistsIsRefused(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	must(t, source, "branch", "taken")
	root := t.TempDir()
	_, err := Create(root, source, "taken")
	var taken *BranchTakenError
	if !errors.As(err, &taken) || taken.Branch != "taken" {
		t.Fatalf("got %v", err)
	}
	if records, _ := List(root); len(records) != 0 {
		t.Errorf("a refused name left %+v", records)
	}
	// A checkout of the name is named as such, not as a branch.
	made, err := Create(root, source, "made")
	if err != nil {
		t.Fatal(err)
	}
	var exists *ExistsError
	if _, err := Create(root, source, "made"); !errors.As(err, &exists) || exists.Record.Path != made.Path {
		t.Errorf("second checkout of a name: %v", err)
	}
}

// land fast-forwards the branch checked out in the source, in the source, so
// its files follow; the hashes stay, and the checkout and its branch are left
// as they were. A second land with nothing new moves nothing.
func TestLandFastForwardsTheSourceBranch(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	record, err := Create(t.TempDir(), source, "work")
	if err != nil {
		t.Fatal(err)
	}
	commit(t, record.Path, "one")
	tip := commit(t, record.Path, "two")
	landing, err := Land(record, "")
	if err != nil {
		t.Fatal(err)
	}
	if landing.Target != "main" || landing.Old != record.Commit || landing.New != tip || landing.Commits != 2 || landing.Checkout != source {
		t.Errorf("landing %+v", landing)
	}
	if head := must(t, source, "rev-parse", "HEAD"); head != tip {
		t.Errorf("main at %s, want %s", head, tip)
	}
	if status := must(t, source, "status", "--porcelain"); status != "" {
		t.Errorf("the source is not at its new commit:\n%s", status)
	}
	if branch, _, _ := currentBranch(record.Path); branch != "work" || must(t, record.Path, "rev-parse", "HEAD") != tip {
		t.Errorf("the checkout moved: %s", branch)
	}
	again, err := Land(record, "")
	if err != nil || again.Commits != 0 || again.Old != tip || again.New != tip {
		t.Errorf("second land: %+v, %v", again, err)
	}
	next := commit(t, record.Path, "three")
	if later, err := Land(record, ""); err != nil || later.Commits != 1 || later.New != next {
		t.Errorf("land after more work: %+v, %v", later, err)
	}
}

// A target that moved on since the branch left it is refused: no rebase, no
// merge commit, nothing moved.
func TestLandRefusesATargetThatMovedOn(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	record, err := Create(t.TempDir(), source, "work")
	if err != nil {
		t.Fatal(err)
	}
	commit(t, record.Path, "mine")
	theirs := commit(t, source, "theirs")
	_, err = Land(record, "")
	var diverged *DivergedError
	if !errors.As(err, &diverged) || diverged.Target != "main" || diverged.Branch != "work" {
		t.Fatalf("got %v", err)
	}
	if head := must(t, source, "rev-parse", "HEAD"); head != theirs {
		t.Errorf("main moved to %s", head)
	}
}

// A target checked out nowhere has only its ref moved, and --into names it.
func TestLandIntoABranchCheckedOutNowhere(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	must(t, source, "branch", "release")
	record, err := Create(t.TempDir(), source, "work")
	if err != nil {
		t.Fatal(err)
	}
	tip := commit(t, record.Path, "fix")
	landing, err := Land(record, "release")
	if err != nil {
		t.Fatal(err)
	}
	if landing.Target != "release" || landing.Checkout != "" || landing.Commits != 1 || landing.New != tip {
		t.Errorf("landing %+v", landing)
	}
	if got := must(t, source, "rev-parse", "release"); got != tip {
		t.Errorf("release at %s", got)
	}
	if got := must(t, source, "rev-parse", "main"); got != record.Commit {
		t.Errorf("main moved to %s", got)
	}
	for _, into := range []string{"missing", "m*", "work", "-x"} {
		var unusable *UnusableError
		if _, err := Land(record, into); !errors.As(err, &unusable) {
			t.Errorf("--into %s: %v", into, err)
		}
	}
}

// Changes in the source the fast-forward would overwrite stop it, with git's
// reason; changes elsewhere in the source do not.
func TestLandPassesOnGitsRefusalInADirtySource(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	record, err := Create(t.TempDir(), source, "work")
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(record.Path, "src", "nested", "file"), "two\n")
	must(t, record.Path, "commit", "-q", "-am", "Change file")
	write(t, filepath.Join(source, "src", "nested", "file"), "local\n")
	_, err = Land(record, "")
	var refused *RefusedError
	if !errors.As(err, &refused) || refused.Checkout != source || !strings.Contains(refused.Reason, "src/nested/file") {
		t.Fatalf("got %v", err)
	}
	if got := must(t, source, "rev-parse", "HEAD"); got != record.Commit {
		t.Errorf("main moved to %s", got)
	}
	must(t, source, "checkout", "--", ".")
	write(t, filepath.Join(source, "elsewhere"), "untouched\n")
	if _, err := Land(record, ""); err != nil {
		t.Errorf("with changes the merge does not touch: %v", err)
	}
}

// A source with no branch checked out gives land no target of its own.
func TestLandNeedsATargetFromADetachedSource(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	record, err := Create(t.TempDir(), source, "work")
	if err != nil {
		t.Fatal(err)
	}
	must(t, source, "switch", "-q", "--detach")
	var unusable *UnusableError
	if _, err := Land(record, ""); !errors.As(err, &unusable) || !strings.Contains(err.Error(), "--into") {
		t.Errorf("got %v", err)
	}
	legacy := record
	legacy.Branch = ""
	if _, err := Land(legacy, "main"); !errors.As(err, &unusable) || !strings.Contains(err.Error(), "detached") {
		t.Errorf("a record without a branch: %v", err)
	}
}

// DropBranch deletes a branch another ref holds, and keeps one with commits
// only it has.
func TestDropBranchKeepsWorkOnlyItHolds(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	record, err := Create(t.TempDir(), source, "work")
	if err != nil {
		t.Fatal(err)
	}
	commit(t, record.Path, "mine")
	if dropped, err := DropBranch(record); err != nil || dropped {
		t.Errorf("checked out: %v, %v", dropped, err)
	}
	if err := Remove(record, false); err != nil {
		t.Fatal(err)
	}
	if dropped, err := DropBranch(record); err != nil || dropped {
		t.Errorf("with commits only it holds: %v, %v", dropped, err)
	}
	must(t, source, "merge", "-q", "--ff-only", "work")
	if dropped, err := DropBranch(record); err != nil || !dropped {
		t.Errorf("held by main: %v, %v", dropped, err)
	}
	if tip, _ := refTip(record.CommonDir, "refs/heads/work"); tip != "" {
		t.Errorf("work is still at %s", tip)
	}
}
