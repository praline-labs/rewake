package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/praline-labs/rewake/internal/host/terminal"
)

// TestFixtureHostMode is the host mode on its main path, on the fixture:
// main started with --host on a terminal the scenario holds shows its screen
// above a bar; a space it opens gets a tab; Ctrl+\ n shows the worker and
// takes its keys, Ctrl+\ m shows main again; main's end gives the terminal
// back. The screen is read the way a person sees it, through an emulator fed
// from the terminal's master.
func TestFixtureHostMode(t *testing.T) {
	binary := enterScenario(t, "fixture-host")
	c := Start(t, Spec{
		Name:         "fixture-host",
		Harness:      fixtureColumn.harness,
		Observations: fixtureHostObservations,
		Deadline:     90 * time.Second,
	})
	iso := Isolate(t, c, binary)
	size := terminal.Size{Cols: 100, Rows: 30}
	person, err := openPersonTerminal(size)
	if err != nil {
		t.Fatal(err)
	}
	defer person.close()

	asks := newRequests(iso, "lead")
	lead, launch := sessionLaunch(t, c, iso, "", fixtureColumn.harness, "lead", "--main", []string{"--host"}, nil, asks.env(), shimInboxJSON+"=1")
	launch.Stdin, launch.Stdout = person.side, person.side
	release := c.captureStderr(launch, iso.Home, "lead")
	process, err := c.start(launch, false)
	release()
	if err != nil {
		t.Fatalf("launching the session: %v", err)
	}
	lead.home, lead.process = iso.Home, process
	_ = person.side.Close()

	shows := func(what string, bound time.Duration, want func(string) bool) bool {
		c.Note("waiting for the screen to show " + what)
		return waitFor(c, bound, func() bool { return want(person.text()) })
	}
	if !shows("main", 20*time.Second, func(s string) bool {
		return strings.Contains(s, "fixture lead-fixture") && strings.Contains(person.row(size.Rows-1), " 0 main ")
	}) {
		c.Contradicted(obsHostMain, "the screen never showed main above the bar: %q", person.text())
		stopSession(t, c, lead)
		return
	}
	person.typed("hello\r")
	if shows("main's answer", 5*time.Second, func(s string) bool { return strings.Contains(s, "got hello") }) {
		c.Observed(obsHostMain, "main's screen above a bar naming main, and a line typed reached main")
	} else {
		c.Contradicted(obsHostMain, "a line typed into main never reached it: %q", person.text())
	}

	worker, env := spaceWorker(iso, "api", fixtureColumn.harness)
	encoded, _ := json.Marshal(shimRequest{Args: []string{"space", "open", "api", "--harness", fixtureColumn.harness, "--json"}, Env: env})
	code, out, answered := asks.send(c, encoded, "space open api")
	var opened struct {
		ID string `json:"id"`
	}
	if !answered || code != 0 || json.Unmarshal([]byte(out), &opened) != nil {
		c.Contradicted(obsHostWorker, "space open answered %d, %v: %s", code, answered, out)
		stopSession(t, c, lead)
		return
	}
	if !shows("the worker's tab", 10*time.Second, func(string) bool { return strings.Contains(person.row(size.Rows-1), " 1 "+worker.name) }) {
		c.Contradicted(obsHostWorker, "no tab for %s on the bar: %q", worker.name, person.row(size.Rows-1))
		stopSession(t, c, lead)
		return
	}
	person.typed("\x1cn")
	switched := shows("the worker", 5*time.Second, func(s string) bool { return strings.Contains(s, "fixture "+worker.name) })
	person.typed("world\r")
	switch {
	case !switched:
		c.Contradicted(obsHostWorker, "Ctrl+\\ n never showed the worker: %q", person.text())
	case !shows("the worker's answer", 5*time.Second, func(s string) bool { return strings.Contains(s, "got world") }):
		c.Contradicted(obsHostWorker, "a line typed into the worker never reached it: %q", person.text())
	default:
		c.Observed(obsHostWorker, "a tab for the space, Ctrl+\\ n showed the worker and a line typed reached it")
	}
	person.typed("\x1cm")
	if shows("main again", 5*time.Second, func(s string) bool {
		return strings.Contains(s, "got hello") && !strings.Contains(s, "fixture "+worker.name)
	}) {
		c.Observed(obsHostBack, "Ctrl+\\ m showed main again")
	} else {
		c.Contradicted(obsHostBack, "Ctrl+\\ m did not show main: %q", person.text())
	}

	stopSession(t, c, lead)
	if waitFor(c, 5*time.Second, func() bool { return strings.Contains(person.raw(), "\x1b[?1049l") }) {
		c.Observed(obsHostRestored, "main's end left the alternate screen")
	} else {
		c.Contradicted(obsHostRestored, "the terminal was never given back")
	}
	// The space ends with main's run; the case waits for it so nothing of it
	// outlives the case.
	waitFor(c, 20*time.Second, func() bool {
		record := spaceRecordOf(iso, opened.ID)
		return (record.State == "closed" || record.State == "kept") && !runAlive(record.Worker.Run) && !runAlive(record.Host)
	})
}

