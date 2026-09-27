package worktree

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// isolate keeps git away from the machine's configuration: a global hook or
// a signing setting would make these tests about that machine.
func isolate(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.invalid")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.invalid")
}

func must(t *testing.T, dir string, args ...string) string {
	t.Helper()
	output, err := gitOutput(dir, args...)
	if err != nil {
		t.Fatalf("git %q in %s: %v", args, dir, err)
	}
	return output
}

// repository makes a repository with one commit holding src/nested/file, and
// returns its top.
func repository(t *testing.T, name string) string {
	t.Helper()
	top := filepath.Join(t.TempDir(), name)
	write(t, filepath.Join(top, "src", "nested", "file"), "one\n")
	must(t, top, "init", "-q", "-b", "main")
	must(t, top, "add", ".")
	must(t, top, "commit", "-q", "-m", "First")
	resolved, err := filepath.EvalSymlinks(top)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A launch gets the commit it stood on, on a new branch of the checkout's
// name, so the branch checked out in the source stays free and the work has a
// branch to land from; the place within the repository comes along.
func TestCreateChecksOutHeadOnItsOwnBranchAtTheSamePlace(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	root := t.TempDir()
	record, err := Create(root, filepath.Join(source, "src", "nested"), "")
	if err != nil {
		t.Fatal(err)
	}
	if !ValidName(record.Name) || !regexp.MustCompile(`^wt-[0-9a-f]{6}$`).MatchString(record.Name) || record.Branch != record.Name {
		t.Errorf("generated name %q, branch %q", record.Name, record.Branch)
	}
	if !strings.HasPrefix(record.Repository, "project-") || record.Path != filepath.Join(root, record.Repository, record.Name) {
		t.Errorf("placed at %s in %s", record.Path, record.Repository)
	}
	if record.Source != source || record.Subdir != filepath.Join("src", "nested") {
		t.Errorf("source %s, subdir %q", record.Source, record.Subdir)
	}
	if head := must(t, record.Path, "rev-parse", "HEAD"); head != record.Commit || head != must(t, source, "rev-parse", "HEAD") {
		t.Errorf("checked out %s, recorded %s", head, record.Commit)
	}
	if branch := must(t, record.Path, "symbolic-ref", "-q", "HEAD"); branch != "refs/heads/"+record.Name {
		t.Errorf("the checkout is on %q, not on a branch of its name", branch)
	}
	if source, _, _ := currentBranch(source); source != "main" {
		t.Errorf("the source moved to %q", source)
	}
	if got := record.Workdir(); got != filepath.Join(record.Path, "src", "nested") {
		t.Errorf("workdir %s", got)
	}
	if listed := must(t, source, "worktree", "list", "--porcelain"); !strings.Contains(listed, "worktree "+record.Path+"\n") {
		t.Errorf("git does not know the checkout:\n%s", listed)
	}
	if records, err := List(root); err != nil || len(records) != 1 || records[0].Ref() != record.Ref() {
		t.Errorf("listed %+v, %v", records, err)
	}
}

// A directory that is not in the commit — an untracked one — has no place in
// the checkout, and the launch starts at its top.
func TestAnUntrackedLaunchDirectoryStartsAtTheTop(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	untracked := filepath.Join(source, "scratch")
	if err := os.Mkdir(untracked, 0o755); err != nil {
		t.Fatal(err)
	}
	record, err := Create(t.TempDir(), untracked, "top")
	if err != nil {
		t.Fatal(err)
	}
	if record.Subdir != "scratch" || record.Workdir() != record.Path {
		t.Errorf("subdir %q, workdir %s", record.Subdir, record.Workdir())
	}
	atTop, err := Create(t.TempDir(), source, "root")
	if err != nil {
		t.Fatal(err)
	}
	if atTop.Subdir != "" || atTop.Workdir() != atTop.Path {
		t.Errorf("subdir %q, workdir %s", atTop.Subdir, atTop.Workdir())
	}
}

// A name is one path element that cannot be read as a record's file.
func TestNamesOutOfShapeAreRefused(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	root := t.TempDir()
	for _, name := range []string{"../escape", "a/b", "a.json", ".hidden", "-flag", "_x", "has space", strings.Repeat("a", 41), "HEAD", strings.Repeat("a", 40)} {
		var unusable *UnusableError
		if _, err := Create(root, source, name); !errors.As(err, &unusable) {
			t.Errorf("%q: %v", name, err)
		}
	}
	if entries, _ := os.ReadDir(root); len(entries) != 0 {
		t.Errorf("a refused name left %d entries", len(entries))
	}
	if _, err := Create(root, source, strings.Repeat("a", 39)+"g"); err != nil {
		t.Errorf("forty characters: %v", err)
	}
}

// A taken name is refused, whether by a checkout of rewake's or by anything
// standing where the checkout would go, and nothing of the other is touched.
func TestATakenNameIsRefused(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	root := t.TempDir()
	first, err := Create(root, source, "fix")
	if err != nil {
		t.Fatal(err)
	}
	var exists *ExistsError
	if _, err := Create(root, source, "fix"); !errors.As(err, &exists) || exists.Record.Path != first.Path || exists.Record.Commit != first.Commit {
		t.Fatalf("second fix: %v", err)
	}
	stray := filepath.Join(root, first.Repository, "stray")
	write(t, filepath.Join(stray, "keep"), "mine\n")
	if _, err := Create(root, source, "stray"); !errors.As(err, &exists) {
		t.Fatalf("a directory in the way: %v", err)
	}
	if _, err := os.Stat(filepath.Join(stray, "keep")); err != nil {
		t.Errorf("the directory in the way was touched: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, first.Repository, "stray.json")); !os.IsNotExist(err) {
		t.Errorf("a record was left for the refused name: %v", err)
	}
}

// Only a repository with a commit and a working tree can give a checkout.
func TestWhatHasNothingToCheckOutIsRefused(t *testing.T) {
	isolate(t)
	plain := t.TempDir()
	empty := t.TempDir()
	must(t, empty, "init", "-q")
	bare := t.TempDir()
	must(t, bare, "init", "-q", "--bare")
	for name, dir := range map[string]string{"plain": plain, "empty": empty, "bare": bare} {
		var unusable *UnusableError
		if _, err := Create(t.TempDir(), dir, ""); !errors.As(err, &unusable) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// Two repositories of one name keep their checkouts apart, and a bare name
// found in both is ambiguous where <repository>/<name> is not.
func TestRepositoriesOfOneNameAreKeptApart(t *testing.T) {
	isolate(t)
	root := t.TempDir()
	one, err := Create(root, repository(t, "same"), "fix")
	if err != nil {
		t.Fatal(err)
	}
	two, err := Create(root, repository(t, "same"), "fix")
	if err != nil {
		t.Fatal(err)
	}
	if one.Repository == two.Repository {
		t.Fatalf("both in %s", one.Repository)
	}
	if found, _ := Find(root, "fix"); len(found) != 2 {
		t.Errorf("bare name found %d", len(found))
	}
	if found, _ := Find(root, two.Ref()); len(found) != 1 || found[0].Path != two.Path {
		t.Errorf("%s found %+v", two.Ref(), found)
	}
	if found, _ := Find(root, "other/fix"); len(found) != 0 {
		t.Errorf("a wrong repository found %+v", found)
	}
}

// A GIT_DIR inherited from whatever started rewake — a hook, say — must not
// turn the checkout into one of another repository.
func TestAnInheritedGitDirIsIgnored(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	other := repository(t, "other")
	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	t.Setenv("GIT_WORK_TREE", other)
	record, err := Create(t.TempDir(), source, "")
	if err != nil {
		t.Fatal(err)
	}
	if record.CommonDir != filepath.Join(source, ".git") || record.Source != source {
		t.Errorf("made from %s (%s)", record.Source, record.CommonDir)
	}
}

// The owner written at claim is what List reads back.
func TestClaimRecordsTheSession(t *testing.T) {
	isolate(t)
	root := t.TempDir()
	record, err := Create(root, repository(t, "project"), "owned")
	if err != nil {
		t.Fatal(err)
	}
	owner := Owner{Name: "writer-codex", Room: "default", Epoch: "10.20", Harness: "codex", Dir: "/state/rooms/default"}
	if _, err := Claim(record, owner); err != nil {
		t.Fatal(err)
	}
	records, err := List(root)
	if err != nil || len(records) != 1 || records[0].Session == nil || *records[0].Session != owner {
		t.Fatalf("listed %+v, %v", records, err)
	}
}

// A file under the root that is not a record, or names another place, is
// left out rather than hiding the rest.
func TestListSkipsWhatIsNotARecord(t *testing.T) {
	isolate(t)
	root := t.TempDir()
	record, err := Create(root, repository(t, "project"), "real")
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(root, record.Repository)
	write(t, filepath.Join(directory, "junk.json"), "{")
	moved := record
	moved.Path = "/elsewhere/real"
	data, _ := encode(moved)
	write(t, filepath.Join(directory, "moved.json"), string(data))
	records, err := List(root)
	if err != nil || len(records) != 1 || records[0].Name != "real" {
		t.Fatalf("listed %+v, %v", records, err)
	}
}

func TestRootComesFromTheEnvironment(t *testing.T) {
	t.Setenv(RootEnv, "/data/trees/")
	if root, err := Root(); err != nil || root != "/data/trees" {
		t.Errorf("%s: %s, %v", RootEnv, root, err)
	}
	t.Setenv(RootEnv, "relative")
	if _, err := Root(); err == nil {
		t.Errorf("a relative %s was taken", RootEnv)
	}
	t.Setenv(RootEnv, "")
	t.Setenv("XDG_DATA_HOME", "/data")
	if root, _ := Root(); root != "/data/rewake/worktrees" {
		t.Errorf("XDG_DATA_HOME: %s", root)
	}
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", "/home/someone")
	if root, _ := Root(); root != "/home/someone/.local/share/rewake/worktrees" {
		t.Errorf("home: %s", root)
	}
}

// Codex takes a linked worktree's trust from its main checkout, and only when
// the metadata has the layout git worktree add writes: the checkout's .git
// file names <common>/worktrees/<name>, whose gitdir names that .git back and
// whose commondir leads to the common directory, the main checkout's .git
// (git-utils/src/trust.rs, docs/research-codex.md). A checkout of rewake's
// must keep that layout, or a trusted repository's launch would not be.
func TestTheCheckoutHasTheLayoutCodexTrusts(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	record, err := Create(t.TempDir(), source, "trusted")
	if err != nil {
		t.Fatal(err)
	}
	pointer, err := os.ReadFile(filepath.Join(record.Path, ".git"))
	if err != nil {
		t.Fatal(err)
	}
	private := strings.TrimSpace(strings.TrimPrefix(string(pointer), "gitdir:"))
	if !filepath.IsAbs(private) {
		private = filepath.Join(record.Path, private)
	}
	if filepath.Base(filepath.Dir(private)) != "worktrees" || filepath.Dir(filepath.Dir(private)) != record.CommonDir {
		t.Fatalf("the .git file names %s", private)
	}
	back, err := os.ReadFile(filepath.Join(private, "gitdir"))
	if err != nil || strings.TrimSpace(string(back)) != filepath.Join(record.Path, ".git") {
		t.Errorf("gitdir names %q, %v", back, err)
	}
	common, err := os.ReadFile(filepath.Join(private, "commondir"))
	if err != nil || filepath.Clean(filepath.Join(private, strings.TrimSpace(string(common)))) != record.CommonDir {
		t.Errorf("commondir names %q, %v", common, err)
	}
	if record.CommonDir != filepath.Join(source, ".git") {
		t.Errorf("the common directory %s is not the main checkout's .git", record.CommonDir)
	}
}
