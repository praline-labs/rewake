package mainhost

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/praline-labs/rewake/internal/host/terminal"
)

// Main alone (docs/shell.md#restart-crash-and-close): when a restart's new
// shell does not start, main gets the person's terminal as in a plain
// launch, with one responder to its queries — the person's terminal. The
// grid's answers are dropped meanwhile, while the grid takes every byte, so
// the next shell's preamble is current.

// aloneRun is main alone at work.
type aloneRun struct {
	h    *Host
	stop atomic.Bool
	told atomic.Bool
	wg   sync.WaitGroup

	mu     sync.Mutex
	stream *terminal.Stream
}

// enterAlone gives the person's terminal to main.
func (h *Host) enterAlone(why string) {
	if h.main == nil || h.main.exited || h.lost {
		return
	}
	h.dirty = true
	_ = makeRaw()
	h.main.relay.Mute(true)
	a := &aloneRun{h: h}
	h.solo = a
	a.resize()
	a.wg.Add(1)
	go a.draw()
	// The line is drawn over main's bottom row, until main draws there.
	line := "rewake's shell did not start (" + why + "); rewake restart shell tries again"
	if size, err := sizeOfTerminal(); err == nil {
		time.AfterFunc(200*time.Millisecond, func() {
			if !a.stop.Load() {
				_, _ = os.Stdout.WriteString("\x1b7\x1b[" + strconv.Itoa(size.Rows) + ";1H\x1b[2K" + line + "\x1b8")
			}
		})
	}
	a.wg.Add(1)
	go a.keys()
}

// draw draws main from its preamble and copies its live bytes, again from a
// fresh preamble after a stream the grid dropped for falling behind, until
// leave or main's end.
func (a *aloneRun) draw() {
	defer a.wg.Done()
	for {
		a.mu.Lock()
		if a.stop.Load() {
			a.mu.Unlock()
			return
		}
		s := a.h.main.relay.Grid().Attach()
		a.stream = s
		a.mu.Unlock()
		// A clean slate first: the gap's screen, and whatever the last shell set.
		_, _ = os.Stdout.WriteString(resetModes + "\x1b[!p\x1b[2J\x1b[H")
		_, _ = os.Stdout.Write(s.Preamble)
		for p := range s.Live {
			if _, err := os.Stdout.Write(p); err != nil {
				a.gone("the terminal took no output: " + err.Error())
				return
			}
		}
		select {
		case <-a.h.main.done:
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// keys copies what the person types, and their terminal's answers, to main.
// The read waits on a select with a timeout, so leave can stop it without
// making the descriptor non-blocking, which the next shell would inherit.
func (a *aloneRun) keys() {
	defer a.wg.Done()
	buf := make([]byte, 4096)
	for !a.stop.Load() {
		var set syscall.FdSet
		set.Bits[0] = 1 // descriptor 0
		timeout := syscall.Timeval{Usec: 100_000}
		n, err := syscall.Select(1, &set, nil, nil, &timeout)
		if err != nil && err != syscall.EINTR {
			a.gone("the terminal could not be read: " + err.Error())
			return
		}
		if n <= 0 || a.stop.Load() {
			continue
		}
		got, err := syscall.Read(0, buf)
		switch {
		case err == syscall.EINTR || err == syscall.EAGAIN:
			continue
		case err != nil:
			a.gone("the terminal could not be read: " + err.Error())
			return
		case got <= 0:
			a.gone("the terminal's input ended")
			return
		}
		_ = a.h.main.relay.Type(buf[:got])
	}
}

// gone tells the loop, once, that the person's terminal is gone. No signal
// need follow — the terminal may not be the host's controlling one — and no
// shell is there to say lost, so main alone says it. The post is its own
// goroutine: the loop may be in leave, waiting for this one.
func (a *aloneRun) gone(why string) {
	if a.stop.Load() || a.told.Swap(true) {
		return
	}
	go a.h.post(ev{kind: kTerminalGone, why: why})
}

// resize gives main the person's window size.
func (a *aloneRun) resize() {
	if a == nil {
		return
	}
	if size, err := sizeOfTerminal(); err == nil {
		a.h.main.relay.SetSize(size)
	}
}

// leave takes the terminal back from main: the copies stop, the modes main's
// preamble set on the person's terminal are given back, and the grid answers
// main again.
func (a *aloneRun) leave() {
	if a == nil || a.stop.Swap(true) {
		return
	}
	a.mu.Lock()
	if a.stream != nil {
		a.stream.Detach()
	}
	a.mu.Unlock()
	a.wg.Wait()
	if !a.h.lost {
		_, _ = os.Stdout.WriteString(resetModes)
	}
	a.h.main.relay.Mute(false)
}

// The person's terminal while no shell draws on it.
const (
	// enterGap takes the alternate screen, clears it and hides the cursor.
	enterGap = "\x1b[?1049h\x1b[?25l\x1b[2J"
	// resetModes gives back what a program may have set and left: the mouse,
	// paste and focus reports, the kitty keyboard flags, modifyOtherKeys,
	// cursor keys and keypad, the cursor, the alternate screen.
	resetModes = "\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?9l\x1b[?2004l\x1b[?1004l" +
		"\x1b[<99u\x1b[>4;0m\x1b[?1l\x1b>\x1b[?25h"
)

// gap draws one line on a blank alternate screen, the terminal raw so a key
// typed now neither echoes nor waits.
func (h *Host) gap(line string) {
	if h.lost {
		return
	}
	h.dirty = true
	_ = makeRaw()
	row := 1
	if size, err := sizeOfTerminal(); err == nil && size.Rows > 2 {
		row = size.Rows / 2
	}
	_, _ = os.Stdout.WriteString(resetModes + enterGap + fmt.Sprintf("\x1b[%d;3H", row) + strings.TrimSpace(line))
}

// restoreTerminal gives the person's terminal back as the host found it.
func (h *Host) restoreTerminal() {
	if h.lost {
		return
	}
	if h.dirty {
		_, _ = os.Stdout.WriteString(resetModes + "\x1b[?1049l")
	}
	if h.mode != nil {
		_ = h.mode.Restore(os.Stdin)
	}
}

// The person's terminal, as the host reaches it.

func makeRaw() error                         { return terminal.MakeRaw(os.Stdin) }
func sizeOfTerminal() (terminal.Size, error) { return terminal.SizeOf(os.Stdin) }
func (h *Host) flushInput()                  { _ = terminal.FlushInput(os.Stdin) }
