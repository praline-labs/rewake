package worktree

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// hook installs an executable hook in the repository that leaves a mark when
// it runs.
func hook(t *testing.T, source, name, mark string) {
	t.Helper()
	path := filepath.Join(source, ".git", "hooks", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\ntouch '"+mark+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// Making a checkout runs none of the repository's hooks, and an inherited
// configuration does not reach git; landing runs the hooks, as a merge the
// person made in their own checkout would.
func TestHooksRunOnlyWhenLanding(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	marks := t.TempDir()
	hook(t, source, "post-checkout", filepath.Join(marks, "checkout"))
	hook(t, source, "reference-transaction", filepath.Join(marks, "ref"))
	hook(t, source, "post-merge", filepath.Join(marks, "merge"))
	// A parent git's -c, which would make every call see a bare repository.
	t.Setenv("GIT_CONFIG_PARAMETERS", "'core.bare'='true'")
	record, err := Create(t.TempDir(), source, "work")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"checkout", "ref"} {
		if _, err := os.Stat(filepath.Join(marks, name)); !os.IsNotExist(err) {
			t.Errorf("the %s hook ran when the checkout was made: %v", name, err)
		}
	}
	write(t, filepath.Join(record.Path, "new"), "x\n")
	must(t, record.Path, "add", "new")
	must(t, record.Path, "commit", "-q", "-m", "New")
	if _, err := Land(record, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(marks, "merge")); err != nil {
		t.Errorf("the post-merge hook did not run on land: %v", err)
	}
}

// A git worktree add that fails leaves no branch, directory or record: the
// branch -b made before the checkout failed is taken back with it.
func TestAFailedAddLeavesNothing(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	write(t, filepath.Join(source, ".gitattributes"), "* filter=broken\n")
	must(t, source, "add", ".gitattributes")
	must(t, source, "commit", "-q", "-m", "Filter")
	must(t, source, "config", "filter.broken.smudge", "false")
	must(t, source, "config", "filter.broken.required", "true")
	directory := t.TempDir()
	_, err := Create(directory, source, "work")
	if err == nil || !strings.Contains(err.Error(), "git worktree add failed") {
		t.Fatalf("create: %v", err)
	}
	if branches := must(t, source, "branch", "--list", "work"); branches != "" {
		t.Errorf("the branch stayed: %q", branches)
	}
	if records, err := List(directory); err != nil || len(records) != 0 {
		t.Errorf("records: %+v, %v", records, err)
	}
	if left, _ := filepath.Glob(filepath.Join(directory, "*", "work*")); len(left) != 0 {
		t.Errorf("left behind: %v", left)
	}
}
