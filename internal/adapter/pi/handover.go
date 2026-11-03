package pi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// The run across an exec of its wrapper (adapter.Handover,
// docs/research-pi.md#handing-the-run-over). The extension keeps one
// connection for the run's life and has no replay after a reconnect, so the
// connection itself is handed over, with the listener, as files: the
// extension never sees the exec. The handover halts the reader first, so
// nothing the extension sends is taken by this image after the snapshot: it
// waits in the socket for the next, with what the reader took of a frame. What lives only in this image's memory keeps
// the exec from going until it is gone, which an idle session makes it: an
// open turn, whose end only this image would match to its start; an end being
// published, whose answer the extension waits for; a request of the host
// other than the state's poll. The extension's files stay as the first image
// wrote them: the switch admitted only a build of the same wire (Contracts).

// handoverVersion is the version of handedState; an image adopts only its
// own, and a switch of the room admits only a build that takes the same.
const handoverVersion = 2

type handedState struct {
	Version int    `json:"version"`
	PID     int    `json:"pid"`
	Start   uint64 `json:"start"`
	Thread  string `json:"thread"`
	Next    int64  `json:"next"`
	Unread  []byte `json:"unread,omitempty"`
}

// maxUnread bounds the part of a frame a handover carries in the exec's
// environment, which takes at most 128 KiB a variable; a longer one is
// waited for.
const maxUnread = 64 << 10

// errHanding refuses the host's requests while a handover holds the reader.
var errHanding = errors.New("the run is being handed over to rewake's next image")

// Busy answers what keeps the run from being handed over now.
func (r *run) Busy() string {
	l, _ := r.connection()
	if l == nil {
		return "pi's extension has no connection to hand over"
	}
	if r.publishing.Load() > 0 {
		return "a turn's end is being published; the extension waits for its answer"
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	switch {
	case l.turn != "":
		return "a turn is open: its end would find no start in the next image, and pi does not send it again"
	case l.asking > 0:
		return "a request to the extension waits for its answer"
	}
	return ""
}

// HandOver answers the listener and the extension's connection, as copies of
// their descriptors, and what the next image needs to take them. It halts
// the reader and refuses the host's requests first, and looks again: what it
// answers is then all this image holds, until the exec or Resume.
func (r *run) HandOver() (adapter.HandedOver, error) {
	if why := r.Busy(); why != "" {
		return adapter.HandedOver{}, errors.New(why)
	}
	l, thread := r.connection()
	conn, ok := l.conn.(*net.UnixConn)
	if !ok || r.listener == nil {
		return adapter.HandedOver{}, errors.New("the run's sockets are not unix sockets of the state")
	}
	unread, err := l.stop()
	if err != nil {
		return adapter.HandedOver{}, err
	}
	fail := func(err error) (adapter.HandedOver, error) {
		l.start()
		return adapter.HandedOver{}, err
	}
	if why := r.Busy(); why != "" {
		return fail(errors.New(why))
	}
	if len(unread) > maxUnread {
		return fail(errors.New("a long frame of the extension is being read"))
	}
	l.mu.Lock()
	handed := handedState{Version: handoverVersion, PID: r.process.PID, Start: r.process.Start, Thread: thread, Next: l.next, Unread: unread}
	l.mu.Unlock()
	raw, err := json.Marshal(handed)
	if err != nil {
		return fail(err)
	}
	listening, err := r.listener.File()
	if err != nil {
		return fail(err)
	}
	connected, err := conn.File()
	if err != nil {
		_ = listening.Close()
		return fail(err)
	}
	return adapter.HandedOver{State: raw, Files: []*os.File{listening, connected}, Resume: l.start}, nil
}

// stop halts the link's reader at a frame's boundary or inside one, and
// answers what it took of that frame; the host's requests are refused until
// start.
func (l *link) stop() ([]byte, error) {
	l.mu.Lock()
	if l.halt != nil {
		l.mu.Unlock()
		return nil, errors.New("a handover of the run is under way")
	}
	halt := make(chan []byte, 1)
	l.halt, l.resume, l.frozen = halt, make(chan struct{}), true
	l.mu.Unlock()
	if err := l.conn.SetReadDeadline(time.Now()); err != nil {
		l.start()
		return nil, err
	}
	select {
	case unread := <-halt:
		return unread, nil
	case <-l.closed:
		return nil, errClosed
	case <-time.After(2 * time.Second):
		l.start()
		return nil, errors.New("the extension's reader did not halt")
	}
}

// start lets the reader and the host's requests go on after stop.
func (l *link) start() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.halt == nil {
		return
	}
	_ = l.conn.SetReadDeadline(time.Time{})
	close(l.resume)
	l.halt, l.resume, l.frozen = nil, nil, false
}

// halted is the reader's stop for a handover: a read cut by stop's deadline
// gives what it took of a frame and waits; it answers that part to read on
// from once started again, and false for any other error, or a link closed.
func (l *link) halted(err error, rest []byte) ([]byte, bool) {
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		return nil, false
	}
	l.mu.Lock()
	halt, resume := l.halt, l.resume
	l.mu.Unlock()
	if halt == nil {
		return nil, false
	}
	halt <- rest
	select {
	case <-resume:
		return rest, true
	case <-l.closed:
		return nil, false
	}
}

// Adopt runs the service on what an earlier image of the run handed over, in
// place of Start: the listener, and the extension's connection as the run's
// current one, its hello long answered.
func (r *run) Adopt(ctx context.Context, _ func(string), handed adapter.HandedOver) error {
	var got handedState
	if err := json.Unmarshal(handed.State, &got); err != nil || got.Version != handoverVersion || len(handed.Files) != 2 {
		for _, file := range handed.Files {
			_ = file.Close()
		}
		return fmt.Errorf("pi's run was handed over as version %d with %d files; this build takes version %d with 2", got.Version, len(handed.Files), handoverVersion)
	}
	if start, err := proc.StartTime(got.PID); err != nil || start != got.Start {
		_ = handed.Files[0].Close()
		_ = handed.Files[1].Close()
		return errors.New("pi's process handed over no longer runs")
	}
	r.ctx, r.cancel = context.WithCancel(ctx)
	listener, err := state.Inherit(r.socket, handed.Files[0])
	if err != nil {
		_ = handed.Files[1].Close()
		return err
	}
	r.listener = listener
	conn, err := net.FileConn(handed.Files[1])
	_ = handed.Files[1].Close()
	if err != nil {
		return err
	}
	r.HarnessStarted(adapter.RunProcess{PID: got.PID, Start: got.Start})
	l := newLink(conn)
	l.next = got.Next
	r.mu.Lock()
	r.links[l] = struct{}{}
	r.mu.Unlock()
	go r.accept()
	if !r.change(l, frame{Thread: got.Thread}) {
		r.disconnect(l)
		return errors.New("pi's run closed while its connection was adopted")
	}
	go r.pollState(l, got.Thread)
	go r.sendTools(l, got.Thread)
	go func() {
		defer r.disconnect(l)
		// What the last image read of a frame comes first.
		reader := bufio.NewReader(io.MultiReader(bytes.NewReader(got.Unread), conn))
		l.serve(reader, func(request frame) { r.handle(l, request) })
	}()
	r.serveControl()
	return nil
}
