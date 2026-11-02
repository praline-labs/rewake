package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/host/switching"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// busyAlways is a participant whose turn never ends.
type busyAlways struct{}

func (busyAlways) Who() string                { return "session lead" }
func (busyAlways) Answer() (string, string)   { return switching.Busy, "a turn is open" }
func (busyAlways) Drop(switching.Park) string { return "not in this test" }

// deferring is a room with a main registered and a participant that stays
// busy, with the deferral's bound and pause short.
func deferring(t *testing.T) (string, registry.Session) {
	t.Helper()
	room := switchRoom(t)
	installed(t, func(*Contracts) {})
	switchBounds = switching.Bounds{Settle: 300 * time.Millisecond, Ask: 400 * time.Millisecond, Drop: time.Second, Drain: time.Second, Confirm: time.Second}
	wasFor, wasPause := deferFor, deferPause
	t.Cleanup(func() { deferFor, deferPause = wasFor, wasPause })
	deferFor, deferPause = 3*time.Second, 200*time.Millisecond
	main := otherSession(t, room, "lead")
	main.Role = role.Main.ID
	if err := registry.Publish(room, main); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	t.Cleanup(switching.Watch(ctx, room, busyAlways{}))
	return room, main
}

// switchNotes are the texts in name's mailbox.
func switchNotes(t *testing.T, room, name string) []string {
	t.Helper()
	var texts []string
	for _, dir := range []string{state.InboxPath(room, name), state.UnreadPath(room, name)} {
		entries, _ := os.ReadDir(dir)
		for _, entry := range entries {
			data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err == nil && strings.Contains(string(data), "rewake restart rewake") {
				texts = append(texts, string(data))
			}
		}
	}
	return texts
}

// requestGone waits for the leader to let its request go.
func requestGone(t *testing.T, room string) {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if _, err := os.Stat(state.SwitchRequestPath(room)); errors.Is(err, os.ErrNotExist) {
			return
		}
	}
	t.Fatal("the deferred switch's request is still there")
}

// A deferred switch tries again while a session stays busy, and gives up
// once its bound has passed: the room as it was, the person told, and main
// told by a note, since the switch was asked on main's behalf.
func TestADeferredSwitchGivesUpAfterItsBound(t *testing.T) {
	room, main := deferring(t)
	before, _ := state.ReadBuildRecord(room)
	started := time.Now()
	code, _, errOut := run("restart", "rewake")
	if code != ExitFailed || !strings.Contains(errOut, "not every session was idle") || !strings.Contains(errOut, "tried again for 3s") {
		t.Errorf("restart rewake: %d %q", code, errOut)
	}
	if took := time.Since(started); took < 2*time.Second {
		t.Errorf("the switch gave up after %s, before its bound", took)
	}
	unswitched(t, room, before)
	requestGone(t, room)
	if got := switchNotes(t, room, main.Name); len(got) != 1 || !strings.Contains(got[0], "rewake was not switched") {
		t.Errorf("main's notes: %q", got)
	}
}

// The switch is bound to main's run when it was asked: once that run ends
// it stops trying, and nothing is said to a later main of the name.
func TestADeferredSwitchEndsWithMainsRun(t *testing.T) {
	room, main := deferring(t)
	go func() {
		time.Sleep(700 * time.Millisecond)
		_, _ = registry.RemoveOwned(room, main.Name, main.Epoch())
	}()
	code, _, errOut := run("restart", "rewake")
	if code != ExitFailed || !strings.Contains(errOut, "main lead ended before the switch could run") {
		t.Errorf("restart rewake: %d %q", code, errOut)
	}
	requestGone(t, room)
	if got := switchNotes(t, room, main.Name); len(got) != 0 {
		t.Errorf("a note went to a main that ended: %q", got)
	}
}

// Asked from a session's turn, which keeps that session busy, the command
// returns at once; one leader waits at a time, and its outcome comes to main.
func TestASwitchAskedFromATurnReturnsAtOnce(t *testing.T) {
	room, main := deferring(t)
	t.Setenv(state.SessionEnv, main.Name)
	t.Setenv(epochEnv, main.Epoch())
	started := time.Now()
	code, out, errOut := run("restart", "rewake")
	if code != ExitOK || !strings.Contains(out, "runs once every session is idle") || time.Since(started) > 2*time.Second {
		t.Errorf("restart rewake from a turn: %d %q %q after %s", code, out, errOut, time.Since(started))
	}
	code, _, errOut = run("restart", "rewake")
	if code != ExitFailed || !strings.Contains(errOut, "a switch already waits") {
		t.Errorf("a second restart rewake: %d %q", code, errOut)
	}
	requestGone(t, room)
	if got := switchNotes(t, room, main.Name); len(got) != 1 || !strings.Contains(got[0], "tried again for 3s") {
		t.Errorf("main's notes: %q", got)
	}
}
