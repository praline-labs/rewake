// Package hostmode is main's wrapper holding the person's terminal
// (docs/v2/host-mode.md): main's harness runs on a pseudo-terminal of the
// host's, each worker in its space host, and the host draws the shown one
// above a bar of tabs from their emulators, the person's keys going to it.
package hostmode

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/praline-labs/rewake/internal/host/spacehost"
	"github.com/praline-labs/rewake/internal/host/terminal"
)

const (
	// frameGap gathers the changes of a few milliseconds into one draw.
	frameGap = 8 * time.Millisecond
	// pollEvery is how often the spaces are listed for tabs to add or drop.
	pollEvery = time.Second
)

// Config is what a host is given.
type Config struct {
	// In and Out are the person's terminal.
	In, Out *os.File
	// Env gives the terminal's type and color profile.
	Env []string
	// Panel lists main's spaces.
	Panel spacehost.Panel
}

// Host is the host mode of one run.
type Host struct {
	cfg    Config
	pty    *terminal.Plain
	grid   *terminal.Grid
	canvas *terminal.Canvas
	lay    layout
	tabs   []*tab
	cur    *tab
	in     input
	dirty  bool
	// framed says the paste going on reaches its program in brackets,
	// decided at its start.
	framed bool

	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
	updates chan update
	keys    chan []byte
	winch   chan os.Signal
	opened  chan opened
	gone    chan string

	raw       func() error
	leaveOnce sync.Once
	stopOnce  sync.Once
}

// Open readies a host on the person's terminal: main's pseudo-terminal and
// its screen, sized to the region. Nothing is drawn until Start.
func Open(cfg Config) (*Host, error) {
	if !terminal.IsTerminal(cfg.In) || !terminal.IsTerminal(cfg.Out) {
		return nil, errors.New("the host mode needs a terminal on standard input and output")
	}
	size, err := terminal.SizeOf(cfg.Out)
	if err != nil {
		return nil, err
	}
	lay := layoutFor(size)
	pty, err := terminal.OpenPlain(lay.region)
	if err != nil {
		return nil, err
	}
	grid, err := terminal.NewGrid(lay.region)
	if err != nil {
		_ = pty.Close()
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Host{
		cfg: cfg, pty: pty, grid: grid, lay: lay, ctx: ctx, cancel: cancel,
		done: make(chan struct{}), updates: make(chan update), keys: make(chan []byte, 16),
		winch: make(chan os.Signal, 1), opened: make(chan opened), gone: make(chan string),
	}, nil
}

// Prepare makes cmd, main's harness, run on the host's pseudo-terminal in a
// session of its own.
func (h *Host) Prepare(cmd *exec.Cmd) error { return h.pty.Prepare(cmd) }

// Start takes the person's terminal once the harness started: raw mode, the
// alternate screen, and the loop that draws and routes keys.
func (h *Host) Start() error {
	_ = h.pty.ReleaseSide()
	raw, err := terminal.MakeRaw(h.cfg.In)
	if err != nil {
		return err
	}
	h.raw = raw
	h.canvas = terminal.NewCanvas(h.cfg.Out, h.cfg.Env, h.lay.size)
	if err := h.canvas.Enter(); err != nil {
		h.leave()
		return err
	}
	signal.Notify(h.winch, syscall.SIGWINCH)
	main := newTab("main", "", h.mainKeys, h.mainResize, func() {
		_ = h.pty.Close()
		_ = h.grid.Close()
	})
	h.watch(main, &gridSource{grid: h.grid})
	h.cur = main
	go h.pump()
	go h.answer()
	go h.read()
	go h.poll()
	go h.loop()
	return nil
}

// Stop ends the loop and gives the person's terminal back as it was. It may
// be called more than once, and before Start.
func (h *Host) Stop() {
	h.stopOnce.Do(func() {
		h.cancel()
		if h.canvas == nil {
			_ = h.pty.Close()
			_ = h.grid.Close()
			return
		}
		<-h.done
		signal.Stop(h.winch)
		h.leave()
		for _, t := range h.tabs {
			t.close()
		}
	})
}

// leave restores the person's terminal, once, on every way out.
func (h *Host) leave() {
	h.leaveOnce.Do(func() {
		if h.canvas != nil {
			_ = h.canvas.Leave()
		}
		if h.raw != nil {
			_ = h.raw()
		}
	})
}

func (h *Host) watch(t *tab, src source) {
	ctx, cancel := context.WithCancel(h.ctx)
	t.cancel = cancel
	h.tabs = append(h.tabs, t)
	go t.watch(ctx, src, h.updates)
}

func (h *Host) mainKeys(p []byte) error {
	_, err := h.pty.Master.Write(p)
	return err
}

// mainResize sizes the grid before the terminal, so the harness draws for
// the new size into a grid that already has it.
func (h *Host) mainResize(s terminal.Size) error {
	return errors.Join(h.grid.Resize(s), h.pty.Resize(s))
}

// pump feeds what main's harness writes to its grid.
func (h *Host) pump() {
	buf := make([]byte, 32<<10)
	for {
		n, err := h.pty.Master.Read(buf)
		if n > 0 {
			h.grid.Write(buf[:n])
		}
		if err != nil {
			h.grid.End()
			return
		}
	}
}

// answer writes the grid's answers to main's queries back to main: nothing
// of main reaches the person's terminal unasked.
func (h *Host) answer() {
	for a := range h.grid.Answers() {
		_, _ = h.pty.Master.Write(a)
	}
}

// read takes the person's keys. It stays blocked in its read when the host
// stops, until the process exits.
func (h *Host) read() {
	buf := make([]byte, 4096)
	for {
		n, err := h.cfg.In.Read(buf)
		if n > 0 {
			select {
			case h.keys <- append([]byte(nil), buf[:n]...):
			case <-h.ctx.Done():
				return
			}
		}
		if err != nil {
			return
		}
	}
}
