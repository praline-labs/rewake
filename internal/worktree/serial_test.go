package worktree

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// swap replaces a test hook for one test.
func swap[T any](t *testing.T, at *T, with T) {
	t.Helper()
	was := *at
	*at = with
	t.Cleanup(func() { *at = was })
}

// halfEntry leaves an entry in the repository's worktrees directory the way
// git 2.43 leaves one it is in the middle of writing: its gitdir written, its
// commondir created and still empty.
func halfEntry(t *testing.T, commonDir, name string) string {
	t.Helper()
	entry := filepath.Join(commonDir, "worktrees", name)
	write(t, filepath.Join(entry, "gitdir"), filepath.Join(t.TempDir(), name, ".git")+"\n")
	write(t, filepath.Join(entry, "commondir"), "")
	return entry
}

// Launches in one repository at once make their checkouts one after another:
// each add here stands for a git worktree add in the middle of writing its
// entry, and an add of another launch meeting it would fail on it.
func TestCheckoutsOfOneRepositoryAreMadeOneAtATime(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	root := t.TempDir()
	commonDir := filepath.Join(source, ".git")
	var counting sync.Mutex
	var inside, most int
	swap(t, &beforeAdd, func(record Record) {
		counting.Lock()
		inside++
		most = max(most, inside)
		counting.Unlock()
		entry := filepath.Join(commonDir, "worktrees", "half-"+record.Name)
		_ = os.MkdirAll(entry, 0o755)
		_ = os.WriteFile(filepath.Join(entry, "gitdir"), []byte(entry+"/gitdir\n"), 0o644)
		_ = os.WriteFile(filepath.Join(entry, "commondir"), nil, 0o644)
		time.Sleep(300 * time.Millisecond)
		_ = os.RemoveAll(entry)
		counting.Lock()
		inside--
		counting.Unlock()
	})
	names := []string{"one", "two", "three"}
	errs := make([]error, len(names))
	start := make(chan struct{})
	var done sync.WaitGroup
	for i, name := range names {
		done.Add(1)
		go func() {
			defer done.Done()
			<-start
			_, errs[i] = Create(root, source, name)
		}()
	}
	close(start)
	done.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("%s: %v", names[i], err)
		}
	}
	if most != 1 {
		t.Errorf("%d adds ran at once", most)
	}
	records, err := List(root)
	if err != nil || len(records) != len(names) {
		t.Errorf("recorded %d checkouts: %v", len(records), err)
	}
}

// An add that meets another worktree's entry half-written — by a git worktree
// add rewake did not run — is tried once more, and succeeds once that entry is
// whole.
func TestAnAddMeetingAHalfWrittenEntryIsTriedOnceMore(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	entry := halfEntry(t, filepath.Join(source, ".git"), "elsewhere")
	var adds int
	swap(t, &beforeAdd, func(Record) { adds++ })
	swap(t, &beforeRetry, func() { write(t, filepath.Join(entry, "commondir"), "../..\n") })
	record, err := Create(t.TempDir(), source, "retried")
	if err != nil {
		t.Fatal(err)
	}
	if adds != 2 {
		t.Errorf("%d adds", adds)
	}
	if head := must(t, record.Path, "rev-parse", "--abbrev-ref", "HEAD"); head != "retried" {
		t.Errorf("checked out %q", head)
	}
}

// Once only: an entry that stays half-written is not waited out, and the failed
// add leaves nothing behind — no branch, no record, no directory.
func TestAnEntryThatStaysHalfWrittenFailsTheAdd(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	halfEntry(t, filepath.Join(source, ".git"), "elsewhere")
	var adds int
	swap(t, &beforeAdd, func(Record) { adds++ })
	swap(t, &beforeRetry, func() {})
	root := t.TempDir()
	_, err := Create(root, source, "stuck")
	if err == nil || !strings.Contains(err.Error(), "elsewhere/commondir") {
		t.Fatalf("error %v", err)
	}
	if adds != 2 {
		t.Errorf("%d adds", adds)
	}
	if out, _ := gitOutput(source, "branch", "--list", "stuck"); out != "" {
		t.Errorf("branch left: %q", out)
	}
	if records, _ := List(root); len(records) != 0 {
		t.Errorf("records left: %+v", records)
	}
}

// Other failures of git worktree add are not tried again.
func TestAnotherAddFailureIsNotTriedAgain(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	var adds int
	swap(t, &beforeAdd, func(record Record) {
		adds++
		write(t, filepath.Join(record.Path, "in-the-way"), "x\n")
	})
	swap(t, &beforeRetry, func() { t.Error("retried") })
	if _, err := Create(t.TempDir(), source, "blocked"); err == nil {
		t.Fatal("an add into a directory with a file in it succeeded")
	}
	if adds != 1 {
		t.Errorf("%d adds", adds)
	}
}

// A repository another rewake command keeps locked refuses a checkout in
// bounded time, naming the lock and its holder, and makes nothing; so does a
// removal.
func TestABusyRepositoryIsRefusedInTime(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	root := t.TempDir()
	made, err := Create(root, source, "first")
	if err != nil {
		t.Fatal(err)
	}
	lock := lockPath(filepath.Join(root, made.Repository))
	held, err := os.OpenFile(lock, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.Close() }()
	if err := syscall.Flock(int(held.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lock, []byte("4242\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	swap(t, &lockWait, 200*time.Millisecond)
	began := time.Now()
	_, err = Create(root, source, "second")
	var busy *BusyError
	if !errors.As(err, &busy) || busy.Holder != 4242 || !strings.Contains(err.Error(), lock) {
		t.Fatalf("error %v", err)
	}
	if took := time.Since(began); took > 5*time.Second {
		t.Errorf("refused after %s", took)
	}
	if out, _ := gitOutput(source, "branch", "--list", "second"); out != "" {
		t.Errorf("branch made: %q", out)
	}
	if err := Remove(made, false); !errors.As(err, &busy) {
		t.Errorf("removal while locked: %v", err)
	}
	if !isDir(made.Path) {
		t.Error("the locked removal took the checkout")
	}
	_ = syscall.Flock(int(held.Fd()), syscall.LOCK_UN)
	if _, err := Create(root, source, "second"); err != nil {
		t.Errorf("after the lock was let go: %v", err)
	}
}

// The lock lives beside the repository's directory under the root, not in the
// repository, and the directory still goes with its last checkout.
func TestTheLockStaysOutOfTheRepository(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	root := t.TempDir()
	before := fmt.Sprint(entries(t, filepath.Join(source, ".git")))
	record, err := Create(root, source, "only")
	if err != nil {
		t.Fatal(err)
	}
	if err := Remove(record, false); err != nil {
		t.Fatal(err)
	}
	if after := fmt.Sprint(entries(t, filepath.Join(source, ".git"))); after != before {
		t.Errorf("the Git directory went from %s to %s", before, after)
	}
	if got := entries(t, root); fmt.Sprint(got) != fmt.Sprint([]string{record.Repository + ".lock"}) {
		t.Errorf("root holds %v", got)
	}
}

func entries(t *testing.T, dir string) []string {
	t.Helper()
	read, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range read {
		names = append(names, entry.Name())
	}
	return names
}
