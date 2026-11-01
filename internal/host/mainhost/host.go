// Package mainhost is main's host in the person's pane (docs/shell.md#who-owns-which-pty):
// the launcher of a main on a terminal stays the pane's process, holds main's
// pseudo-terminal, its emulator and its stream, and runs rewake's shell as a
// child it can replace — on request, or once after a crash — while main and
// its workers run on. Closing the window ends everything, as before. It does
// nothing with mail or with the harness.
package mainhost

import (
	"encoding/json"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/praline-labs/rewake/internal/host/terminal"
)

// The host's bounds.
const (
	// helloWithin: a started shell says hello within it, or it is killed.
	helloWithin = 10 * time.Second
	// readyWithin: a shell given the terminal draws its first frame within it.
	readyWithin = 10 * time.Second
	// releaseWithin: a shell asked to release the terminal exits within it,
	// or it is killed.
	releaseWithin = 3 * time.Second
	// firstReadyWait: main starts after the first shell's colors, or
	// without them after this.
	firstReadyWait = 5 * time.Second
	// crashSettle: a shell's exit with no word waits this long for a signal
	// of the terminal's end that may be on its way.
	crashSettle = 300 * time.Millisecond
	// endWait: once main has ended, the shell finishes its closing within
	// it: the workers' close (10 s at most) and a little more.
	endWait = 15 * time.Second
	// doorWait: a restart asked at the door is answered within it.
	doorWait = 40 * time.Second
)

// fallbackSize is main's size when the first shell gave none.
var fallbackSize = terminal.Size{Cols: 120, Rows: 40}

// Config is one host's launch.
type Config struct {
	// Shell is the shell's program and ShellArgs its arguments.
	Shell     string
	ShellArgs []string
	// ShellEnv is added to the shell's environment.
	ShellEnv []string
	// Main is main's command line: this rewake again, with the launch's
	// arguments. MainEnv is added to its environment.
	Main    []string
	MainEnv []string
	// Base and Room name the room; Dir is its directory.
	Base, Room, Dir string
}

// Host is one host at work. Every field but the channels is the loop's.
type Host struct {
	cfg    Config
	events chan ev

	phase   phase
	crashed bool
	ending  bool
	lost    bool
	code    int
	// dirty: a shell may have left the terminal in its modes, never given
	// back — a crash, or main alone.
	dirty bool

	gen       int
	shell     *shellProc
	candidate *shellProc
	// released is what the last shell asked to release handed on.
	released json.RawMessage
	// waiting are the answers owed for the restart under way.
	waiting []chan Message

	colors string
	size   terminal.Size
	main   *mainRun
	ref    *MainRef
	solo   *aloneRun

	mode   *terminal.Mode
	parent int
}

// Run runs the host until main has ended and the shell has finished, and
// returns main's exit code.
func Run(cfg Config) int {
	h := &Host{cfg: cfg, events: make(chan ev, 64), parent: os.Getppid(), code: 1}
	h.mode, _ = terminal.TakeMode(os.Stdin)
	signals := make(chan os.Signal, 8)
	signal.Notify(signals, syscall.SIGHUP, syscall.SIGPIPE, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGWINCH)
	defer signal.Stop(signals)
	go h.watchParent()

	first, err := h.startShell("first")
	if err != nil {
		_, _ = os.Stderr.WriteString("rewake: " + err.Error() + "\n")
		return 1
	}
	h.shell = first
	h.after(firstReadyWait, ev{kind: kFirstWait})
	for {
		select {
		case e := <-h.events:
			if done := h.handle(e); done {
				return h.finish()
			}
		case sig := <-signals:
			if h.signal(sig) {
				return h.finish()
			}
		}
	}
}

// post hands an event to the loop.
func (h *Host) post(e ev) { h.events <- e }

// after posts e once d has passed.
func (h *Host) after(d time.Duration, e ev) {
	time.AfterFunc(d, func() { h.post(e) })
}

// watchParent says when the host's parent leaves: a closed window takes it.
func (h *Host) watchParent() {
	for range time.Tick(time.Second) {
		if os.Getppid() != h.parent {
			h.post(ev{kind: kParentGone})
			return
		}
	}
}

// signal takes one signal; true ends the host.
func (h *Host) signal(sig os.Signal) bool {
	switch sig {
	case syscall.SIGWINCH:
		h.solo.resize()
		return false
	case syscall.SIGHUP, syscall.SIGPIPE:
		return h.act(decide(evLost, h.situation()), "the terminal hung up")
	}
	return h.act(decide(evClose, h.situation()), "the host was asked to end")
}

// situation is what decide needs to know now.
func (h *Host) situation() situation {
	return situation{phase: h.phase, crashed: h.crashed, window: h.windowThere(), ending: h.ending}
}

// windowThere says the person's terminal is there, positively: nothing said
// it went, the host's parent is the one it had, and the terminal answers its
// size, which one that hung up refuses.
func (h *Host) windowThere() bool {
	if h.lost || os.Getppid() != h.parent {
		return false
	}
	_, err := terminal.SizeOf(os.Stdin)
	return err == nil
}

// finish gives the terminal back and says main's code.
func (h *Host) finish() int {
	h.solo.leave()
	h.restoreTerminal()
	// What a launch that failed said, read from the grid before it closes,
	// or the person would see the window vanish without a word.
	if h.code != 0 && !h.lost && h.main != nil {
		for _, line := range h.main.relay.Lines(20) {
			_, _ = os.Stderr.WriteString(line + "\n")
		}
	}
	h.main.close()
	return h.code
}
