package spacehost

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/praline-labs/rewake/internal/host/terminal"
)

// The space's unix socket. A client writes one byte first, the mode: an
// attach or a view (view.go). The space host answers an attach with one
// byte: accepted, or busy followed by the close. After acceptance it writes
// the worker's output as it comes; the client writes frames — a kind, a
// big-endian length of two bytes, the payload: keys to type, or a resize of
// four bytes, columns then rows. A view's client writes the same frames.
const (
	modeAttach  byte = 'a'
	modeView    byte = 'v'
	accepted    byte = 'A'
	busy        byte = 'B'
	frameInput  byte = 'i'
	frameResize byte = 'r'
	maxFrame         = 1 << 15
)

// clientWrite bounds a write to the attached client: one that stops reading
// is detached rather than left to block the worker's output.
const clientWrite = 5 * time.Second

// tailBytes is how much of the worker's last output is kept, for the line a
// worker that exited by itself last wrote.
const tailBytes = 4 << 10

// relay carries the worker's terminal: it reads the master all the time, so
// the worker never blocks on output while nobody is attached, relays it to
// one attached client and feeds it to the grid the views read.
type relay struct {
	pty  *terminal.Plain
	grid *terminal.Grid

	mu     sync.Mutex
	client net.Conn
	tail   []byte
}

// pump reads the master until the terminal closes or hangs up.
func (r *relay) pump() {
	defer r.grid.End()
	buf := make([]byte, 32<<10)
	for {
		n, err := r.pty.Master.Read(buf)
		if n > 0 {
			r.grid.Write(buf[:n])
			r.output(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

// answer writes the grid's answers to the worker's queries while nobody is
// attached: an attached terminal answers them itself, and two answers would
// both reach the worker.
func (r *relay) answer() {
	for reply := range r.grid.Answers() {
		r.mu.Lock()
		attached := r.client != nil
		r.mu.Unlock()
		if !attached {
			_, _ = r.pty.Master.Write(reply)
		}
	}
}

func (r *relay) output(p []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tail = append(r.tail, p...)
	if len(r.tail) > tailBytes {
		r.tail = append(r.tail[:0], r.tail[len(r.tail)-tailBytes:]...)
	}
	if r.client == nil {
		return
	}
	_ = r.client.SetWriteDeadline(time.Now().Add(clientWrite))
	if _, err := r.client.Write(p); err != nil {
		_ = r.client.Close()
		r.client = nil
	}
}

// serve takes clients until the listener closes, one attached at a time
// and any number of views.
func (r *relay) serve(l net.Listener) {
	for {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		go r.take(conn)
	}
}

// take reads a client's mode and serves it.
func (r *relay) take(conn net.Conn) {
	mode := make([]byte, 1)
	_ = conn.SetReadDeadline(time.Now().Add(clientWrite))
	if _, err := io.ReadFull(conn, mode); err != nil {
		_ = conn.Close()
		return
	}
	_ = conn.SetReadDeadline(time.Time{})
	switch mode[0] {
	case modeAttach:
		r.attach(conn)
	case modeView:
		r.view(conn)
	default:
		_ = conn.Close()
	}
}

// attach takes one client's attach, or refuses it busy. The answer and the
// client's place are taken under one lock, so no output reaches the client
// before its acceptance.
func (r *relay) attach(conn net.Conn) {
	var err error
	r.mu.Lock()
	_ = conn.SetWriteDeadline(time.Now().Add(clientWrite))
	if r.client != nil {
		_, _ = conn.Write([]byte{busy})
		err = errBusy
	} else if _, err = conn.Write([]byte{accepted}); err == nil {
		r.client = conn
	}
	r.mu.Unlock()
	if err != nil {
		_ = conn.Close()
		return
	}
	r.attached(conn)
}

var errBusy = errors.New("busy")

// attached relays one client's frames to the terminal until it leaves.
func (r *relay) attached(conn net.Conn) {
	defer r.detach(conn)
	r.frames(conn, true)
}

// frames relays a client's frames to the terminal until the client leaves or
// sends one that does not read. The first resize of an attach makes the
// worker repaint; a view reads the grid, which needs none.
func (r *relay) frames(conn net.Conn, attach bool) {
	first := attach
	header := make([]byte, 3)
	for {
		if _, err := io.ReadFull(conn, header); err != nil {
			return
		}
		length := int(binary.BigEndian.Uint16(header[1:]))
		if length > maxFrame {
			return
		}
		payload := make([]byte, length)
		if _, err := io.ReadFull(conn, payload); err != nil {
			return
		}
		switch header[0] {
		case frameInput:
			if _, err := r.pty.Master.Write(payload); err != nil {
				return
			}
		case frameResize:
			if len(payload) != 4 {
				return
			}
			size := terminal.Size{Cols: int(binary.BigEndian.Uint16(payload)), Rows: int(binary.BigEndian.Uint16(payload[2:]))}
			r.resize(size, first)
			first = false
		default:
			return
		}
	}
}

// resize sets the worker's size to the client's, the grid's first. The
// first one of an attach makes the worker repaint: the kernel signals
// SIGWINCH only for a size that changed, so a client of the size the worker
// already has is answered by a size one row shorter first.
func (r *relay) resize(size terminal.Size, repaint bool) {
	if repaint {
		if current, err := r.pty.Size(); err == nil && current == size && size.Rows > 1 {
			r.setSize(terminal.Size{Cols: size.Cols, Rows: size.Rows - 1})
			time.Sleep(50 * time.Millisecond)
		}
	}
	r.setSize(size)
}

func (r *relay) setSize(size terminal.Size) {
	if r.grid.Resize(size) == nil {
		_ = r.pty.Resize(size)
	}
}

func (r *relay) detach(conn net.Conn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.client == conn {
		r.client = nil
	}
	_ = conn.Close()
}

// close detaches the client, if any, once the worker is gone.
func (r *relay) close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.client != nil {
		_ = r.client.Close()
		r.client = nil
	}
}

var escapes = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[@-_]`)

// lastLine is the last line of text the worker wrote, its escapes taken out.
func (r *relay) lastLine() string {
	r.mu.Lock()
	text := escapes.ReplaceAllString(string(r.tail), "")
	r.mu.Unlock()
	lines := strings.FieldsFunc(text, func(c rune) bool { return c == '\n' || c == '\r' })
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			if len(line) > 300 {
				line = line[:300]
			}
			return line
		}
	}
	return ""
}
