package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/host/switching"
	"github.com/praline-labs/rewake/internal/infra/build"
	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// installAt makes dir the install this test's rewake reads.
func installAt(t *testing.T, dir string) {
	t.Helper()
	was := installedDir
	t.Cleanup(func() { installedDir = was })
	installedDir = func() string { return dir }
}

// installSelf makes this test's binary the installed rewake: the room's build.
func installSelf(t *testing.T) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Symlink(exe, filepath.Join(dir, "rewake")); err != nil {
		t.Fatal(err)
	}
	installAt(t, dir)
}

// installed puts an installed rewake that is another build — a script —
// speaking contracts as change says over this build's.
func installed(t *testing.T, change func(*Contracts)) {
	t.Helper()
	contracts := contractsOf(testCatalog())
	change(&contracts)
	data, err := json.Marshal(map[string]any{"contracts": contracts})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	script := "#!/bin/sh\nprintf '%s\\n' '" + string(data) + "'\n"
	if err := os.WriteFile(filepath.Join(dir, "rewake"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	installAt(t, dir)
}

func switchRoom(t *testing.T) string {
	t.Helper()
	// Short, for the switch's socket.
	short, err := os.MkdirTemp("", "sw")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(short) })
	t.Setenv(state.DirEnv, short)
	t.Setenv(state.RoomEnv, "")
	t.Setenv(state.SessionEnv, "")
	t.Setenv(epochEnv, "")
	room := buildtest.Current(t)
	was := switchBounds
	t.Cleanup(func() { switchBounds = was })
	switchBounds = switching.Bounds{Settle: 300 * time.Millisecond, Ask: 3 * time.Second, Drop: time.Second, Drain: time.Second, Confirm: time.Second}
	return room
}

// unswitched holds a refused switch to what it promises: the build as it was
// and no mark left.
func unswitched(t *testing.T, room, before string) {
	t.Helper()
	if after, err := state.ReadBuildRecord(room); err != nil || after != before {
		t.Errorf("the build is %q (%v), was %q", after, err, before)
	}
	if _, err := state.ReadSwitchMark(room); !errors.Is(err, state.ErrNoSwitchMark) {
		t.Errorf("a refused switch left its mark: %v", err)
	}
}

// counting is a participant that counts how often it is asked.
type counting struct{ asked atomic.Int32 }

func (c *counting) Who() string { return "a counting participant" }
func (c *counting) Answer() (string, string) {
	c.asked.Add(1)
	return switching.Ready, ""
}
func (c *counting) Drop(switching.Park) string { return "not in this test" }

// A target whose contracts are not the room build's is refused before any
// participant is asked — the handover's own and an adapter's alike — and the
// room stays as it was.
func TestRestartRewakeRefusesAnotherContract(t *testing.T) {
	room := switchRoom(t)
	before, _ := state.ReadBuildRecord(room)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	participant := &counting{}
	defer switching.Watch(ctx, room, participant)()
	for name, change := range map[string]func(*Contracts){
		"handover 99":        func(c *Contracts) { c.Speaks[handoverContract] = "99" },
		"handover.fixture 2": func(c *Contracts) { c.Speaks["handover.fixture"] = "2" },
		"switch 99":          func(c *Contracts) { c.Speaks[switchContract] = "99" },
	} {
		installed(t, change)
		code, _, errOut := run("restart", "rewake")
		if code != ExitFailed || !strings.Contains(errOut, "the installed rewake speaks "+name) {
			t.Errorf("%s: %d %q", name, code, errOut)
		}
		unswitched(t, room, before)
	}
	time.Sleep(600 * time.Millisecond)
	if n := participant.asked.Load(); n != 0 {
		t.Errorf("a participant was asked %d times by switches refused in their preflight", n)
	}
}

// A process that cannot hand over refuses the switch at its asking, named
// with why, and the room stays as it was.
func TestRestartRewakeNamesWhoCannotHandOver(t *testing.T) {
	room := switchRoom(t)
	before, _ := state.ReadBuildRecord(room)
	installed(t, func(*Contracts) {})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer switching.Watch(ctx, room, switching.CannotHandOver("the space host of s1", "a space host does not hand its run over yet"))()
	code, _, errOut := run("restart", "rewake")
	if code != ExitFailed || !strings.Contains(errOut, "restart the room: the space host of s1 cannot: a space host does not hand its run over yet") {
		t.Errorf("restart rewake: %d %q", code, errOut)
	}
	unswitched(t, room, before)
}

// A question's wait holds the room's lease, so a switch that drops its holders
// ends it unanswered — the question stays open — rather than wait for it.
func TestAQuestionEndsItsWaitAtADrop(t *testing.T) {
	room := switchRoom(t)
	base, err := state.Base()
	if err != nil {
		t.Fatal(err)
	}
	lease, err := build.Acquire(base, filepath.Base(room))
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Release()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stopped := make(chan struct{})
	refused := make(chan refusal, 1)
	go watchRefusal(ctx, refusalWatch{dir: room}, refused, func() { close(stopped) })
	sw, err := build.Claim(lease, strings.Repeat("b", 64), state.SocketPath(room, "switch", ""), time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sw.End() }()
	select {
	case <-stopped:
		t.Fatal("the wait ended while the switch was only asking")
	case <-time.After(600 * time.Millisecond):
	}
	if err := sw.Phase(state.PhaseDropping, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stopped:
	case <-ctx.Done():
		t.Fatal("the wait went on through the drop")
	}
	if len(refused) != 0 {
		t.Error("a wait ended by a drop said the question failed")
	}
}
