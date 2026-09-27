package worktree

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// A claim makes its branch in one step, so a branch somebody else made is
// never taken for it: the claim is refused with their branch left as it was,
// and a checkout that then fails takes back only the branch its claim made.
func TestAClaimTakesBackOnlyItsOwnBranch(t *testing.T) {
	isolate(t)
	source, err := inspect(repository(t, "project"))
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	record := source
	record.Name, record.Branch, record.Path = "taken", "taken", filepath.Join(directory, "taken")
	must(t, source.Source, "branch", "taken", record.Commit)
	var taken *BranchTakenError
	if err := claim(record); !errors.As(err, &taken) {
		t.Errorf("a claim of an existing branch: %v", err)
	}
	if tip, _ := refTip(record.CommonDir, branchRef("taken")); tip != record.Commit {
		t.Errorf("their branch is at %q", tip)
	}
	if _, err := os.Stat(recordPath(record)); !os.IsNotExist(err) {
		t.Errorf("the refused claim left its record: %v", err)
	}

	record.Name, record.Branch, record.Path = "mine", "mine", filepath.Join(directory, "mine")
	if err := claim(record); err != nil {
		t.Fatal(err)
	}
	if tip, _ := refTip(record.CommonDir, branchRef("mine")); tip != record.Commit {
		t.Fatalf("the claim made no branch: %q", tip)
	}
	write(t, record.Path, "somebody's file\n")
	if err := checkout(source, record); err == nil {
		t.Fatal("git worktree add over a file succeeded")
	}
	if tip, _ := refTip(record.CommonDir, branchRef("mine")); tip != "" {
		t.Errorf("the claim's own branch stayed at %s", tip)
	}
	if text, err := os.ReadFile(record.Path); err != nil || string(text) != "somebody's file\n" {
		t.Errorf("the file at the path went: %q, %v", text, err)
	}
}

// A record naming a branch other than its name, or an included path out of
// its checkout, is no record rewake wrote: land would move a branch it did not
// make, rm would take a file outside for a copy.
func TestAForgedRecordIsLeftOut(t *testing.T) {
	root := t.TempDir()
	good := Record{Name: "good", Repository: "project-abc", Path: filepath.Join(root, "project-abc", "good"), Branch: "good", Included: []string{".env", "config/local"}}
	for _, record := range []Record{
		good,
		{Name: "branch", Repository: "project-abc", Path: filepath.Join(root, "project-abc", "branch"), Branch: "main"},
		{Name: "parent", Repository: "project-abc", Path: filepath.Join(root, "project-abc", "parent"), Branch: "parent", Included: []string{"../outside"}},
		{Name: "absolute", Repository: "project-abc", Path: filepath.Join(root, "project-abc", "absolute"), Included: []string{"/etc/passwd"}},
		{Name: "unclean", Repository: "project-abc", Path: filepath.Join(root, "project-abc", "unclean"), Included: []string{"a/../b"}},
	} {
		data, err := encode(record)
		if err != nil {
			t.Fatal(err)
		}
		write(t, recordPath(record), string(data))
	}
	records, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Name != "good" {
		t.Errorf("listed %+v", records)
	}
}
