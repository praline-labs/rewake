package stream

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/host/terminal"
)

// execed is what the next image takes of h: each descriptor named copied,
// as the exec keeps it, and every one the old image held closed, as the exec
// closes them.
func execed(t *testing.T, r *Relay, h *Handing) Handover {
	t.Helper()
	dup := func(fd int) int {
		copied, err := syscall.Dup(fd)
		if err != nil {
			t.Fatal(err)
		}
		return copied
	}
	next := Handover{Master: dup(h.Handover.Master), Listener: dup(h.Handover.Listener), Screen: dup(h.Handover.Screen)}
	for _, file := range h.files {
		_ = file.Close()
	}
	_ = r.listener.Close()
	_ = r.pty.Master.Close()
	return next
}

// adopted is the next image's relay, serving.
func adopted(t *testing.T, h Handover, socket string) *Relay {
	t.Helper()
	r, listener, err := Adopt(h, socket, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	go r.Serve(listener, nil)
	t.Cleanup(func() {
		_ = listener.Close()
		_ = r.pty.Master.Close()
		_ = r.Grid().Close()
	})
	return r
}

// screenOf waits until the grid shows want and answers its picture.
func screenOf(t *testing.T, r *Relay, want string) terminal.Snapshot {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		shot := r.Grid().Snapshot()
		if strings.Contains(text(shot), want) {
			return shot
		}
		if time.Now().After(deadline) {
			t.Fatalf("the screen shows %q, not %q", text(shot), want)
		}
	}
}

func text(shot terminal.Snapshot) string {
	var b strings.Builder
	for _, row := range shot.Rows {
		for _, run := range row {
			b.WriteString(run.Text)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// What the program writes between the handover and the adoption waits in
// the kernel's buffer and reaches the next image's screen once; a client
// connected at the handover sees its stream end, connects again to the same
// path before the next image serves, and is served the same screen by it.
func TestAHandoverKeepsTheScreenAndWhatTheProgramWritesMeanwhile(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "go")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	r, socket := relayed(t, `printf '\033[?2004h\033[1;32mbefore\033[0m'; read x < `+fifo+`; printf 'during'; exec cat`)
	screenOf(t, r, "before")
	client := attach(t, socket)
	readUntil(t, client, []byte("before"))

	h, err := r.HandOver(2 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(client); err != nil {
		t.Fatalf("the client's stream did not end at the handover: %v", err)
	}
	if err := os.WriteFile(fifo, []byte("\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	handed := r.Grid().Snapshot()
	next := execed(t, r, h)
	again := attach(t, socket)

	n := adopted(t, next, socket)
	got := readUntil(t, again, []byte("\x1b[?2004h"))
	if got[0] != Accepted || !bytes.Contains(got, []byte("before")) {
		t.Fatalf("the client connected again was served %q", got)
	}
	shot := screenOf(t, n, "during")
	if line := strings.SplitN(text(shot), "\n", 2)[0]; strings.TrimRight(line, " ") != "beforeduring" {
		t.Fatalf("the next image's screen begins %q, want the output before and during the handover, once", line)
	}
	if !reflect.DeepEqual(shot.Rows[0][0], handed.Rows[0][0]) {
		t.Fatalf("the next image draws %+v where the handed screen drew %+v", shot.Rows[0][0], handed.Rows[0][0])
	}
	if lines := n.Lines(1); len(lines) != 1 || lines[0] != "beforeduring" {
		t.Fatalf("the next image's last line is %q", lines)
	}
}

// A paste in flight when a handover begins is written to its end before the
// input freezes; anything after is refused whole, never begun.
func TestAPasteInFlightArrivesWholeAndInputAfterTheFreezeIsRefused(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out")
	paste := "\x1b[200~" + strings.Repeat("p", MaxFrame-12) + "\x1b[201~"
	r, socket := relayed(t, `stty raw -echo; sleep 1; head -c `+strconv.Itoa(len(paste))+` > `+out+`; exec sleep 30`)
	conn := attach(t, socket)
	frame := []byte{FrameInput, 0, 0}
	binary.BigEndian.PutUint16(frame[1:], uint16(len(paste)))
	if _, err := conn.Write(append(frame, paste...)); err != nil {
		t.Fatal(err)
	}
	// The program reads nothing for a second: the kernel takes part of the
	// paste, and the relay's write of the rest is in flight.
	time.Sleep(200 * time.Millisecond)
	h, err := r.HandOver(3 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Type([]byte("after")); !errors.Is(err, ErrFrozen) {
		t.Fatalf("input after the freeze was not refused: %v", err)
	}
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if got, _ := os.ReadFile(out); string(got) == paste {
			break
		} else if time.Now().After(deadline) {
			t.Fatalf("the program read %d bytes of the %d-byte paste", len(got), len(paste))
		}
	}
	if err := h.Resume(); err != nil {
		t.Fatal(err)
	}
	if err := r.Type([]byte("x")); err != nil {
		t.Fatalf("a resumed relay refuses input: %v", err)
	}
}

// A write the program does not take within the bound is not cut: the
// handover is refused, and the relay runs on.
func TestAHandoverIsRefusedWhileAWriteIsStuck(t *testing.T) {
	r, socket := relayed(t, `stty raw -echo; exec sleep 30`)
	conn := attach(t, socket)
	paste := strings.Repeat("q", MaxFrame)
	frame := []byte{FrameInput, 0, 0}
	binary.BigEndian.PutUint16(frame[1:], uint16(len(paste)))
	if _, err := conn.Write(append(frame, paste...)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if _, err := r.HandOver(300 * time.Millisecond); err == nil || !strings.Contains(err.Error(), "still being written") {
		t.Fatalf("the handover with a write stuck answered %v", err)
	}
	r.gate.RLock()
	frozen := r.frozen
	r.gate.RUnlock()
	if frozen {
		t.Fatal("a refused handover left the input frozen")
	}
}

// A relay resumed after an exec that failed serves its clients again on the
// same socket, with the same screen.
func TestAResumedRelayServesTheSameScreen(t *testing.T) {
	r, socket := relayed(t, `printf 'kept'; exec cat`)
	screenOf(t, r, "kept")
	h, err := r.HandOver(2 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	waiting := attach(t, socket)
	if err := h.Resume(); err != nil {
		t.Fatal(err)
	}
	readUntil(t, waiting, []byte("kept"))
	if _, err := waiting.Write(append([]byte{FrameInput, 0, 4}, "zq\r\n"...)); err != nil {
		t.Fatal(err)
	}
	readUntil(t, waiting, []byte("zq"))
}
