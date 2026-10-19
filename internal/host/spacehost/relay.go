package spacehost

import (
	"encoding/binary"
	"io"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/praline-labs/rewake/internal/host/terminal"
)

// The space's unix socket. A client writes one byte first, the mode: a view
// (view.go) for the host mode (docs/v2/host-mode.md), or the raw bytes
// (bytes.go) for the shell (docs/v2/shell.md). The space host answers it with
// one byte and then writes the screen; the client writes frames — a kind, a big-endian length
// of two bytes, the payload: keys to type, or a resize of four bytes, columns
// then rows.
const (
	modeView    byte = 'v'
	modeBytes   byte = 'b'
	accepted    byte = 'A'
	frameInput  byte = 'i'
	frameResize byte = 'r'
	maxFrame         = 1 << 15
)

// clientWrite bounds a write to a view's client: one that stops reading is
// dropped rather than left to hold the view.
const clientWrite = 5 * time.Second

// tailBytes is how much of the worker's last output is kept, for the line a
// worker that exited by itself last wrote.
const tailBytes = 4 << 10

// relay carries the worker's terminal: it reads the master all the time, so
// the worker never blocks on output while nobody looks, and feeds it to the
// grid the views read.
type relay struct {
	pty  *terminal.Plain
	grid *terminal.Grid

	mu   sync.Mutex
	tail []byte
}

// pump reads the master until the terminal closes or hangs up.
func (r *relay) pump() {
	defer r.grid.End()
	buf := make([]byte, 32<<10)
	for {
		n, err := r.pty.Master.Read(buf)
		if n > 0 {
			r.grid.Write(buf[:n])
			r.keepTail(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

// answer writes the grid's answers to the worker's queries: no terminal of
// a person ever sees the worker's output, so the grid is the one to answer.
func (r *relay) answer() {
	for reply := range r.grid.Answers() {
		_, _ = r.pty.Master.Write(reply)
	}
}

func (r *relay) keepTail(p []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tail = append(r.tail, p...)
	if len(r.tail) > tailBytes {
		r.tail = append(r.tail[:0], r.tail[len(r.tail)-tailBytes:]...)
	}
}

// serve takes views until the listener closes.
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
	if _, err := io.ReadFull(conn, mode); err != nil || (mode[0] != modeView && mode[0] != modeBytes) {
		_ = conn.Close()
		return
	}
	_ = conn.SetReadDeadline(time.Time{})
	if mode[0] == modeBytes {
		r.bytes(conn)
		return
	}
	r.view(conn)
}

// frames relays a client's frames to the terminal until the client leaves or
// sends one that does not read.
func (r *relay) frames(conn net.Conn) {
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
			r.setSize(terminal.Size{Cols: int(binary.BigEndian.Uint16(payload)), Rows: int(binary.BigEndian.Uint16(payload[2:]))})
		default:
			return
		}
	}
}

// setSize sets the worker's size to the client's, the grid's first.
func (r *relay) setSize(size terminal.Size) {
	if r.grid.Resize(size) == nil {
		_ = r.pty.Resize(size)
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
