package grantauth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/grant"
	"github.com/iiiokojiadbi/rewake/internal/proc"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// ended is the run of a process that has exited.
func ended(t *testing.T) string {
	t.Helper()
	child := exec.Command("true")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	start, err := proc.StartTime(child.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	_ = child.Wait()
	return fmt.Sprintf("%d.%d", child.Process.Pid, start)
}

// ownRun is this process as a run: the test stands in for the wrapper of the
// resumed run, which asks, and for main's, which answers.
func ownRun(t *testing.T) string {
	own := ownExpect(t)
	return fmt.Sprintf("%d.%d", own.PID, own.Start)
}

// taskOpen is a task whose state a test sets.
type taskOpen struct{ open, found atomic.Bool }

func (o *taskOpen) check(Grant) (bool, bool) { return o.open.Load(), o.found.Load() }

// delivered is a grant registered for a run that has ended since, confirmed
// to it once, as a delivery confirms it; open says its task is.
func delivered(t *testing.T, lifetime time.Duration) (string, *taskOpen) {
	t.Helper()
	authority, path := listen(t, os.Getpid(), lifetime)
	task := &taskOpen{}
	task.open.Store(true)
	task.found.Store(true)
	authority.Open = task.check
	grant := lib
	grant.ToEpoch = ended(t)
	if err := Register(path, grant); err != nil {
		t.Fatal(err)
	}
	if _, err := Confirm(path, ownExpect(t), grant.ID, grant.To, grant.ToEpoch); err != nil {
		t.Fatal(err)
	}
	return path, task
}

// A grant whose task is open outlives the wait of its message: main's wrapper
// keeps it until the task is closed, and forgets it then.
func TestAGrantIsHeldWhileItsTaskIsOpen(t *testing.T) {
	path, task := delivered(t, 20*time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	run := ownRun(t)
	got, err := Reconfirm(path, ownExpect(t), lib.ID, lib.To, run)
	if err != nil || got.ToEpoch != run || !sameSet(got.Dirs, lib.Dirs) {
		t.Fatalf("an open task's grant past the wait: %+v, %v", got, err)
	}
	task.open.Store(false)
	if _, err := Confirm(path, ownExpect(t), lib.ID, lib.To, run); !errors.Is(err, ErrNotConfirmed) {
		t.Fatalf("confirmed once its task closed: %v", err)
	}
}

// Asked again for a run that resumed the conversation, main hands the grant
// over to it: the run it was granted to can no longer have it confirmed, and
// the same run asking again is answered the same.
func TestAGrantIsHandedOverToTheResumedRun(t *testing.T) {
	path, _ := delivered(t, time.Minute)
	run := ownRun(t)
	if _, err := Reconfirm(path, ownExpect(t), lib.ID, lib.To, run); err != nil {
		t.Fatal(err)
	}
	if _, err := Reconfirm(path, ownExpect(t), lib.ID, lib.To, run); err != nil {
		t.Fatalf("asked again by the same run: %v", err)
	}
	if got, err := Confirm(path, ownExpect(t), lib.ID, lib.To, run); err != nil || got.ToEpoch != run {
		t.Fatalf("not held for the new run: %+v, %v", got, err)
	}
}

// What main refuses to hand over: a grant for another recipient, one never
// delivered, one whose task is closed, one whose run still runs, and one asked
// for by a process that is not the run it names.
func TestAGrantIsNotHandedOverWithoutEveryCondition(t *testing.T) {
	run := ownRun(t)
	cases := map[string]func(t *testing.T) (path, to, epoch string){
		"another recipient": func(t *testing.T) (string, string, string) {
			path, _ := delivered(t, time.Minute)
			return path, "other", run
		},
		"never delivered": func(t *testing.T) (string, string, string) {
			_, path := listen(t, os.Getpid(), time.Minute)
			grant := lib
			grant.ToEpoch = ended(t)
			if err := Register(path, grant); err != nil {
				t.Fatal(err)
			}
			return path, lib.To, run
		},
		"a task not found": func(t *testing.T) (string, string, string) {
			path, task := delivered(t, time.Minute)
			task.open.Store(false)
			task.found.Store(false)
			return path, lib.To, run
		},
		"a closed task": func(t *testing.T) (string, string, string) {
			path, task := delivered(t, time.Minute)
			task.open.Store(false)
			return path, lib.To, run
		},
		"a run still running": func(t *testing.T) (string, string, string) {
			_, path := listen(t, os.Getpid(), time.Minute)
			alive := stranger(t)
			start, err := proc.StartTime(alive)
			if err != nil {
				t.Fatal(err)
			}
			grant := lib
			grant.ToEpoch = fmt.Sprintf("%d.%d", alive, start)
			if err := Register(path, grant); err != nil {
				t.Fatal(err)
			}
			if _, err := Confirm(path, ownExpect(t), grant.ID, grant.To, grant.ToEpoch); err != nil {
				t.Fatal(err)
			}
			return path, lib.To, run
		},
		"a process that is not the run": func(t *testing.T) (string, string, string) {
			path, _ := delivered(t, time.Minute)
			other := stranger(t)
			start, err := proc.StartTime(other)
			if err != nil {
				t.Fatal(err)
			}
			return path, lib.To, fmt.Sprintf("%d.%d", other, start)
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			path, to, epoch := setup(t)
			if got, err := Reconfirm(path, ownExpect(t), lib.ID, to, epoch); !errors.Is(err, ErrNotConfirmed) {
				t.Fatalf("handed over: %+v, %v", got, err)
			}
		})
	}
}

// Restoring by a copy on disk: a hint naming a main that has ended settles it
// — nobody can confirm the grant again, whoever listens at its address now —
// while one naming a main that runs and does not answer may be asked again.
func TestRestoringAsksTheMainTheCopyNames(t *testing.T) {
	dir := t.TempDir()
	run := ownRun(t)
	authority, err := Listen(state.AuthorityAddress(dir, run), os.Getpid(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go authority.Serve(ctx)
	t.Cleanup(func() { cancel(); authority.Close() })
	previous := ended(t)
	granted := lib
	granted.ToEpoch = previous
	address := state.AuthorityAddress(dir, run)
	if err := Register(address, granted); err != nil {
		t.Fatal(err)
	}
	if _, err := Confirm(address, ownExpect(t), lib.ID, lib.To, previous); err != nil {
		t.Fatal(err)
	}
	// Whoever took the address of the main that ended is not asked.
	endedMain := ended(t)
	squatter, err := Listen(state.AuthorityAddress(dir, endedMain), os.Getpid(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	go squatter.Serve(ctx)
	t.Cleanup(squatter.Close)
	silent := stranger(t)
	silentStart, _ := proc.StartTime(silent)
	hints := map[string]grant.Hint{
		"confirmed":  {Message: lib.ID, From: "lead", FromEpoch: run},
		"main ended": {Message: "m9", From: "lead", FromEpoch: endedMain},
		"main quiet": {Message: "m8", From: "lead", FromEpoch: fmt.Sprintf("%d.%d", silent, silentStart)},
		"no main":    {Message: "m7", From: "", FromEpoch: run},
	}
	for name, hint := range hints {
		restored := RestoreHint(dir, lib.To, run, hint)
		switch name {
		case "confirmed":
			if restored.Err != nil || !sameSet(restored.Grant.Dirs, lib.Dirs) {
				t.Errorf("%s: %+v", name, restored)
			}
		case "main quiet":
			if !errors.Is(restored.Err, ErrUnreachable) {
				t.Errorf("%s: %v", name, restored.Err)
			}
		default:
			if !errors.Is(restored.Err, ErrNotConfirmed) {
				t.Errorf("%s: %v", name, restored.Err)
			}
		}
	}
	if restored := RestoreHint(dir, lib.To, run, hints["main ended"]); !strings.Contains(restored.Err.Error(), "has ended") {
		t.Errorf("a main that ended is not named as such: %v", restored.Err)
	}
}