const (
	obsHostMain     = "main started with --host shows its screen above a bar and takes the person's keys"
	obsHostWorker   = "a space main opens gets a tab, and Ctrl+\\ n shows its worker and sends it the keys"
	obsHostBack     = "Ctrl+\\ m shows main again"
	obsHostRestored = "main's end gives the person's terminal back"
)

var fixtureHostObservations = []string{obsHostMain, obsHostWorker, obsHostBack, obsHostRestored}

// personTerminal is a terminal the scenario holds as a person would: rewake
// gets its side, and what rewake draws is kept by an emulator.
type personTerminal struct {
	master, side *os.File
	grid         *terminal.Grid
	mu           sync.Mutex
	all          strings.Builder
}

func openPersonTerminal(size terminal.Size) (*personTerminal, error) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, err
	}
	var unlock int32
	var number uint32
	if err := ptyIoctl(master, syscall.TIOCSPTLCK, unsafe.Pointer(&unlock)); err != nil {
		_ = master.Close()
		return nil, err
	}
	if err := ptyIoctl(master, syscall.TIOCGPTN, unsafe.Pointer(&number)); err != nil {
		_ = master.Close()
		return nil, err
	}
	side, err := os.OpenFile("/dev/pts/"+strconv.Itoa(int(number)), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		_ = master.Close()
		return nil, err
	}
	window := [4]uint16{uint16(size.Rows), uint16(size.Cols)}
	grid, err := terminal.NewGrid(size)
	if err == nil {
		err = ptyIoctl(side, syscall.TIOCSWINSZ, unsafe.Pointer(&window))
	}
	if err != nil {
		_, _ = master.Close(), side.Close()
		return nil, err
	}
	p := &personTerminal{master: master, side: side, grid: grid}
	go p.read()
	// The answers to rewake's queries go nowhere: the host mode asks none it
	// waits on, and a full queue only drops them.
	go func() {
		for answer := range grid.Answers() {
			_ = answer
		}
	}()
	return p, nil
}

func ptyIoctl(f *os.File, request uintptr, arg unsafe.Pointer) error {
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), request, uintptr(arg)); errno != 0 {
		return errno
	}
	return nil
}

func (p *personTerminal) read() {
	buf := make([]byte, 32<<10)
	for {
		n, err := p.master.Read(buf)
		if n > 0 {
			p.grid.Write(buf[:n])
			p.mu.Lock()
			p.all.Write(buf[:n])
			p.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

func (p *personTerminal) screen() terminal.Screen {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return p.grid.Screen(ctx, 0)
}

// row is one row of the screen as text.
func (p *personTerminal) row(index int) string {
	for _, row := range p.screen().Changed {
		if row.Index == index {
			var text strings.Builder
			for _, run := range row.Runs {
				text.WriteString(run.Text)
			}
			return text.String()
		}
	}
	return ""
}

// text is the whole screen, a line per row.
func (p *personTerminal) text() string {
	var lines []string
	for _, row := range p.screen().Changed {
		var text strings.Builder
		for _, run := range row.Runs {
			text.WriteString(run.Text)
		}
		lines = append(lines, text.String())
	}
	return strings.Join(lines, "\n")
}

// raw is every byte rewake wrote.
func (p *personTerminal) raw() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.all.String()
}

func (p *personTerminal) typed(keys string) { _, _ = p.master.Write([]byte(keys)) }

func (p *personTerminal) close() {
	_ = errors.Join(p.master.Close(), p.grid.Close())
}
