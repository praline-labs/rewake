package spacehost

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
		_ = pty.Close()
		_ = grid.Close()
		_ = os.RemoveAll(dir)
	})
	return socket
}

// screenWith waits for a screen whose text holds want.
func screenWith(t *testing.T, view *View, want string) terminal.Picture {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var seq uint64
	for {
		p, err := view.Picture(ctx, seq)
		if strings.Contains(textOf(p), want) {
			return p
		}
		if err != nil || ctx.Err() != nil || p.Ended {
			t.Fatalf("no screen with %q (err %v), the last:\n%s", want, err, textOf(p))
		}
		seq = p.Seq
	}
}

func textOf(p terminal.Picture) string {
	var b strings.Builder
	for _, row := range p.Rows {
		for _, run := range row {
			b.WriteString(run.Text)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// Keys a view types reach the worker, the view sees the echo on the screen,
// and a size it sends is the worker's.
func TestAViewTypesIntoTheWorkerAndSizesIt(t *testing.T) {
	socket := served(t, "exec cat")
	view, err := OpenView(socket)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = view.Close() }()
	if err := view.Keys([]byte("typed in the host\r")); err != nil {
		t.Fatal(err)
	}
	p := screenWith(t, view, "typed in the host")
	if p.Size != (terminal.Size{Cols: 60, Rows: 10}) || len(p.Rows) != 10 {
		t.Fatalf("the screen's size: %+v, %d rows", p.Size, len(p.Rows))
	}
	if err := view.Resize(terminal.Size{Cols: 40, Rows: 8}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for p.Size != (terminal.Size{Cols: 40, Rows: 8}) {
		if p, err = view.Picture(ctx, p.Seq); err != nil || ctx.Err() != nil {
			t.Fatalf("the view was not resized: %+v, %v", p.Size, err)
		}
	}
}

// The grid answers the worker's query itself: a cursor report asked for
// reaches the worker, which prints what it read.
func TestTheGridAnswersAQuery(t *testing.T) {
	socket := served(t, `stty raw -echo; printf '\033[6n'; head -c 4 | od -An -c; sleep 10`)
	view, err := OpenView(socket)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = view.Close() }()
	screenWith(t, view, "033")
}
