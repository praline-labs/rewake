package worktree

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// A reviewer's checkout is detached at the commit main named, whatever the
// writer's branch does after, with no branch of its own; the same commit
// given again finds it, and a space never takes it for its worker's own.
func TestAReviewCheckoutIsDetachedAtTheNamedCommit(t *testing.T) {
	isolate(t)
	t.Setenv(state.DirEnv, t.TempDir())
	source := repository(t, "project")
	root := t.TempDir()
	must(t, source, "switch", "-q", "-c", "work")
	must(t, source, "commit", "-q", "--allow-empty", "-m", "Reviewed")
	commit, err := Resolve(source, "work")
	if err != nil {
		t.Fatal(err)
	}
	must(t, source, "commit", "-q", "--allow-empty", "-m", "The writer's next work")

	review := Review{Brief: "3", Reviewer: Owner{Name: "review-pi", Epoch: "123.456", Dir: "/rooms/r"}}
	record, err := CreateReview(root, source, commit, review)
	if err != nil {
		t.Fatal(err)
	}
	if head := must(t, record.Path, "rev-parse", "HEAD"); head != commit {
		t.Errorf("checked out %s, want %s", head, commit)
	}
	if branch := must(t, record.Path, "branch", "--show-current"); branch != "" {
		t.Errorf("on branch %q, want detached", branch)
	}
	if record.Session != nil || record.Branch != "" || record.Name != "review-3-review-pi-"+commit[:7] {
		t.Errorf("record %+v", record)
	}
	again, err := CreateReview(root, source, commit, review)
	if err != nil || again.Path != record.Path {
		t.Errorf("the same review again: %+v %v", again, err)
	}
	if _, err := Land(record, ""); err == nil {
		t.Error("a review checkout landed")
	}
	if _, err := Resolve(source, "no-such-branch"); err == nil {
		t.Error("a name of nothing resolved")
	}
}

// The end of a review removes its checkout, and keeps one holding changes.
func TestReleaseReviewsKeepsOnlyChanges(t *testing.T) {
	isolate(t)
	t.Setenv(state.DirEnv, t.TempDir())
	source := repository(t, "project")
	root := t.TempDir()
	commit := must(t, source, "rev-parse", "HEAD")
	clean, err := CreateReview(root, source, commit, Review{Brief: "1", Reviewer: Owner{Name: "a", Epoch: "1.1", Dir: "/rooms/r"}})
	if err != nil {
		t.Fatal(err)
	}
	changed, err := CreateReview(root, source, commit, Review{Brief: "1", Reviewer: Owner{Name: "b", Epoch: "2.2", Dir: "/rooms/r"}})
	if err != nil {
		t.Fatal(err)
	}
	other, err := CreateReview(root, source, commit, Review{Brief: "2", Reviewer: Owner{Name: "a", Epoch: "1.1", Dir: "/rooms/r"}})
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(changed.Path, "notes"), "the reviewer's\n")
	kept, err := ReleaseReviews(root, func(r Record) bool { return r.Review.Brief == "1" })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(clean.Path); !os.IsNotExist(err) {
		t.Errorf("the clean checkout stays: %v", err)
	}
	if _, err := os.Stat(changed.Path); err != nil || kept[changed.Path] == "" || len(kept) != 1 {
		t.Errorf("kept %v, want only %s with its reason", kept, changed.Path)
	}
	if _, err := os.Stat(other.Path); err != nil {
		t.Errorf("another brief's checkout went: %v", err)
	}
}
