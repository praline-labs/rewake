package spacehost

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/praline-labs/rewake/internal/host/terminal"
)

// A view, over the space's socket (docs/v2/host-mode.md): the space host
// accepts it at once with one byte, then writes the
// screen as frames — frameScreen, a big-endian length of four bytes, the JSON
// of a terminal.Screen holding the rows changed since the frame before, every
// row in the first — at most one per viewEvery, until the worker's output is
// over and a frame says so. The client writes frames of keys and sizes.
//
// A frame after the first is taken viewEvery after the change that woke it,
// not at once: a program such as Claude Code hides the cursor, draws, and
// shows it again within a few milliseconds, and a screen taken at the first
// change shows it hidden, so the cursor blinked with every frame the program
// drew.
const (
	frameScreen byte = 's'
	viewEvery        = 30 * time.Millisecond
	// maxScreenFrame bounds a screen frame a client reads: a screen of any
	// terminal a person has is far below it.
	maxScreenFrame = 16 << 20
)

// view serves one view until its client leaves or the worker's output ends.
func (r *relay) view(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	_ = conn.SetWriteDeadline(time.Now().Add(clientWrite))
	if _, err := conn.Write([]byte{accepted}); err != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		r.frames(conn)
		cancel()
	}()
	var seq uint64
	for {
		s := r.grid.Screen(ctx, seq)
		if ctx.Err() != nil {
			return
		}
		if seq != 0 && !s.Ended {
			select {
			case <-ctx.Done():
				return
			case <-time.After(viewEvery):
			}
			// Changed since seq, so it answers at once, with the program's
			// frame finished.
			if s = r.grid.Screen(ctx, seq); ctx.Err() != nil {
				return
			}
		}
		data, err := json.Marshal(s)
		if err != nil {
			return
		}
		header := []byte{frameScreen, 0, 0, 0, 0}
		binary.BigEndian.PutUint32(header[1:], uint32(len(data)))
		_ = conn.SetWriteDeadline(time.Now().Add(clientWrite))
		if _, err := conn.Write(append(header, data...)); err != nil || s.Ended {
			return
		}
		seq = s.Seq
	}
}

// View is a client's view of a worker's screen, kept whole from the frames
// that change it, modes included (terminal.Picture).
type View struct {
	conn net.Conn
	w    *frameWriter

	mu      sync.Mutex
	picture terminal.Picture
	err     error
	// wake is closed when the screen changed or the view ended, and
	// replaced.
	wake chan struct{}
}

// OpenView connects to the space host at socket as a view.
func OpenView(socket string) (*View, error) {
	conn, err := net.DialTimeout("unix", socket, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("the space host does not answer at %s: %w", socket, err)
	}
	answer := make([]byte, 1)
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	_, err = conn.Write([]byte{modeView})
	if err == nil {
		_, err = io.ReadFull(conn, answer)
	}
	_ = conn.SetDeadline(time.Time{})
	if err != nil || answer[0] != accepted {
		_ = conn.Close()
		return nil, errors.Join(errors.New("the space host did not take the view"), err)
	}
	v := &View{conn: conn, w: &frameWriter{conn: conn}, wake: make(chan struct{})}
	go v.read()
	return v, nil
}

func (v *View) read() {
	header := make([]byte, 5)
	var err error
	for err == nil {
		if _, err = io.ReadFull(v.conn, header); err != nil {
			break
		}
		length := binary.BigEndian.Uint32(header[1:])
		if header[0] != frameScreen || length > maxScreenFrame {
			err = errors.New("the space host sent a frame that does not read")
			break
		}
		payload := make([]byte, length)
		if _, err = io.ReadFull(v.conn, payload); err != nil {
			break
		}
		var s terminal.Screen
		if err = json.Unmarshal(payload, &s); err == nil {
			v.mu.Lock()
			v.picture.Apply(s)
			v.notifyLocked()
			v.mu.Unlock()
		}
	}
	v.mu.Lock()
	v.err = err
	v.notifyLocked()
	v.mu.Unlock()
}

func (v *View) notifyLocked() {
	close(v.wake)
	v.wake = make(chan struct{})
}

// Picture waits until the screen's sequence passes after, the worker's
// output ends or ctx does, and answers the screen as it stands. A view whose
// connection ended answers its last screen with the error, once it has none
// newer.
func (v *View) Picture(ctx context.Context, after uint64) (terminal.Picture, error) {
	for {
		v.mu.Lock()
		p, err, wake := v.picture, v.err, v.wake
		v.mu.Unlock()
		if p.Ended {
			return p, nil
		}
		if p.Seq != after && p.Rows != nil {
			return p, nil
		}
		if err != nil {
			return p, err
		}
		select {
		case <-wake:
		case <-ctx.Done():
			return p, nil
		}
	}
}

// Keys writes bytes to the worker as typed.
func (v *View) Keys(p []byte) error { return v.w.frame(frameInput, p) }

// Resize sets the worker's size.
func (v *View) Resize(size terminal.Size) error {
	if size.Cols < 1 || size.Rows < 1 || size.Cols > 1<<16-1 || size.Rows > 1<<16-1 {
		return fmt.Errorf("a terminal of %dx%d is no size", size.Cols, size.Rows)
	}
	payload := make([]byte, 4)
	binary.BigEndian.PutUint16(payload, uint16(size.Cols))
	binary.BigEndian.PutUint16(payload[2:], uint16(size.Rows))
	return v.w.frame(frameResize, payload)
}

// Close ends the view.
func (v *View) Close() error { return v.conn.Close() }

// frameWriter writes the client's frames one at a time: keys and resizes
// come from two goroutines. A write is bounded, so a space host that stops
// reading cannot hold the person's keys.
type frameWriter struct {
	mu   sync.Mutex
	conn net.Conn
}

func (w *frameWriter) frame(kind byte, payload []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	for len(payload) > 0 || kind == frameResize {
		part := payload
		if len(part) > maxFrame {
			part = part[:maxFrame]
		}
		header := []byte{kind, 0, 0}
		binary.BigEndian.PutUint16(header[1:], uint16(len(part)))
		_ = w.conn.SetWriteDeadline(time.Now().Add(clientWrite))
		if _, err := w.conn.Write(append(header, part...)); err != nil {
			return err
		}
		payload = payload[len(part):]
		if kind == frameResize {
			return nil
		}
	}
	return nil
}
