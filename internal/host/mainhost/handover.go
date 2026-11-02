package mainhost

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/praline-labs/rewake/internal/host/stream"
	"github.com/praline-labs/rewake/internal/host/switching"
	"github.com/praline-labs/rewake/internal/host/terminal"
)

// HandoverEnv carries main's host's handover to its next image: the CLI
// takes it before any argument (cli.Run), and the image removes it, so no
// child inherits it.
const HandoverEnv = "REWAKE_MAIN_HOST"

// HandoverVersion is the shape of Handover and of the screen it carries
// (stream.ScreenVersion): an image adopts only its own, and a switch admits
// only a build that takes the same (the contract handover.mainhost).
const HandoverVersion = 1

// Handover is what main's host hands to the next image of itself: the
// launch, main's terminal and the shell's channel as descriptors, and the
// loop's state. Its phase is running, which a drop needs.
type Handover struct {
	Version int             `json:"version"`
	Config  Config          `json:"config"`
	Stream  stream.Handover `json:"stream"`
	Main    Child           `json:"main"`
	Shell   HandedShell     `json:"shell"`
	// Parent is the host's parent at its start, whose leaving is a closed
	// window; Mode the person's terminal's modes, given back at the end.
	Parent   int             `json:"parent"`
	Mode     *terminal.Mode  `json:"mode,omitempty"`
	Crashed  bool            `json:"crashed,omitempty"`
	Dirty    bool            `json:"dirty,omitempty"`
	Gen      int             `json:"gen"`
	Colors   string          `json:"colors,omitempty"`
	Size     terminal.Size   `json:"size"`
	Ref      MainRef         `json:"ref"`
	Released json.RawMessage `json:"released,omitempty"`
	// Count is how many switches the host has been through.
	Count int `json:"count"`
	// Park is set on an image a switch parked; Signals are those caught
	// while parked, which the next loop takes first.
	Park    *switching.Park `json:"park,omitempty"`
	Signals []int           `json:"signals,omitempty"`
}

// Child is a child of the host, by pid and start.
type Child struct {
	PID   int    `json:"pid"`
	Start uint64 `json:"start"`
}

// HandedShell is the drawing shell: its channel's descriptor with what was
// read of it and not taken, its standard error's with the tail kept, and
// what its hello and digest said.
type HandedShell struct {
	Child
	Gen      int               `json:"gen"`
	Why      string            `json:"why"`
	Conn     int               `json:"conn"`
	Unread   []byte            `json:"unread,omitempty"`
	Err      int               `json:"err"`
	Tail     string            `json:"tail,omitempty"`
	Digested bool              `json:"digested"`
	Digest   string            `json:"digest,omitempty"`
	File     string            `json:"file,omitempty"`
	Requires map[string]string `json:"requires,omitempty"`
	Face     *FaceContracts    `json:"face,omitempty"`
}

// handOff execs this image parked, with everything the drop stilled; it
// returns only when the exec did not happen, the shell's descriptors kept
// here again.
func (h *Host) handOff(park switching.Park, handing *stream.Handing) error {
	p := h.shell
	conn, ok := p.conn.(syscall.Conn)
	if !ok {
		return errors.New("the shell's channel is no socket")
	}
	connFD, err := inherit(conn, true)
	if err != nil {
		return err
	}
	defer func() { _, _ = inherit(conn, false) }()
	errFD, err := inherit(p.errPipe, true)
	if err != nil {
		return err
	}
	defer func() { _, _ = inherit(p.errPipe, false) }()
	p.mu.Lock()
	shell := HandedShell{
		Child: Child{PID: p.pid, Start: p.start}, Gen: p.gen, Why: p.why, Conn: connFD, Unread: p.lines.unread(),
		Err: errFD, Tail: p.err.String(), Digested: closed(p.digested), Digest: p.digest, File: p.file, Requires: p.requires, Face: p.face,
	}
	p.mu.Unlock()
	next := Handover{
		Version: HandoverVersion, Config: h.cfg, Stream: handing.Handover, Main: Child{PID: h.main.pid, Start: h.main.start}, Shell: shell,
		Parent: h.parent, Mode: h.mode, Crashed: h.crashed, Dirty: h.dirty, Gen: h.gen, Colors: h.colors, Size: h.size, Ref: *h.ref,
		Released: h.released, Count: h.count + 1, Park: &park,
	}
	raw, err := json.Marshal(next)
	if err != nil {
		return err
	}
	env := append(withoutEnv(os.Environ(), HandoverEnv), HandoverEnv+"="+string(raw))
	// The image this process runs, which the install's path may no longer
	// hold.
	return execImage("/proc/self/exe", os.Args, env)
}

// execImage replaces the image; a test sees what it would have been given.
var execImage = syscall.Exec

// Adopt is main's host's next image: parked, it waits for its switch's
// leader, then execs the switch's target on go, or takes the run on here on
// resume. The signals are taken before anything else, so a window closed
// while parked still ends everything once the loop runs. updates is the
// launch's watch for a release, which a function cannot carry.
func Adopt(raw string, updates func(dir string) (stop func())) int {
	signals := make(chan os.Signal, 8)
	signal.Notify(signals, hostSignals...)
	defer signal.Stop(signals)
	_ = os.Unsetenv(HandoverEnv)
	var handed Handover
	if err := json.Unmarshal([]byte(raw), &handed); err != nil || handed.Version != HandoverVersion {
		_, _ = os.Stderr.WriteString("rewake: main's host's handover does not read in this build\n")
		return 1
	}
	var lost string
	if handed.Park != nil {
		park := *handed.Park
		handed.Park = nil
		decided, err := switching.Wait(park, "main's host")
		switch {
		case err != nil:
			lost = "main's host parked for a switch was lost: " + err.Error()
		case decided == switching.Go:
			handed.Signals = append(handed.Signals, caught(signals)...)
			raw, err := json.Marshal(handed)
			if err == nil {
				err = syscall.Exec(park.Staged, os.Args, append(withoutEnv(os.Environ(), HandoverEnv), HandoverEnv+"="+string(raw)))
			}
			lost = "main's host parked for a switch could not exec the new build: " + err.Error()
		}
	}
	handed.Config.Updates = updates
	h := newHost(handed.Config, signals)
	return h.adopt(&handed, lost)
}

// caught empties what signals holds, for an exec to carry.
func caught(signals chan os.Signal) []int {
	var got []int
	for {
		select {
		case sig := <-signals:
			if value, ok := sig.(syscall.Signal); ok {
				got = append(got, int(value))
			}
		default:
			return got
		}
	}
}

// shellOf is the shell handed over, its channel and its standard error
// taken on.
func shellOf(handed HandedShell) (*shellProc, *lines, error) {
	file := os.NewFile(uintptr(handed.Conn), "shell")
	conn, err := net.FileConn(file)
	_ = file.Close()
	if err != nil {
		return nil, nil, err
	}
	p := &shellProc{
		gen: handed.Gen, why: handed.Why, pid: handed.PID, start: handed.Start, conn: conn,
		errPipe: os.NewFile(uintptr(handed.Err), "shell-stderr"), digested: make(chan struct{}), errDone: make(chan struct{}),
		hello: true, given: true, ready: true, digest: handed.Digest, file: handed.File, requires: handed.Requires, face: handed.Face,
	}
	p.err.WriteString(handed.Tail)
	_, _ = inherit(p.errPipe, false)
	return p, newLines(io.MultiReader(bytes.NewReader(handed.Unread), conn)), nil
}
