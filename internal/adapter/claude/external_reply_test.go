package claude

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// A socket the caller named lies wherever they put it, outside the room, and
// the receipt protocol wants the reply socket beside it. The run owns that one
// path while it holds the room's lease: a dead run's socket there is cleared,
// a held line and a refused one are told, and the reply socket goes when the
// lane closes — through the launch plan, as a launch builds it.
func TestACallerSocketOutsideTheRoomKeepsReceipts(t *testing.T) {
	for _, status := range []string{"held", "refused"} {
		t.Run(status, func(t *testing.T) {
			room := buildtest.Dir(t)
			outside := outsideDir(t)
			r := startReceiver(t, outside, status)
			request := adapter.LaunchRequest{Name: "api", Epoch: "1.2", Dir: t.TempDir(), RoomDir: room, Args: []string{socketFlag, r.socket}}
			reply := replyPath(request, r.socket, false)
			deadSocket(t, reply)
			plan, err := New().Launch(request)
			if err != nil {
				t.Fatal(err)
			}
			defer plan.Service.Close()
			if err := plan.Service.Start(context.Background(), func(note string) { t.Fatalf("the start noted: %s", note) }); err != nil {
				t.Fatal(err)
			}
			result := plan.Wake.Deliver(context.Background(), mail.Message{ID: "m1", Kind: mail.Task, Text: "work"})
			want := map[string]mail.State{"held": mail.Held, "refused": mail.Failed}[status]
			if result.State != want {
				t.Fatalf("a %s line beside a caller's socket outside the room: %+v", status, result)
			}
			if line := <-r.lines; line["from"] != "uds:"+reply {
				t.Fatalf("the line asked for receipts at %v, want %s", line["from"], reply)
			}
			plan.Service.Close()
			if _, err := os.Lstat(reply); !os.IsNotExist(err) {
				t.Fatalf("the reply socket outlived the lane: %v", err)
			}
		})
	}
}

// The lane's claim is scoped as the run is: with the room's lease let go it
// binds nothing beside the caller's socket and says why; the gate still opens.
func TestALaneOutsideTheRoomBindsNothingWithoutTheLease(t *testing.T) {
	room := buildtest.Dir(t)
	outside := outsideDir(t)
	l := newLane(outside+"/session.sock", outside+"/session.reply.sock", false)
	l.room, l.limit = room, 50*time.Millisecond
	defer l.Close()
	buildtest.Release(room)
	before := state.NoLeaseRefusals()
	var noted string
	if err := l.Start(context.Background(), func(note string) { noted = note }); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(noted, state.ErrNoLease.Error()) || state.NoLeaseRefusals() == before {
		t.Fatalf("a lane without the lease: noted %q", noted)
	}
	if _, err := os.Lstat(l.reply); !os.IsNotExist(err) {
		t.Fatalf("the reply socket was bound without the lease: %v", err)
	}
	select {
	case <-l.Opened():
	case <-l.done:
		t.Fatal("closed before it opened")
	}
}

// A caller's socket in a room of another base is in a state, not outside
// every one: the room's lease the lane holds clears nothing there and binds
// nothing, and the lane says why; the gate still opens.
func TestALaneBesideAnotherBasesRoomWritesNothingThere(t *testing.T) {
	room := buildtest.Dir(t)
	foreign := buildtest.Established(t, outsideDir(t), state.DefaultRoom)
	sock := state.SocketPath(foreign, "api", "1.2")
	l := newLane(sock, strings.TrimSuffix(sock, ".sock")+".reply.sock", false)
	l.room, l.limit = room, 50*time.Millisecond
	defer l.Close()
	deadSocket(t, l.reply)
	before := state.NoLeaseRefusals()
	var noted string
	if err := l.Start(context.Background(), func(note string) { noted = note }); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(noted, state.ErrNoLease.Error()) || state.NoLeaseRefusals() == before {
		t.Fatalf("a lane beside another base's room: noted %q", noted)
	}
	if info, err := os.Lstat(l.reply); err != nil || info.Mode()&os.ModeSocket == 0 {
		t.Fatalf("the dead socket in another base's room was not left: %v", err)
	}
	select {
	case <-l.Opened():
	case <-l.done:
		t.Fatal("closed before it opened")
	}
}

// A file at the reply path that is no socket is not the run's to remove.
func TestALaneOutsideTheRoomLeavesWhatIsNoSocket(t *testing.T) {
	room := buildtest.Dir(t)
	outside := outsideDir(t)
	l := newLane(outside+"/session.sock", outside+"/session.reply.sock", false)
	l.room = room
	defer l.Close()
	if err := os.WriteFile(l.reply, []byte("theirs"), 0o600); err != nil {
		t.Fatal(err)
	}
	var noted string
	if err := l.Start(context.Background(), func(note string) { noted = note }); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(l.reply); err != nil || string(data) != "theirs" || !strings.Contains(noted, "not a socket") {
		t.Fatalf("a file at the reply path: %q %v, noted %q", data, err, noted)
	}
}

// outsideDir is a short directory outside every room, where a caller might
// put their socket.
func outsideDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "caller")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// deadSocket leaves at path what a killed run leaves: a socket nobody listens on.
func deadSocket(t *testing.T, path string) {
	t.Helper()
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	listener.SetUnlinkOnClose(false)
	if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		t.Fatal(err)
	}
}
