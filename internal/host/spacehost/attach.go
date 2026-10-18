package spacehost

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/praline-labs/rewake/internal/host/terminal"
)

// DetachKey detaches a client and leaves the worker running: Ctrl-\, the
// key dtach detaches with. In raw mode it is a byte like any other, and a
// harness's interface has no use for it that the attach would take away.
const DetachKey = 0x1c

// DetachKeyName is DetachKey as the person is told it.
const DetachKeyName = `Ctrl-\`

// ErrBusy is a space another client is attached to.
var ErrBusy = errors.New("another client is attached to this space; detach it first")

// Attach connects the person's terminal to the worker's until the person
// presses DetachKey, answering true, or the worker's terminal closes,
// answering false. The person's terminal is in raw mode meanwhile, so every
// key reaches the worker, and its size follows the person's window.
func Attach(socket string, in, out *os.File) (detached bool, err error) {
	conn, err := net.DialTimeout("unix", socket, 5*time.Second)
	if err != nil {
		return false, fmt.Errorf("the space host does not answer at %s: %w", socket, err)
	}
	defer func() { _ = conn.Close() }()
	answer := make([]byte, 1)
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write([]byte{modeAttach}); err != nil {
		return false, fmt.Errorf("the space host did not take the attach: %w", err)
	}
	if _, err := io.ReadFull(conn, answer); err != nil {
		return false, fmt.Errorf("the space host did not take the attach: %w", err)
	}
	_ = conn.SetDeadline(time.Time{})
	if answer[0] != accepted {
		return false, ErrBusy
	}
	restore, err := terminal.MakeRaw(in)
	if err != nil {
		return false, fmt.Errorf("cannot put this terminal in raw mode: %w", err)
	}
	defer func() { _ = restore() }()

	w := &frameWriter{conn: conn}
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	defer signal.Stop(winch)
	sendSize := func() {
		if size, err := terminal.SizeOf(out); err == nil {
			payload := make([]byte, 4)
			binary.BigEndian.PutUint16(payload, uint16(size.Cols))
			binary.BigEndian.PutUint16(payload[2:], uint16(size.Rows))
			_ = w.frame(frameResize, payload)
		}
	}
	sendSize()
	go func() {
		for range winch {
			sendSize()
		}
	}()

	closed := make(chan struct{})
	go func() {
		_, _ = io.Copy(out, conn)
		close(closed)
	}()
	keys := make(chan bool, 1)
	go func() { keys <- relayKeys(in, w) }()
	select {
	case <-closed:
		return false, nil
	case detached := <-keys:
		return detached, nil
	}
}

// relayKeys sends what the person types until DetachKey, answering true, or
// until the terminal or the connection fails.
func relayKeys(in io.Reader, w *frameWriter) bool {
	buf := make([]byte, 4096)
	for {
		n, err := in.Read(buf)
		chunk := buf[:n]
		if at := bytes.IndexByte(chunk, DetachKey); at >= 0 {
			if at > 0 {
				_ = w.frame(frameInput, chunk[:at])
			}
			return true
		}
		if n > 0 && w.frame(frameInput, chunk) != nil {
			return false
		}
		if err != nil {
			return false
		}
	}
}

// frameWriter writes the client's frames one at a time: keys and resizes
// come from two goroutines.
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
