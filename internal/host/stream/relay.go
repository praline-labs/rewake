// Package stream carries a program's pseudo-terminal to the clients that draw
// it: it reads the master all the time into a terminal.Grid, answers the
// program's queries from the grid, and serves a unix socket whose clients
// take the raw stream or a snapshot (docs/shell.md#how-a-pane-is-fed-the-raw-stream).
// A space host serves a worker's this way, and main's host main's.
package stream

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/praline-labs/rewake/internal/host/terminal"
)

// The unix socket. A client writes one byte first, the mode: the raw bytes
// (bytes.go) for the shell, a snapshot (snapshot.go) for rewake screen, or a
// mode the server adds. The relay answers the first with one byte and then
// writes the screen; the client writes frames — a kind, a big-endian length
// of two bytes, the payload: keys to type, or a resize of four bytes, columns
// then rows. A snapshot is one line of JSON and the end.
const (
	ModeBytes    byte = 'b'
	ModeSnapshot byte = 's'
	Accepted     byte = 'A'
	FrameInput   byte = 'i'
	FrameResize  byte = 'r'
	MaxFrame          = 1 << 15
)

// clientWrite bounds a write to a client: one that stops reading is dropped
// rather than left to hold the program's output.
const clientWrite = 5 * time.Second

// modeWithin is how long a client has to send its mode byte before it is
// dropped; a variable so a test need not wait for it.
var modeWithin atomic.Int64

func init() { modeWithin.Store(int64(clientWrite)) }

// tailBytes is how much of the program's last output is kept: for the line a
// worker that exited by itself last wrote, and for what a main that failed
// inside the shell said.
const tailBytes = 16 << 10

// Relay carries a program's terminal: it reads the master all the time, so
// the program never blocks on output while nobody looks, and feeds it to the
// grid the clients attach to.
type Relay struct {
	pty *terminal.Plain
	// grid is replaced only by a resume after a handover that did not exec
	// (handover.go).
	grid atomic.Pointer[terminal.Grid]
	// defaults are the person's default colors, "fg;bg", for a snapshot.
	defaults string
	// muted drops the grid's answers: another terminal answers the program.
	muted atomic.Bool
	watch func([]byte)
	// gate admits a client's write to the master and a resize, each whole
	// under its read lock, until a handover freezes it (handover.go).
	gate   sync.RWMutex
	frozen bool

	mu   sync.Mutex
	tail []byte
	// pumped, answered and served close when Pump, Answer and Serve return;
	// listener and more are what Serve serves, for a handover to stop and a
	// resume to serve again.
	pumped, answered, served chan struct{}
	listener                 net.Listener
	more                     map[byte]func(net.Conn)
}

// New is the relay of the program on pty, whose screen grid keeps. watch, if
// not nil, sees every read of the master, in order, before the grid takes it.
func New(pty *terminal.Plain, grid *terminal.Grid, defaults string, watch func([]byte)) *Relay {
	r := &Relay{pty: pty, defaults: defaults, watch: watch, pumped: make(chan struct{}), answered: make(chan struct{})}
	r.grid.Store(grid)
	return r
}

// Grid is the screen the relay feeds.
func (r *Relay) Grid() *terminal.Grid { return r.grid.Load() }

