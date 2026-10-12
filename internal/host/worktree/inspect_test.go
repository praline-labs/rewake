package worktree

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// A checkout still at the commit it was made at loses that commit too once
// the branches that held it are gone: reachability is asked every time, not
// only after the HEAD moved. Its own branch holds it while it stays; one the
// checkout left, detached, holds nothing.
func TestACommitNoBranchHoldsAnyMoreIsUnreachable(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	must(t, source, "checkout", "-q", "-b", "topic")
	must(t, source, "commit", "-q", "--allow-empty", "-m", "Only on topic")
	record, err := Create(t.TempDir(), source, "work")
	if err != nil {
		t.Fatal(err)
	}
	must(t, source, "checkout", "-q", "main")
	must(t, source, "branch", "-q", "-D", "topic")
	if check, err := Inspect(record); err != nil || check.Dirty() || check.Branch != "work" {
		t.Fatalf("while its own branch holds it: %+v, %v", check, err)
	}
	must(t, record.Path, "switch", "-q", "--detach")
	must(t, source, "branch", "-q", "-D", "work")
	check, err := Inspect(record)
	if err != nil || !check.Unreachable || check.Head != record.Commit || !check.Dirty() {
		t.Fatalf("after topic was deleted: %+v, %v", check, err)
	}
	// A remote-tracking ref holds it as well as a branch does.
	must(t, source, "update-ref", "refs/remotes/origin/topic", record.Commit)
	if check, err := Inspect(record); err != nil || check.Unreachable {
		t.Errorf("held by a remote-tracking ref: %+v, %v", check, err)
	}
}

// Files git ignores are what git worktree remove deletes without asking — a
// .env, a local build — so they count as work; they are told apart from
// changes.
func TestIgnoredFilesAreWork(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	write(t, filepath.Join(source, ".gitignore"), ".env\n")
	must(t, source, "add", ".gitignore")
	must(t, source, "commit", "-q", "-m", "Ignore .env")
	record, err := Create(t.TempDir(), source, "env")
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(record.Path, ".env"), "TOKEN=x\n")
	check, err := Inspect(record)
	if err != nil || !check.Ignored || check.Changes || !check.Dirty() {
		t.Fatalf("with a .env: %+v, %v", check, err)
	}
}

// A checkout whose directory is gone is looked at through the repository's
// list, and removing it takes out its own entry only: another missing
// checkout's entry stays for git worktree repair. Once the repository has
// forgotten it too, only the record is left to remove.
func TestAMissingCheckoutIsRemovedAlone(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	root := t.TempDir()
	gone, err := Create(root, source, "gone")
	if err != nil {
		t.Fatal(err)
	}
	moved, err := Create(root, source, "moved")
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range []Record{gone, moved} {
		if err := os.Rename(record.Path, record.Path+".away"); err != nil {
			t.Fatal(err)
		}
	}
	check, err := Inspect(gone)
	if err != nil || !check.Missing || check.Forgotten || check.Head != gone.Commit || check.Unreachable {
		t.Fatalf("a missing checkout: %+v, %v", check, err)
	}
	if err := Remove(gone, true); err != nil {
		t.Fatal(err)
	}
	listed := must(t, source, "worktree", "list", "--porcelain")
	if strings.Contains(listed, "worktree "+gone.Path+"\n") || !strings.Contains(listed, "worktree "+moved.Path+"\n") {
		t.Fatalf("after removing %s git lists:\n%s", gone.Name, listed)
	}
	if records, _ := List(root); len(records) != 1 || records[0].Name != "moved" {
		t.Errorf("records left: %+v", records)
	}

	must(t, source, "worktree", "prune")
	check, err = Inspect(moved)
	if err != nil || !check.Missing || !check.Forgotten || check.Dirty() {
		t.Fatalf("a forgotten checkout: %+v, %v", check, err)
	}
	if err := Remove(moved, false); err != nil {
		t.Fatal(err)
	}
	if records, _ := List(root); len(records) != 0 {
		t.Errorf("records left: %+v", records)
	}
	if _, err := os.Stat(moved.Path + ".away"); err != nil {
		t.Errorf("the moved directory was touched: %v", err)
	}
}

