//go:build rewaketest

package fixture

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// The run across an exec of its wrapper (adapter.Handover), the probe of
// .scratch/reexec-probe.md. The listener and the program's connection are
// handed over as files; the program is the wrapper's child still, by pid and
// start; the connection's hello and the request numbering go in the state.
// The probes run again on the adopted connection, which proves it, and the
// briefing is not sent again. What a turn or a delivery holds in memory is
// not handed over: Busy refuses while there is any.

// handoverVersion is the version of handedState; an image adopts only its own.
const handoverVersion = 1

type handedState struct {
	Version int      `json:"version"`
	PID     int      `json:"pid"`
	Start   uint64   `json:"start"`
	Thread  string   `json:"thread"`
	Program string   `json:"program"`
	Serves  []string `json:"serves"`
	Next    int64    `json:"next"`
}

// Busy answers what keeps the run from being handed over now.
func (b *backend) Busy() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch {
	case b.closing:
		return "the run is closing"
	case b.link == nil:
		return "the program has no connection to hand over"
	case len(b.open) > 0:
		return "a turn of the program is open: its end is captured only in this image"
	case b.inflight.Load() > 0:
		return "a call into the wrapper's handler is under way"
	case b.link.pending() > 0:
		return "a request to the program waits for its answer"
	}
	return ""
}

// HandOver answers the listener and the connection, as copies of their
// descriptors, and what the next image needs to take them.
func (b *backend) HandOver() (adapter.HandedOver, error) {
	if why := b.Busy(); why != "" {
		return adapter.HandedOver{}, errors.New(why)
	}
	b.mu.Lock()
	l := b.link
	handed := handedState{Version: handoverVersion, PID: b.pid, Start: b.start, Thread: b.thread, Program: b.version, Serves: b.serves}
	b.mu.Unlock()
	listener, ok := b.listener.(*state.Listener)
	conn, connOK := l.conn.(*net.UnixConn)
	if !ok || !connOK {
		return adapter.HandedOver{}, errors.New("the run's sockets are not unix sockets of the state")
	}
	l.mu.Lock()
	handed.Next = l.next
	l.mu.Unlock()
	raw, err := json.Marshal(handed)
	if err != nil {
		return adapter.HandedOver{}, err
	}
	listening, err := listener.File()
	if err != nil {
		return adapter.HandedOver{}, err
	}
	connected, err := conn.File()
	if err != nil {
		_ = listening.Close()
		return adapter.HandedOver{}, err
	}
	return adapter.HandedOver{State: raw, Files: []*os.File{listening, connected}}, nil
}

// Adopt runs the backend on what an earlier image handed over: the program it
// started, the listener and the connection.
func (b *backend) Adopt(ctx context.Context, note func(string), handed adapter.HandedOver) error {
	var got handedState
	if err := json.Unmarshal(handed.State, &got); err != nil {
		return fmt.Errorf("the fixture's handover cannot be read: %w", err)
	}
	if got.Version != handoverVersion || len(handed.Files) != 2 {
		return fmt.Errorf("the fixture's handover is version %d with %d files; this image takes version %d with 2", got.Version, len(handed.Files), handoverVersion)
	}
	if start, err := proc.StartTime(got.PID); err != nil || start != got.Start {
		return errors.New("the fixture's program handed over no longer runs")
	}
	b.note = note
	if b.note == nil {
		b.note = func(string) {}
	}
	b.ctx, b.cancel = context.WithCancel(ctx)
	listener, err := state.Inherit(b.socket, handed.Files[0])
	if err != nil {
		_ = handed.Files[1].Close()
		b.cancel()
		return err
	}
	conn, err := net.FileConn(handed.Files[1])
	_ = handed.Files[1].Close()
	if err != nil {
		_ = listener.Close()
		b.cancel()
		return err
	}
	b.listener, b.pid, b.start = listener, got.PID, got.Start
	// Told in the first image; a second briefing would be a second message.
	b.brief = nil
	go b.reap()
	b.routines.Add(2)
	go b.accept()
	l := newLink(conn)
	l.next = got.Next
	go func() {
		defer b.routines.Done()
		b.attach(l, bufio.NewReader(conn), Frame{Op: opHello, Thread: got.Thread, Version: got.Program, Serves: got.Serves})
	}()
	select {
	case <-b.hello:
		b.serveControl()
		return nil
	case <-b.exited:
		b.Close()
		return errors.New("the fixture ended before its adopted connection was probed")
	case <-ctx.Done():
		b.Close()
		return ctx.Err()
	}
}

// reap waits for the adopted program, which is this process's child still,
// as spawn's Wait does for one it started.
func (b *backend) reap() {
	for {
		var status syscall.WaitStatus
		_, err := syscall.Wait4(b.pid, &status, 0, nil)
		if !errors.Is(err, syscall.EINTR) {
			break
		}
	}
	close(b.exited)
	b.withdraw(nil)
	b.cancel()
}

// pending counts the requests waiting for their answers.
func (l *link) pending() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.waiting)
}