// Pump reads the master until the terminal closes or hangs up, or a
// handover stops it.
func (r *Relay) Pump() {
	r.mu.Lock()
	done := r.pumped
	r.mu.Unlock()
	defer close(done)
	grid := r.Grid()
	defer grid.End()
	buf := make([]byte, 32<<10)
	for {
		n, err := r.pty.Master.Read(buf)
		if n > 0 {
			if r.watch != nil {
				r.watch(buf[:n])
			}
			grid.Write(buf[:n])
			r.keepTail(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

// Answer writes the grid's answers to the program's queries: no terminal of
// a person sees the program's output, so the grid is the one to answer —
// unless Mute says one does.
func (r *Relay) Answer() {
	r.mu.Lock()
	done := r.answered
	r.mu.Unlock()
	defer close(done)
	for reply := range r.Grid().Answers() {
		if !r.muted.Load() {
			_, _ = r.pty.Master.Write(reply)
		}
	}
}

// Mute drops the grid's answers while on: the program is drawn by a terminal
// that answers it itself, and two answers to one query would reach it.
func (r *Relay) Mute(on bool) { r.muted.Store(on) }

// Type writes keys to the program, whole, unless a handover froze the
// input (ErrFrozen).
func (r *Relay) Type(p []byte) error {
	return r.admit(func() error {
		_, err := r.pty.Master.Write(p)
		return err
	})
}

func (r *Relay) keepTail(p []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tail = append(r.tail, p...)
	if len(r.tail) > tailBytes {
		r.tail = append(r.tail[:0], r.tail[len(r.tail)-tailBytes:]...)
	}
}

// Serve takes clients until the listener closes. more serves the modes the
// server adds beside the raw stream and the snapshot.
func (r *Relay) Serve(l net.Listener, more map[byte]func(net.Conn)) {
	served := make(chan struct{})
	defer close(served)
	r.mu.Lock()
	r.listener, r.more, r.served = l, more, served
	r.mu.Unlock()
	for {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		go r.take(conn, more)
	}
}

// take reads a client's mode and serves it.
func (r *Relay) take(conn net.Conn, more map[byte]func(net.Conn)) {
	mode := make([]byte, 1)
	_ = conn.SetReadDeadline(time.Now().Add(time.Duration(modeWithin.Load())))
	if _, err := io.ReadFull(conn, mode); err != nil {
		_ = conn.Close()
		return
	}
	_ = conn.SetReadDeadline(time.Time{})
	switch serve := more[mode[0]]; {
	case mode[0] == ModeSnapshot:
		r.snapshot(conn)
	case mode[0] == ModeBytes:
		r.bytes(conn)
	case serve != nil:
		serve(conn)
	default:
		_ = conn.Close()
	}
}

// frames relays a client's frames to the terminal until the client leaves or
// sends one that does not read.
func (r *Relay) frames(conn net.Conn) {
	header := make([]byte, 3)
	for {
		if _, err := io.ReadFull(conn, header); err != nil {
			return
		}
		length := int(binary.BigEndian.Uint16(header[1:]))
		if length > MaxFrame {
			return
		}
		payload := make([]byte, length)
		if _, err := io.ReadFull(conn, payload); err != nil {
			return
		}
		switch header[0] {
		case FrameInput:
			// A frame after a handover's freeze is dropped whole; the client
			// connects again to the next image.
			if err := r.Type(payload); err != nil && !errors.Is(err, ErrFrozen) {
				return
			}
		case FrameResize:
			if len(payload) != 4 {
				return
			}
			r.SetSize(terminal.Size{Cols: int(binary.BigEndian.Uint16(payload)), Rows: int(binary.BigEndian.Uint16(payload[2:]))})
		default:
			return
		}
	}
}

// SetSize sets the program's size, the grid's first; after a handover's
// freeze it is dropped, and the client gives it again on connecting to the
// next image.
func (r *Relay) SetSize(size terminal.Size) {
	_ = r.admit(func() error {
		if err := r.Grid().Resize(size); err != nil {
			return err
		}
		return r.pty.Resize(size)
	})
}

var escapes = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[@-_]`)

// Lines are the last lines of text the program wrote, at most n, its escapes
// taken out and blank lines left out.
func (r *Relay) Lines(n int) []string {
	r.mu.Lock()
	text := escapes.ReplaceAllString(string(r.tail), "")
	r.mu.Unlock()
	var out []string
	for _, line := range strings.FieldsFunc(text, func(c rune) bool { return c == '\n' || c == '\r' }) {
		if line = strings.TrimRight(line, " \t"); strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	if len(out) > n {
		out = out[len(out)-n:]
	}
	return out
}

// LastLine is the last line of text the program wrote, trimmed and cut at
// 300 bytes.
func (r *Relay) LastLine() string {
	lines := r.Lines(1)
	if len(lines) == 0 {
		return ""
	}
	line := strings.TrimSpace(lines[0])
	if len(line) > 300 {
		line = line[:300]
	}
	return line
}
