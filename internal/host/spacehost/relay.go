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

// The attach protocol, over the space's unix socket. The space host answers
// a connection with one byte: accepted, or busy followed by why and the
// close. After acceptance it writes the worker's output as it comes; the
// client writes frames — a kind, a big-endian length of two bytes, the
// payload: keys to type, or a resize of four bytes, columns then rows.
const (
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
// the worker never blocks on output while nobody is attached, and relays it
// to one attached client.
type relay struct {
	pty *terminal.Plain

	mu     sync.Mutex
	client net.Conn
	tail   []byte
}

// pump reads the master until the terminal closes or hangs up.
func (r *relay) pump() {
	buf := make([]byte, 32<<10)
	for {
		n, err := r.pty.Master.Read(buf)
		if n > 0 {
			r.output(buf[:n])
		}
		if err != nil {
			return
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

// serve takes clients until the listener closes, one attached at a time.
func (r *relay) serve(l net.Listener) {
	for {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		// The answer and the client's place are taken under one lock, so no
		// output reaches the client before its acceptance.
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
			continue
		}
		go r.attached(conn)
	}
}

var errBusy = errors.New("busy")

// attached relays one client's frames to the terminal until it leaves.
func (r *relay) attached(conn net.Conn) {
	defer r.detach(conn)
	first := true
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

// resize sets the worker's size to the client's. The first one of an attach
// makes the worker repaint: the kernel signals SIGWINCH only for a size that
// changed, so a client of the size the worker already has is answered by a
// size one row shorter first.
func (r *relay) resize(size terminal.Size, repaint bool) {
	if repaint {
		if current, err := r.pty.Size(); err == nil && current == size && size.Rows > 1 {
			_ = r.pty.Resize(terminal.Size{Cols: size.Cols, Rows: size.Rows - 1})
			time.Sleep(50 * time.Millisecond)
		}
	}
	_ = r.pty.Resize(size)
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
