package switching

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/build"
	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// A parked image whose leader is gone decides by the room's build alone: its
// own build resumes it, the switch's target sends it on, anything else is
// lost; and while a dropping mark's leader still runs it waits, as the leader
// may yet have been about to commit, but not for one that is gone.
func TestAParkedImageFollowsTheRecordWithoutItsLeader(t *testing.T) {
	room := buildtest.Dir(t)
	own, err := build.ID()
	if err != nil {
		t.Fatal(err)
	}
	target := strings.Repeat("b", 64)
	park := Park{Room: room, Socket: filepath.Join(t.TempDir(), "gone.sock"), Target: target}
	for _, c := range []struct {
		record, want string
		err          error
	}{
		{own, Resume, nil},
		{target, Go, nil},
		{strings.Repeat("c", 64), "", ErrLost},
	} {
		if err := os.WriteFile(filepath.Join(room, "build"), []byte(c.record+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		decided, err := Wait(park, "session w1")
		if decided != c.want || !errors.Is(err, c.err) {
			t.Errorf("with the build %s: %q, %v", build.Short(c.record), decided, err)
		}
	}
	if err := os.WriteFile(filepath.Join(room, "build"), []byte(target+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	start, _ := proc.StartTime(os.Getpid())
	mark := state.SwitchMark{
		Version: state.SwitchVersion, Leader: os.Getpid(), LeaderStart: start, Target: target,
		Phase: state.PhaseDropping, Deadline: time.Now().Add(400 * time.Millisecond).UnixMilli(),
	}
	if _, err := state.ClaimSwitch(room, mark, func(state.SwitchMark) bool { return true }); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	decided, err := Wait(park, "session w1")
	if decided != Go || err != nil || time.Since(started) < 300*time.Millisecond {
		t.Errorf("under a dropping mark: %q, %v after %s", decided, err, time.Since(started))
	}
	// A leader gone while it dropped commits nothing: the image decides at
	// once, while the mark still keeps launches out, and not at its deadline.
	gone := mark
	gone.LeaderStart, gone.Deadline = 1, time.Now().Add(time.Minute).UnixMilli()
	if _, err := state.ClaimSwitch(room, gone, func(state.SwitchMark) bool { return true }); err != nil {
		t.Fatal(err)
	}
	started = time.Now()
	decided, err = Wait(park, "session w1")
	if decided != Go || err != nil || time.Since(started) > time.Second {
		t.Errorf("under a dropping mark whose leader is gone: %q, %v after %s", decided, err, time.Since(started))
	}
}

// The mark is published before its leader serves the socket, while the
// candidate is staged: a participant that looked in between joins once the
// socket is served, rather than sitting the attempt out and holding its lease
// through the drop.
func TestAParticipantJoinsALeaderThatServesLate(t *testing.T) {
	room := buildtest.Dir(t)
	short, err := os.MkdirTemp("", "sw")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(short) })
	start, _ := proc.StartTime(os.Getpid())
	mark := state.SwitchMark{
		Version: state.SwitchVersion, Leader: os.Getpid(), LeaderStart: start, Target: strings.Repeat("b", 64),
		Socket: filepath.Join(short, "switch.sock"), Phase: state.PhaseAsking, Deadline: time.Now().Add(time.Minute).UnixMilli(),
	}
	if _, err := state.ClaimSwitch(room, mark, func(state.SwitchMark) bool { return true }); err != nil {
		t.Fatal(err)
	}
	stop := Watch(t.Context(), room, CannotHandOver("session w1", "a test"))
	defer stop()
	time.Sleep(3 * poll)
	listener, err := net.Listen("unix", mark.Socket)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	_ = listener.(*net.UnixListener).SetDeadline(time.Now().Add(4 * poll))
	conn, err := listener.Accept()
	if err != nil {
		t.Fatalf("the participant did not join once the socket was served: %v", err)
	}
	defer func() { _ = conn.Close() }()
	var said answer
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	if err := newLine(conn).receive(&said); err != nil || said.Who != "session w1" || said.Answer != Cannot {
		t.Errorf("the participant said %+v, %v", said, err)
	}
}
