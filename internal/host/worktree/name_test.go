package worktree

import (
	"errors"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Every name of rewake's characters is valid exactly when git takes it as a
// branch, but for the few rewake refuses beyond git; git is the judge, over a
// table and over random names of the characters its rules are about.
func TestNamesAgreeWithGit(t *testing.T) {
	isolate(t)
	top := repository(t, "project")
	names := []string{
		"fix", "feat/super-feature", "a/b/c", "release-1.2", "v1.2.3", "a_b", "x.lock.y",
		"feat/", "/feat", "feat//x", "a..b", "a/.b", ".a", "a.", "a/b.", "a.lock",
		"a.lock/b", "a/b.lock", "-a", "a/-b", "a/.", "a/..", ".", "..",
	}
	random := rand.New(rand.NewPCG(1, 2))
	for range 300 {
		const letters = "ab./-_lockjson"
		word := make([]byte, 1+random.IntN(10))
		for i := range word {
			word[i] = letters[random.IntN(len(letters))]
		}
		names = append(names, string(word))
	}
	for _, name := range names {
		command := exec.Command("git", "check-ref-format", "--branch", name)
		command.Dir = top
		gitTakes := command.Run() == nil
		refused := strings.HasSuffix(name, ".json")
		if got := ValidName(name); got != (gitTakes && !refused) {
			t.Errorf("ValidName(%q) = %v, git takes it: %v", name, got, gitTakes)
		}
	}
}

// What rewake refuses beyond git, and what it takes that the old rule did not.
func TestNameRule(t *testing.T) {
	for _, name := range []string{
		"a+b", "feat+x", "a b", "a;b", "a$b", "a'b", "ü", "a@b", "a~1", "a:b",
		"HEAD", strings.Repeat("ab", 20), "x.json", "feat/x.JSON", strings.Repeat("a", MaxName+1), "",
	} {
		if ValidName(name) {
			t.Errorf("%q is valid", name)
		}
	}
	for _, name := range []string{"feat/super-feature", "release-1.2", "a/b/c", strings.Repeat("a", MaxName), "HEAD/x", "wt-0a1b2c"} {
		if !ValidName(name) {
			t.Errorf("%q is not valid", name)
		}
	}
}

// A name with a slash is a branch of that name and one directory beside the
// others, the slash written +; land, ls and find take the name as it is.
func TestANameWithASlash(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	root := t.TempDir()
	record, err := Create(root, source, "feat/super-feature")
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(root, record.Repository)
	if record.Branch != "feat/super-feature" || record.Path != filepath.Join(directory, "feat+super-feature") {
		t.Fatalf("made %+v", record)
	}
	if _, err := os.Stat(filepath.Join(directory, "feat+super-feature.json")); err != nil {
		t.Errorf("the record: %v", err)
	}
	if head := strings.TrimSpace(must(t, record.Path, "symbolic-ref", "HEAD")); head != "refs/heads/feat/super-feature" {
		t.Errorf("the checkout is on %s", head)
	}
	for _, ref := range []string{"feat/super-feature", record.Repository + "/feat/super-feature"} {
		if found, err := Find(root, ref); err != nil || len(found) != 1 || found[0].Path != record.Path {
			t.Errorf("%s found %+v, %v", ref, found, err)
		}
	}
	must(t, record.Path, "commit", "-q", "--allow-empty", "-m", "Work")
	if landing, err := Land(record, ""); err != nil || landing.Commits != 1 || landing.Target != "main" {
		t.Errorf("land: %+v, %v", landing, err)
	}
}

// Git keeps a branch's name either as a branch or as a directory of branches:
// feat/x beside a branch feat is refused as taken, naming feat, as feat beside
// feat/x is, naming feat/x.
func TestANameBelowABranchIsTaken(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	root := t.TempDir()
	if _, err := Create(root, source, "feat"); err != nil {
		t.Fatal(err)
	}
	_, err := Create(root, source, "feat/x")
	var taken *BranchTakenError
	if !errors.As(err, &taken) || taken.Above != "feat" || !strings.Contains(err.Error(), "a branch feat,") {
		t.Errorf("feat/x beside feat: %v", err)
	}
	must(t, source, "branch", "fix/login")
	if _, err := Create(root, source, "fix"); !errors.As(err, &taken) || taken.Below != "fix/login" {
		t.Errorf("fix beside fix/login: %v", err)
	}
	if records, _ := List(root); len(records) != 1 {
		t.Errorf("the refused names left records: %+v", records)
	}
}

// A ref is read as <repository>/<name> first: a checkout named like another's
// full form does not hide it, and is found by its own full form.
func TestAFullRefWinsOverASlashName(t *testing.T) {
	root := t.TempDir()
	plain := Record{Name: "b", Repository: "x-abc123", Path: filepath.Join(root, "x-abc123", "b"), Branch: "b"}
	slashed := Record{Name: "x-abc123/b", Repository: "y-def456", Path: filepath.Join(root, "y-def456", "x-abc123+b"), Branch: "x-abc123/b"}
	elsewhere := Record{Name: "c/d", Repository: "y-def456", Path: filepath.Join(root, "y-def456", "c"), Branch: "c/d"}
	for path, record := range map[string]Record{
		recordPath(plain):     plain,
		recordPath(slashed):   slashed,
		recordPath(elsewhere): elsewhere,
	} {
		data, err := encode(record)
		if err != nil {
			t.Fatal(err)
		}
		write(t, path, string(data))
	}
	for ref, want := range map[string]string{"x-abc123/b": plain.Path, "y-def456/x-abc123/b": slashed.Path} {
		if found, err := Find(root, ref); err != nil || len(found) != 1 || found[0].Path != want {
			t.Errorf("%s found %+v, %v", ref, found, err)
		}
	}
	// A record whose checkout is not its flattened name's directory is
	// none rewake wrote.
	if found, _ := Find(root, "c/d"); len(found) != 0 {
		t.Errorf("a checkout elsewhere was listed: %+v", found)
	}
}
