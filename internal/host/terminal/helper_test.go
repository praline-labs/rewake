package terminal

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

// helperEnv names the small program the test binary runs as when started on
// a terminal: the tests need a real program on a real pseudo-terminal, and
// the test binary is one that is always there.
const helperEnv = "REWAKE_TERMINAL_HELPER"

func TestMain(m *testing.M) {
	if mode := os.Getenv(helperEnv); mode != "" {
		helper(mode)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func helper(mode string) {
	switch mode {
	case "echo":
		// The line discipline echoes what is typed; the program answers each
		// line, and ends on "exit".
		in := bufio.NewScanner(os.Stdin)
		for in.Scan() {
			if in.Text() == "exit" {
				return
			}
			fmt.Printf("got: %s\n", in.Text())
		}
	case "draw":
		fmt.Print("\x1b[?1049h\x1b[H\x1b[1;31mred\x1b[0m \x1b[38;5;208mx\x1b[38;2;1;2;3my\x1b[0m 世界\x1b[2;1H\x1b[7minv\x1b[0m")
		time.Sleep(time.Hour)
	case "size":
		changed := make(chan os.Signal, 1)
		signal.Notify(changed, syscall.SIGWINCH)
		for {
			size, err := getSize(os.Stdin)
			if err != nil {
				fmt.Println(err)
				return
			}
			fmt.Printf("\x1b[2J\x1b[Hsize %dx%d", size.Cols, size.Rows)
			<-changed
		}
	case "sleep":
		time.Sleep(time.Hour)
	case "flood":
		// Queries the terminal and never reads the answers, then says so,
		// in raw mode as a full-screen program runs: with echo on, the
		// line discipline would read the answers itself.
		raw(os.Stdin)
		for i := range 3000 {
			fmt.Printf("\x1b[c\x1b[6n\r%d", i)
		}
		fmt.Print("\r\nflooded")
		time.Sleep(time.Hour)
	case "env":
		_, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
		fmt.Printf("TERM=%s COLORTERM=%s LC_CTYPE=%s ctty=%t\n", os.Getenv("TERM"), os.Getenv("COLORTERM"), os.Getenv("LC_CTYPE"), err == nil)
		time.Sleep(time.Hour)
	}
}

// raw turns off echo and line editing on f, as a full-screen program does.
func raw(f *os.File) {
	var mode syscall.Termios
	if err := ioctl(f, syscall.TCGETS, unsafe.Pointer(&mode)); err != nil {
		panic(err)
	}
	mode.Lflag &^= syscall.ECHO | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
	if err := ioctl(f, syscall.TCSETS, unsafe.Pointer(&mode)); err != nil {
		panic(err)
	}
}

// started is a helper on a terminal of its own, ended with the test.
func started(t *testing.T, mode string, size Size, env ...string) (*Terminal, *exec.Cmd) {
	t.Helper()
	term, err := Open(size)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(append(os.Environ(), helperEnv+"="+mode), env...)
	if err := term.Prepare(cmd); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if err := term.ReleaseSide(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = term.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	return term, cmd
}

// grid is the screen as a reader that applies every answer holds it.
type grid struct {
	seq  uint64
	last Screen
	rows map[int][]Run
}

func (g *grid) apply(s Screen) {
	if g.rows == nil || s.Size != g.last.Size {
		g.rows = map[int][]Run{}
	}
	for _, row := range s.Changed {
		g.rows[row.Index] = row.Runs
	}
	g.seq, g.last = s.Seq, s
}

func (g *grid) line(i int) string {
	var b strings.Builder
	for _, run := range g.rows[i] {
		b.WriteString(run.Text)
	}
	return strings.TrimRight(b.String(), " ")
}

func (g *grid) text() string {
	var lines []string
	for i := range g.last.Size.Rows {
		lines = append(lines, g.line(i))
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

// until reads the screen until want holds, failing with the screen as it is
// when five seconds pass.
func until(t *testing.T, term *Terminal, g *grid, want func(*grid) bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		ctx, cancel := context.WithDeadline(context.Background(), deadline)
		g.apply(term.Screen(ctx, g.seq))
		cancel()
		if want(g) {
			return
		}
		if time.Now().After(deadline) || g.last.Ended {
			t.Fatalf("the screen did not come to what was waited for:\n%s", g.text())
		}
	}
}

func showing(text string) func(*grid) bool {
	return func(g *grid) bool { return strings.Contains(g.text(), text) }
}
