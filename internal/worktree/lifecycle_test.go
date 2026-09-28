package worktree

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/proc"
	"github.com/praline-labs/rewake/internal/state"
)

// liveProcess starts a process that outlives the test's looks and names it
// as a record names its launcher.
func liveProcess(t *testing.T) string {
	t.Helper()
	command := exec.Command("sleep", "60")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = command.Process.Kill()
		_ = command.Wait()
	})
	start, err := proc.StartTime(command.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	return strconv.Itoa(command.Process.Pid) + "." + strconv.FormatUint(start, 10)
}

// stored is the record of a checkout as it is on disk.
func stored(t *testing.T, record Record) Record {
	t.Helper()
	held, err := read(recordPath(record))
	if err != nil {
		t.Fatal(err)
	}
	return held
}

// launchedBy rewrites a record as if another process had made it.
func launchedBy(t *testing.T, record Record, launcher string) Record {
	t.Helper()
	record = stored(t, record)
	record.Launcher = launcher
	data, err := encode(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.WriteAtomic(recordPath(record), data); err != nil {
		t.Fatal(err)
	}
	return record
}

// An rm or a finish that meets a launch still adding the checkout of its
// name looks at it only once the launch has let go of the lock: the look
// taken before would see a name claimed and nothing checked out yet, and the
// removal after the wait would take the checkout the launch had just made.
func TestRemovalLooksAtWhatALaunchMadeNotAtItsClaim(t *testing.T) {
	isolate(t)
	removals := map[string]func(Record, func(Record) error) error{
		"rm": func(record Record, keep func(Record) error) error { return RemoveChecked(record, false, keep) },
		"finish": func(record Record, keep func(Record) error) error {
			_, err := Finish(record, "", keep)
			return err
		},
	}
	for name, removal := range removals {
		t.Run(name, func(t *testing.T) {
			source := repository(t, "project")
			root := t.TempDir()
			claimed, release := make(chan struct{}), make(chan struct{})
			swap(t, &beforeAdd, func(Record) {
				close(claimed)
				<-release
			})
			made := make(chan error, 1)
			go func() {
				_, err := Create(root, source, "work")
				made <- err
			}()
			<-claimed
			records, err := List(root)
			if err != nil || len(records) != 1 {
				t.Fatalf("claimed: %v, %v", records, err)
			}
			seen, removed := make(chan Check, 1), make(chan error, 1)
			kept := errors.New("kept")
			go func() {
				removed <- removal(records[0], func(current Record) error {
					check, err := Inspect(current)
					if err != nil {
						return err
					}
					seen <- check
					return kept
				})
			}()
			select {
			case check := <-seen:
				t.Errorf("it looked while the launch was adding the checkout, and saw %+v", check)
			case <-time.After(300 * time.Millisecond):
			}
			close(release)
			if err := <-made; err != nil {
				t.Fatal(err)
			}
			select {
			case check := <-seen:
				if check.Missing || check.Forgotten {
					t.Errorf("it saw no checkout after the launch made it: %+v", check)
				}
			case <-time.After(10 * time.Second):
				t.Error("it never looked")
			}
			if err := <-removed; !errors.Is(err, kept) {
				t.Errorf("removal: %v", err)
			}
			if !isDir(records[0].Path) {
				t.Error("the checkout the launch made is gone")
			}
		})
	}
}

// A checkout made and not yet claimed by its session is still being launched
// while the process that made it runs; this process's own is not somebody
// else's launch, a claimed one is the session's, and an ended launcher says
// nothing.
func TestALaunchStillStartingIsSeen(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	record, err := Create(t.TempDir(), source, "work")
	if err != nil {
		t.Fatal(err)
	}
	if held := stored(t, record); held.Launcher == "" || held.Launching() {
		t.Errorf("made by this process: launcher %q, launching %v", held.Launcher, held.Launching())
	}
	if other := launchedBy(t, record, liveProcess(t)); !other.Launching() {
		t.Error("a launch by another live process is not seen")
	} else if claimed, err := Claim(other, Owner{Name: "tree", Epoch: "1.1"}); err != nil || claimed.Launching() {
		t.Errorf("claimed: %v, %v", claimed.Launching(), err)
	}
	if ended := launchedBy(t, record, "1.1"); ended.Launching() {
		t.Error("a launcher that ended is taken for a launch")
	}
}

// What .worktreeinclude copied is in the record on disk as soon as the
// checkout is made: a launch killed before its session is claimed would
// otherwise leave the copies looking like ignored work of the checkout's own.
func TestIncludedCopiesAreRecordedBeforeTheClaim(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	write(t, filepath.Join(source, ".gitignore"), ".env\n")
	write(t, filepath.Join(source, IncludeFile), ".env\n")
	must(t, source, "add", ".gitignore", IncludeFile)
	must(t, source, "commit", "-q", "-m", "Ignore")
	write(t, filepath.Join(source, ".env"), "TOKEN=x\n")
	record, err := Create(t.TempDir(), source, "work")
	if err != nil {
		t.Fatal(err)
	}
	if held := stored(t, record); len(held.Included) != 1 || held.Included[0] != ".env" {
		t.Errorf("record on disk lists %v", held.Included)
	} else if check, err := Inspect(held); err != nil || check.Ignored {
		t.Errorf("the copy counts as work: %+v, %v", check, err)
	}
}

// A worktree root inside the main checkout is refused for a launch from a
// linked one as well: the checkouts would show up in the main checkout's
// status and searches.
func TestARootInsideTheMainCheckoutIsRefusedFromALinkedOne(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	linked := filepath.Join(t.TempDir(), "linked")
	must(t, source, "worktree", "add", "-q", "-b", "side", linked)
	_, err := Create(filepath.Join(source, "trees"), linked, "work")
	var unusable *UnusableError
	if !errors.As(err, &unusable) || !strings.Contains(err.Error(), source) {
		t.Errorf("got %v", err)
	}
}

// A checkout somebody locked, or one holding a submodule checked out, is
// what git worktree remove refuses; Inspect says so, and a forced removal
// passes the lock as well.
func TestInspectSeesWhatGitWorktreeRemoveRefuses(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	locked, err := Create(t.TempDir(), source, "locked")
	if err != nil {
		t.Fatal(err)
	}
	must(t, source, "worktree", "lock", "--reason", "on a stick", locked.Path)
	if check, err := Inspect(locked); err != nil || !check.Locked || check.LockReason != "on a stick" {
		t.Errorf("locked: %+v, %v", check, err)
	}
	if err := Remove(locked, false); err == nil {
		t.Error("git removed a locked checkout without force")
	}
	if err := Remove(locked, true); err != nil || isDir(locked.Path) {
		t.Errorf("forced removal of a locked checkout: %v", err)
	}

	sub := repository(t, "library")
	must(t, source, "-c", "protocol.file.allow=always", "submodule", "add", "-q", sub, "library")
	must(t, source, "commit", "-q", "-m", "Library")
	record, err := Create(t.TempDir(), source, "work")
	if err != nil {
		t.Fatal(err)
	}
	if check, err := Inspect(record); err != nil || check.Submodules {
		t.Errorf("a submodule not checked out: %+v, %v", check, err)
	}
	// Checked out by hand: a repository of its own in the submodule's
	// directory, with nothing under the checkout's modules.
	must(t, filepath.Join(record.Path, "library"), "init", "-q")
	if check, err := Inspect(record); err != nil || !check.Submodules {
		t.Errorf("a submodule checked out by hand: %+v, %v", check, err)
	}
	if err := Remove(record, false); err == nil || !strings.Contains(err.Error(), "submodules") {
		t.Errorf("git removed it: %v", err)
	}
	other, err := Create(t.TempDir(), source, "other")
	if err != nil {
		t.Fatal(err)
	}
	must(t, other.Path, "-c", "protocol.file.allow=always", "submodule", "update", "-q", "--init")
	if check, err := Inspect(other); err != nil || !check.Submodules {
		t.Errorf("a submodule checked out: %+v, %v", check, err)
	}
}

// A source that switches to another branch while land runs does not get that
// branch fast-forwarded silently: right before the merge the source is asked
// again, and after it the target must hold the tip.
func TestLandSeesTheSourceSwitchBranches(t *testing.T) {
	isolate(t)
	for _, at := range []string{"look", "merge"} {
		t.Run(at, func(t *testing.T) {
			source := repository(t, "project")
			record, err := Create(t.TempDir(), source, "work")
			if err != nil {
				t.Fatal(err)
			}
			tip := commit(t, record.Path, "mine")
			old := must(t, source, "rev-parse", "main")
			must(t, source, "branch", "other")
			switchSource := func(string) { must(t, source, "switch", "-q", "other") }
			if at == "look" {
				swap(t, &beforeLook, switchSource)
			} else {
				swap(t, &beforeMerge, switchSource)
			}
			_, err = Land(record, "")
			var blocked *StateError
			if !errors.As(err, &blocked) {
				t.Fatalf("got %v", err)
			}
			if main := must(t, source, "rev-parse", "main"); main != old {
				t.Errorf("main moved to %s", main)
			}
			other := must(t, source, "rev-parse", "other")
			switch at {
			case "look":
				if other != old || !strings.Contains(err.Error(), "nothing was merged") {
					t.Errorf("other at %s: %v", other, err)
				}
			case "merge":
				if other != tip || !strings.Contains(err.Error(), "switched from main to other") {
					t.Errorf("other at %s: %v", other, err)
				}
			}
		})
	}
}

// A hook that runs past the wait gets land a refusal naming it instead of a
// hang, and git, not stopped, finishes the landing on its own.
func TestLandStopsWaitingForASlowHook(t *testing.T) {
	isolate(t)
	source := repository(t, "project")
	mark := filepath.Join(t.TempDir(), "done")
	path := filepath.Join(source, ".git", "hooks", "post-merge")
	write(t, path, "#!/bin/sh\nsleep 1\ntouch '"+mark+"'\n")
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
	record, err := Create(t.TempDir(), source, "work")
	if err != nil {
		t.Fatal(err)
	}
	tip := commit(t, record.Path, "mine")
	swap(t, &hookWait, 200*time.Millisecond)
	_, err = Land(record, "")
	var slow *HookWaitError
	if !errors.As(err, &slow) || !strings.Contains(err.Error(), "post-merge hook") {
		t.Fatalf("got %v", err)
	}
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		_, err := os.Stat(mark)
		if err == nil && !proc.Alive(slow.PID, 0) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("git did not finish the landing on its own")
		}
	}
	if main := must(t, source, "rev-parse", "main"); main != tip {
		t.Errorf("main is at %s, not %s", main, tip)
	}
}
