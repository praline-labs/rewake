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

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/host/terminal"
)

// A view, over the space's socket (docs/v2/workers-panel.md): the space host
// accepts it at once with one byte, whatever is attached, then writes the
// screen as frames — frameScreen, a big-endian length of four bytes, the JSON
// of a terminal.Screen holding the rows changed since the frame before, every
// row in the first — at most one per viewEvery, until the worker's output is
// over and a frame says so. The client writes an attach's frames.
//
// A frame after the first is taken viewEvery after the change that woke it,
// not at once: a program such as Claude Code hides the cursor, draws, and
// shows it again within a few milliseconds, and a screen taken at the first
// change shows it hidden, so the panel's cursor blinked with every frame the
// program drew.
const (
	frameScreen byte = 's'
	viewEvery        = 30 * time.Millisecond
	// maxScreenFrame bounds a screen frame a client reads: a screen of the
	// largest size the panel asks for is far below it.
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
		r.frames(conn, false)
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
// that change it.
type View struct {
	conn net.Conn
	w    *frameWriter

	mu     sync.Mutex
	screen adapter.Screen
	err    error
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
			v.take(s)
		}
	}
	v.mu.Lock()
	v.err = err
	v.notifyLocked()
	v.mu.Unlock()
}

// take applies a frame: a new size starts the screen over, since every row
// comes with it.
func (v *View) take(s terminal.Screen) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if s.Size.Cols != v.screen.Cols || s.Size.Rows != v.screen.Rows || v.screen.Lines == nil {
		v.screen.Cols, v.screen.Rows = s.Size.Cols, s.Size.Rows
		v.screen.Lines = make([][]adapter.ScreenRun, s.Size.Rows)
	} else {
		// The lines are replaced, never written in place: a screen handed
		// out keeps the rows it had.
		v.screen.Lines = append([][]adapter.ScreenRun(nil), v.screen.Lines...)
	}
	for _, row := range s.Changed {
		if row.Index >= 0 && row.Index < len(v.screen.Lines) {
			v.screen.Lines[row.Index] = runsOf(row.Runs)
		}
	}
	v.screen.Seq, v.screen.Ended = s.Seq, s.Ended
	v.screen.Cursor = adapter.ScreenCursor{Row: s.Cursor.Row, Col: s.Cursor.Col, Visible: s.Cursor.Visible}
	v.notifyLocked()
}

func (v *View) notifyLocked() {
	close(v.wake)
	v.wake = make(chan struct{})
}

// Screen waits until the screen's sequence passes after, the worker's output
// ends or ctx does, and answers the screen as it stands. A view whose
// connection ended answers its last screen with the error, once it has none
// newer.
func (v *View) Screen(ctx context.Context, after uint64) (adapter.Screen, error) {
	for {
		v.mu.Lock()
		s, err, wake := v.screen, v.err, v.wake
		v.mu.Unlock()
		if s.Ended {
			return s, nil
		}
		if s.Seq != after && s.Lines != nil {
			return s, nil
		}
		if err != nil {
			return s, err
		}
		select {
		case <-wake:
		case <-ctx.Done():
			return s, nil
		}
	}
}

// Keys writes bytes to the worker as typed.
func (v *View) Keys(p []byte) error { return v.w.frame(frameInput, p) }

// Resize sets the worker's size.
func (v *View) Resize(cols, rows int) error {
	if cols < 1 || rows < 1 || cols > 1<<16-1 || rows > 1<<16-1 {
		return fmt.Errorf("a terminal of %dx%d is no size", cols, rows)
	}
	payload := make([]byte, 4)
	binary.BigEndian.PutUint16(payload, uint16(cols))
	binary.BigEndian.PutUint16(payload[2:], uint16(rows))
	return v.w.frame(frameResize, payload)
}

// Close ends the view.
func (v *View) Close() error { return v.conn.Close() }

func runsOf(runs []terminal.Run) []adapter.ScreenRun {
	out := make([]adapter.ScreenRun, len(runs))
	for i, run := range runs {
		out[i] = adapter.ScreenRun{Text: run.Text, Cols: run.Cols, Fg: colorOf(run.Fg), Bg: colorOf(run.Bg), Attrs: adapter.ScreenAttrs(run.Attrs)}
	}
	return out
}

func colorOf(c terminal.Color) adapter.ScreenColor {
	return adapter.ScreenColor{Kind: adapter.ScreenColorKind(c.Kind), Index: c.Index, R: c.R, G: c.G, B: c.B}
}
