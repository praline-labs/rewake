package spacehost

import (
	"context"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
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
	dir, err := os.MkdirTemp("", "view")
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
		r.close()
		_ = pty.Close()
		_ = grid.Close()
		_ = os.RemoveAll(dir)
	})
	return socket
}

// screenWith waits for a screen whose text holds want.
func screenWith(t *testing.T, view *View, want string) adapter.Screen {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var seq uint64
	for {
		s, err := view.Screen(ctx, seq)
		if strings.Contains(textOf(s), want) {
			return s
		}
		if err != nil || ctx.Err() != nil || s.Ended {
			t.Fatalf("no screen with %q (err %v), the last:\n%s", want, err, textOf(s))
		}
		seq = s.Seq
	}
}

func textOf(s adapter.Screen) string {
	var b strings.Builder
	for _, line := range s.Lines {
		for _, run := range line {
			b.WriteString(run.Text)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// A view and an attach are served at once: keys a view types reach the
// worker, the worker's echo reaches both, and the view sees it on the
// screen while the attach gets the bytes.
func TestAViewTypesIntoTheWorkerBesideAnAttach(t *testing.T) {
	socket := served(t, "exec cat")
	conn, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	answer := make([]byte, 1)
	if _, err := conn.Write([]byte{modeAttach}); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(conn, answer); err != nil || answer[0] != accepted {
		t.Fatalf("the attach: %q %v", answer, err)
	}
	view, err := OpenView(socket)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = view.Close() }()
	if err := view.Keys([]byte("typed in the panel\r")); err != nil {
		t.Fatal(err)
	}
	s := screenWith(t, view, "typed in the panel")
	if s.Cols != 60 || s.Rows != 10 || len(s.Lines) != 10 {
		t.Fatalf("the screen's size: %dx%d, %d lines", s.Cols, s.Rows, len(s.Lines))
	}
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	var got []byte
	buf := make([]byte, 256)
	for !strings.Contains(string(got), "typed in the panel") {
		n, err := conn.Read(buf)
		if err != nil {
			t.Fatalf("the attach got %q: %v", got, err)
		}
		got = append(got, buf[:n]...)
	}
	if err := view.Resize(40, 8); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for s.Cols != 40 || s.Rows != 8 {
		if s, err = view.Screen(ctx, s.Seq); err != nil || ctx.Err() != nil {
			t.Fatalf("the view was not resized: %dx%d, %v", s.Cols, s.Rows, err)
		}
	}
}

// With nobody attached the grid answers the worker's query itself: a cursor
// report asked for reaches the worker, which prints what it read.
func TestTheGridAnswersAQueryWhileNobodyIsAttached(t *testing.T) {
	socket := served(t, `stty raw -echo; printf '\033[6n'; head -c 4 | od -An -c; sleep 10`)
	view, err := OpenView(socket)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = view.Close() }()
	screenWith(t, view, "033")
}

// The screen's values cross from the terminal to the adapter by number, so
// the two packages' orders must agree.
func TestTheScreensValuesAgreeWithTheAdapters(t *testing.T) {
	if adapter.ScreenAttrs(terminal.Strikethrough) != adapter.Strikethrough || adapter.ScreenAttrs(terminal.Inverse) != adapter.Inverse ||
		adapter.ScreenAttrs(terminal.Bold) != adapter.Bold || adapter.ScreenColorKind(terminal.RGB) != adapter.RGBColor ||
		adapter.ScreenColorKind(terminal.Indexed) != adapter.IndexedColor {
		t.Fatal("the terminal's attributes or color kinds differ from the adapter's")
	}
}
