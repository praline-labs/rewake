package spacehost

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/host/terminal"
)

// served runs program on a relay's terminal, as the space host does, and
// answers the socket its clients dial.
func served(t *testing.T, program string) string {
	t.Helper()
	pty, err := terminal.OpenPlain(terminal.Size{Cols: 60, Rows: 10})
	if err != nil {
		t.Fatal(err)
	}
	grid, err := terminal.NewGrid(terminal.Size{Cols: 60, Rows: 10})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-c", program)
	cmd.Env = terminal.Environ(os.Environ())
	if err := pty.Prepare(cmd); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_ = pty.ReleaseSide()
	// A unix socket's path is short: the test's own directory may be too
	// long for one.
	dir, err := os.MkdirTemp("", "space")
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(dir, "s")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	r := &relay{pty: pty, grid: grid}
	go r.pump()
	go r.answer()
	go r.serve(listener)
	t.Cleanup(func() {
		_ = listener.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = pty.Close()
		_ = grid.Close()
		_ = os.RemoveAll(dir)
	})
	return socket
}

// attach dials socket in the raw mode.
func attach(t *testing.T, socket string) net.Conn {
	t.Helper()
	conn, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if _, err := conn.Write([]byte{modeBytes}); err != nil {
		t.Fatal(err)
	}
	return conn
}

// The raw stream over the socket: the accepting byte, a preamble with the
// screen and the program's paste mode, then the program's live bytes, here
// the terminal's echo of keys the client typed.
func TestTheRawStreamCarriesThePreambleThenLiveBytes(t *testing.T) {
	socket := served(t, `printf '\033[?2004hready\r\n'; exec cat`)
	// A first client sees the text live or in its preamble; either way the
	// emulator took it, so the next client's preamble holds it. The line
	// ends it, so no glyph of it is held back for a mark (terminal.Grid).
	first := attach(t, socket)
	readUntil(t, first, []byte("ready"))
	_ = first.Close()

	conn := attach(t, socket)
	got := readUntil(t, conn, []byte("\x1b[?2004h"))
	if got[0] != accepted || !bytes.Contains(got, []byte("ready")) {
		t.Fatalf("the stream began %q, want the accepting byte and the screen", got)
	}
	if _, err := conn.Write(append([]byte{frameInput, 0, 4}, "zq\r\n"...)); err != nil {
		t.Fatal(err)
	}
	readUntil(t, conn, []byte("zq"))
}

// A size the client sends is the worker's: frames are taken in order, so the
// line typed after it is read by a program on the new size.
func TestAClientSizesTheWorker(t *testing.T) {
	socket := served(t, `read line; stty size; exec cat`)
	conn := attach(t, socket)
	resize := []byte{frameResize, 0, 4, 0, 0, 0, 0}
	binary.BigEndian.PutUint16(resize[3:], 40)
	binary.BigEndian.PutUint16(resize[5:], 8)
	if _, err := conn.Write(append(resize, append([]byte{frameInput, 0, 1}, '\r')...)); err != nil {
		t.Fatal(err)
	}
	readUntil(t, conn, []byte("8 40"))
}

// The grid answers the worker's query itself: a cursor report asked for
// reaches the worker, which prints what it read.
func TestTheGridAnswersAQuery(t *testing.T) {
	socket := served(t, `stty raw -echo; printf '\033[6n'; head -c 4 | od -An -c; sleep 10`)
	readUntil(t, attach(t, socket), []byte("033"))
}

// readUntil reads conn until what was read holds want.
func readUntil(t *testing.T, conn net.Conn, want []byte) []byte {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	var got []byte
	buf := make([]byte, 4096)
	for !bytes.Contains(got, want) {
		n, err := conn.Read(buf)
		got = append(got, buf[:n]...)
		if err != nil && err != io.EOF || err == io.EOF && !bytes.Contains(got, want) {
			t.Fatalf("no %q in %q: %v", want, got, err)
		}
	}
	return got
}
