package terminal

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A program on the terminal draws with colors, attributes and wide glyphs on
// the alternate screen, and the screen reads back as styled runs whose
// columns add up to the width.
func TestAProgramsDrawingReadsBackAsStyledRuns(t *testing.T) {
	term, _ := started(t, "draw", Size{Cols: 20, Rows: 3})
	var g grid
	until(t, term, &g, showing("inv"))
	if !g.last.Modes.AltScreen {
		t.Error("the alternate screen is not said")
	}
	if got := g.line(0); got != "red xy 世界" {
		t.Errorf("row 0: %q", got)
	}
	want := []Run{
		{Text: "red", Cols: 3, Fg: Color{Kind: Indexed, Index: 1}, Attrs: Bold},
		{Text: " ", Cols: 1},
		{Text: "x", Cols: 1, Fg: Color{Kind: Indexed, Index: 208}},
		{Text: "y", Cols: 1, Fg: Color{Kind: RGB, R: 1, G: 2, B: 3}},
		{Text: " 世界" + strings.Repeat(" ", 9), Cols: 14},
	}
	if got := g.rows[0]; !equalRuns(got, want) {
		t.Errorf("row 0 runs:\n got %+v\nwant %+v", got, want)
	}
	if got := g.rows[1]; len(got) == 0 || got[0] != (Run{Text: "inv", Cols: 3, Attrs: Inverse}) {
		t.Errorf("row 1 runs: %+v", got)
	}
	for i, row := range g.rows {
		if width := colsOf(row); width != 20 {
			t.Errorf("row %d covers %d columns", i, width)
		}
	}
}

func equalRuns(a, b []Run) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func colsOf(row []Run) int {
	n := 0
	for _, run := range row {
		n += run.Cols
	}
	return n
}

// Keys written to the terminal reach the program; the line discipline echoes
// them and the program answers.
func TestKeysReachTheProgram(t *testing.T) {
	term, cmd := started(t, "echo", Size{Cols: 40, Rows: 5})
	if err := term.Keys([]byte("hello\r")); err != nil {
		t.Fatal(err)
	}
	var g grid
	until(t, term, &g, showing("got: hello"))
	if err := term.Keys([]byte("exit\r")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-term.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the program's end was not seen")
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	until(t, term, &g, func(g *grid) bool { return g.last.Ended })
}

// A resize reaches the program as SIGWINCH with the new size, and every row
// is published again at the new size.
func TestAResizeReachesTheProgram(t *testing.T) {
	term, _ := started(t, "size", Size{Cols: 30, Rows: 4})
	var g grid
	until(t, term, &g, showing("size 30x4"))
	if err := term.Resize(Size{Cols: 50, Rows: 6}); err != nil {
		t.Fatal(err)
	}
	until(t, term, &g, showing("size 50x6"))
	if g.last.Size != (Size{Cols: 50, Rows: 6}) || len(g.rows) != 6 {
		t.Fatalf("after the resize: %+v, %d rows", g.last.Size, len(g.rows))
	}
	for _, bad := range []Size{{0, 4}, {30, 0}, {70000, 4}} {
		if err := term.Resize(bad); err == nil {
			t.Errorf("a size of %+v was taken", bad)
		}
	}
}

// Closing the terminal hangs it up: the program's session gets SIGHUP.
func TestAHangUpEndsTheProgramsSession(t *testing.T) {
	term, cmd := started(t, "sleep", Size{Cols: 20, Rows: 2})
	time.Sleep(100 * time.Millisecond)
	closed := make(chan error, 1)
	go func() { closed <- term.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not return")
	}
	var exit *exec.ExitError
	if err := cmd.Wait(); !errors.As(err, &exit) || exit.Sys().(syscall.WaitStatus).Signal() != syscall.SIGHUP {
		t.Fatalf("the program ended with %v, not SIGHUP", err)
	}
	if err := term.Close(); err != nil {
		t.Fatalf("a second Close: %v", err)
	}
}

// A program that floods the terminal with queries and never reads the
// answers does not stop the emulator: its output keeps reaching the screen,
// the answers beyond the queue are dropped and counted, and Close returns.
func TestAProgramThatNeverReadsItsAnswersDoesNotStopTheScreen(t *testing.T) {
	term, _ := started(t, "flood", Size{Cols: 20, Rows: 3})
	var g grid
	until(t, term, &g, showing("flooded"))
	if term.Stats().RepliesDropped == 0 {
		t.Error("no answer was dropped, so the program read them or the queue is unbounded")
	}
	done := make(chan error, 1)
	go func() { done <- term.Close() }()
	select {
	case <-done:
	case <-time.After(writeBound + 5*time.Second):
		t.Fatal("Close did not return beside a program that reads nothing")
	}
}

// The program gets the emulator's TERM and COLORTERM, a UTF-8 locale when it
// had none, and the terminal as its controlling terminal.
func TestTheProgramSeesTheEmulatorsTerminal(t *testing.T) {
	term, _ := started(t, "env", Size{Cols: 120, Rows: 2}, "TERM=xterm-ghostty", "COLORTERM=no", "LANG=C", "LC_ALL=", "LC_CTYPE=")
	var g grid
	until(t, term, &g, showing("TERM=xterm-256color COLORTERM=truecolor LC_CTYPE=C.UTF-8 ctty=true"))
}

func TestEnvironKeepsWhatThePersonSet(t *testing.T) {
	for _, c := range []struct {
		env  []string
		want string
	}{
		{[]string{"LANG=en_US.UTF-8"}, "LANG=en_US.UTF-8 TERM=xterm-256color COLORTERM=truecolor"},
		{[]string{"LANG=C", "LC_CTYPE=POSIX", "PATH=/bin"}, "LANG=C PATH=/bin TERM=xterm-256color COLORTERM=truecolor LC_CTYPE=C.UTF-8"},
		{[]string{"LC_ALL=C", "TERM=tmux-256color"}, "LC_ALL=C TERM=xterm-256color COLORTERM=truecolor"},
		{[]string{"LANG=C", "LC_CTYPE=de_DE.utf8"}, "LANG=C LC_CTYPE=de_DE.utf8 TERM=xterm-256color COLORTERM=truecolor"},
	} {
		if got := strings.Join(Environ(c.env), " "); got != c.want {
			t.Errorf("%q: got %q, want %q", c.env, got, c.want)
		}
	}
}

// A read of the screen with nothing new waits for the bound and answers the
// same sequence with no rows; a change wakes it.
func TestAQuietScreenCostsNothing(t *testing.T) {
	term, _ := started(t, "echo", Size{Cols: 20, Rows: 3})
	var g grid
	until(t, term, &g, func(g *grid) bool { return g.seq > 0 })
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if s := term.Screen(ctx, g.seq); s.Seq != g.seq || len(s.Changed) != 0 {
		t.Fatalf("a quiet screen answered %d with %d rows", s.Seq, len(s.Changed))
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = term.Keys([]byte("x"))
	}()
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s := term.Screen(ctx, g.seq)
	if s.Seq == g.seq || len(s.Changed) != 1 || s.Changed[0].Index != 0 {
		t.Fatalf("a typed key: seq %d after %d, changed %+v", s.Seq, g.seq, s.Changed)
	}
	if all := term.Screen(context.Background(), s.Seq+100); len(all.Changed) != 3 {
		t.Fatalf("a sequence never given gets %d rows, not all", len(all.Changed))
	}
}