// A worktree directory inside the repository would be listed among its own
// files; it is refused, symbolic links resolved, dangling ones as well, before
// anything is made.
func TestARootInsideTheRepositoryIsRefused(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	link, dangling := filepath.Join(t.TempDir(), "link"), filepath.Join(t.TempDir(), "dangling")
	if err := os.Symlink(source, link); err != nil {
		t.Fatal(err)
	}
	// A link to a directory not made yet, which is where the root would be.
	if err := os.Symlink(filepath.Join(source, "trees"), dangling); err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{filepath.Join(source, "trees"), source, filepath.Join(link, "trees"), dangling, filepath.Join(dangling, "x")} {
		var unusable *UnusableError
		if _, err := Create(root, source, "x"); !errors.As(err, &unusable) || !strings.Contains(err.Error(), RootEnv) {
			t.Errorf("%s: %v", root, err)
		}
	}
	if _, err := os.Stat(filepath.Join(source, "trees")); !os.IsNotExist(err) {
		t.Errorf("a refused root was made: %v", err)
	}
	if _, err := Create(filepath.Join(filepath.Dir(source), "project-trees"), source, "x"); err != nil {
		t.Errorf("a sibling whose name starts like the repository's: %v", err)
	}
}

// A record naming a checkout other than the directory of its own name is not
// rewake's to remove.
func TestListSkipsARecordNamingAnotherDirectory(t *testing.T) {
	isolate(t)
	root := t.TempDir()
	record, err := Create(root, repository(t, "project"), "real")
	if err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{
		"other":  filepath.Join(filepath.Dir(record.Path), "real"),
		"dotted": filepath.Dir(record.Path) + "/x/../dotted",
	} {
		forged := record
		forged.Name, forged.Path = name, path
		data, _ := encode(forged)
		write(t, filepath.Join(filepath.Dir(record.Path), name+".json"), string(data))
	}
	records, err := List(root)
	if err != nil || len(records) != 1 || records[0].Name != "real" {
		t.Fatalf("listed %+v, %v", records, err)
	}
}

// Nothing git runs for rewake may wait for a person or stand in a session's
// way: no terminal prompt, a session of its own, and no optional locks for a
// commit to trip on.
func TestGitRunsDetachedAndWithoutOptionalLocks(t *testing.T) {
	t.Setenv("GIT_TERMINAL_PROMPT", "1")
	t.Setenv("GIT_OPTIONAL_LOCKS", "1")
	out, err := run(exec.Command("sh", "-c", `echo "$GIT_TERMINAL_PROMPT $GIT_OPTIONAL_LOCKS $$ $(ps -o sid= -p $$)"`))
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(out)
	if len(fields) != 4 || fields[0] != "0" || fields[1] != "0" {
		t.Fatalf("git would see %q", out)
	}
	if pid, _ := strconv.Atoi(fields[2]); strconv.Itoa(pid) != fields[3] {
		t.Errorf("not a session leader: pid %s, session %s", fields[2], fields[3])
	}
}

// A repository deleted from under its checkouts leaves git nothing to say of
// them. The record of one whose directory is gone is all there is to remove;
// one whose directory is still there is removed only by force, directory and
// all.
func TestARecordWhoseRepositoryIsGone(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	root := t.TempDir()
	kept, err := Create(root, source, "kept")
	if err != nil {
		t.Fatal(err)
	}
	gone, err := Create(root, source, "gone")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(gone.Path); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(source); err != nil {
		t.Fatal(err)
	}
	if check, err := Inspect(gone); err != nil || !check.Missing || !check.Forgotten {
		t.Fatalf("gone with its repository: %+v, %v", check, err)
	}
	if err := Remove(gone, false); err != nil {
		t.Fatal(err)
	}
	if check, err := Inspect(kept); err != nil || check.Missing || !check.Forgotten {
		t.Fatalf("left without its repository: %+v, %v", check, err)
	}
	if err := Remove(kept, false); err == nil {
		t.Fatal("removed without force")
	}
	if _, err := os.Stat(kept.Path); err != nil {
		t.Fatalf("a refused removal touched the directory: %v", err)
	}
	if err := Remove(kept, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(kept.Path); !os.IsNotExist(err) {
		t.Errorf("the directory stayed: %v", err)
	}
	if records, _ := List(root); len(records) != 0 {
		t.Errorf("records left: %+v", records)
	}
}
