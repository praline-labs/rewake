package spacehost

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"
)

// The raw stream over the socket: the accepting byte, a preamble with the
// screen and the program's paste mode, then the program's live bytes, here
// the terminal's echo of keys the client typed.
func TestTheRawStreamCarriesThePreambleThenLiveBytes(t *testing.T) {
	socket := served(t, `printf '\033[?2004hready'; exec cat`)
	// The preamble is taken at attach: wait until the screen holds the text.
	view, err := OpenView(socket)
	if err != nil {
		t.Fatal(err)
	}
	screenWith(t, view, "ready")
	_ = view.Close()

	conn, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.Write([]byte{modeBytes}); err != nil {
		t.Fatal(err)
	}
	got := readUntil(t, conn, []byte("\x1b[?2004h"))
	if got[0] != accepted || !bytes.Contains(got, []byte("ready")) {
		t.Fatalf("the stream began %q, want the accepting byte and the screen", got)
	}
	if _, err := conn.Write(append([]byte{frameInput, 0, 4}, "zq\r\n"...)); err != nil {
		t.Fatal(err)
	}
	readUntil(t, conn, []byte("zq"))
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
