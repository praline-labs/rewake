package worktree

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// What removing a checkout would lose is what Inspect reports: changes git
// status shows, and commits made on its detached HEAD that nothing else holds.
func TestInspectSaysWhatWouldBeLost(t *testing.T) {
	isolate(t)
	root := t.TempDir()
	record, err := Create(root, repository(t, "project"), "work")
	if err != nil {
		t.Fatal(err)
	}
	if check, err := Inspect(record); err != nil || check.Dirty() || check.Missing || check.Head != record.Commit {
		t.Fatalf("fresh: %+v, %v", check, err)
	}
	write(t, filepath.Join(record.Path, "new"), "untracked\n")
	if check, _ := Inspect(record); !check.Changes || check.Unreachable {
		t.Errorf("untracked file: %+v", check)
	}
	must(t, record.Path, "add", "new")
	must(t, record.Path, "commit", "-q", "-m", "Detached work")
	if check, _ := Inspect(record); check.Changes || !check.Unreachable || check.Head == record.Commit {
		t.Errorf("detached commit: %+v", check)
	}
	must(t, record.Path, "branch", "kept")
	if check, _ := Inspect(record); check.Dirty() {
		t.Errorf("commit on a branch: %+v", check)
	}
	if err := os.RemoveAll(record.Path); err != nil {
		t.Fatal(err)
	}
	if check, err := Inspect(record); err != nil || !check.Missing {
		t.Errorf("gone: %+v, %v", check, err)
	}
}

// Remove goes through git, so the repository forgets the checkout, and the
// record and the repository's empty directory go with it.
func TestRemoveTakesTheCheckoutAndItsRecord(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	root := t.TempDir()
	record, err := Create(root, source, "done")
	if err != nil {
		t.Fatal(err)
	}
	if err := Remove(record, false); err != nil {
		t.Fatal(err)
	}
	if listed := must(t, source, "worktree", "list", "--porcelain"); strings.Contains(listed, record.Path) {
		t.Errorf("git still knows it:\n%s", listed)
	}
	if _, err := os.Stat(filepath.Join(root, record.Repository)); !os.IsNotExist(err) {
		t.Errorf("the repository's directory stayed: %v", err)
	}
}

// Without force, git itself keeps a checkout with changes; with it, the
// checkout goes. The command asks Inspect first, and this is the floor under it.
func TestRemoveKeepsChangesUnlessForced(t *testing.T) {
	isolate(t)
	root := t.TempDir()
	record, err := Create(root, repository(t, "project"), "dirty")
	if err != nil {
		t.Fatal(err)
	}
	other, err := Create(root, record.Source, "other")
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(record.Path, "src", "nested", "file"), "changed\n")
	if err := Remove(record, false); err == nil {
		t.Fatal("a checkout with changes was removed without force")
	}
	if records, _ := List(root); len(records) != 2 {
		t.Fatalf("a refused removal lost its record: %+v", records)
	}
	if err := Remove(record, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(record.Path); !os.IsNotExist(err) {
		t.Errorf("forced removal left the checkout: %v", err)
	}
	if _, err := os.Stat(other.Path); err != nil {
		t.Errorf("the other checkout of the repository went too: %v", err)
	}
}

// A checkout deleted by hand is pruned from the repository, and its record
// goes; nothing is left that names it.
func TestRemoveOfAMissingCheckoutPrunes(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	root := t.TempDir()
	record, err := Create(root, source, "gone")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(record.Path); err != nil {
		t.Fatal(err)
	}
	if err := Remove(record, false); err != nil {
		t.Fatal(err)
	}
	if listed := must(t, source, "worktree", "list", "--porcelain"); strings.Contains(listed, record.Path) {
		t.Errorf("git still knows it:\n%s", listed)
	}
	if records, _ := List(root); len(records) != 0 {
		t.Errorf("records left: %+v", records)
	}
}
