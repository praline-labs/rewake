package stream

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
	"time"

	"github.com/praline-labs/rewake/internal/host/terminal"
	"github.com/praline-labs/rewake/internal/infra/memfile"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// A relay handed to the next image of its process, across an exec of a
// switch of the room's build (docs/rules/state.md#switching-a-running-room):
// the master and the listener go over as descriptors, and the screen as the
// seed a fresh grid stands where this one stood after
// (docs/shell.md#the-space-hosts-emulator). The program on the terminal
// runs on and never sees the exec; what it writes meanwhile waits in the
// kernel's buffer, and a client that connects waits in the listener's
// backlog.

// ScreenVersion is the shape of the screen file, which a contract of the
// handover that carries a relay names.
const ScreenVersion = 1

// Contract is the version of a handover contract of a host that carries a
// relay: its own handover's version and the screen file's, "<handover>.<screen>",
// so that a raise of either is a different contract and a switch admits only a
// build that reads both.
func Contract(handoverVersion int) string {
	return fmt.Sprintf("%d.%d", handoverVersion, ScreenVersion)
}

// ErrFrozen is a client's input or resize refused once a handover froze
// them: the client gives them again to the next image.
var ErrFrozen = errors.New("the program's terminal is being handed over")

// Handover names the descriptors a relay hands over, kept across the exec.
type Handover struct {
	Master   int `json:"master"`
	Listener int `json:"listener"`
	Screen   int `json:"screen"`
}

// Screen is what the screen file holds: the grid's size and seed, the
// glyph cluster it held back for a mark that may follow, and the program's
// last output for the line a closing space reports.
type Screen struct {
	Version int           `json:"version"`
	Size    terminal.Size `json:"size"`
	Seed    []byte        `json:"seed"`
	Held    []byte        `json:"held"`
	Tail    []byte        `json:"tail"`
}

// Handing is a relay stopped for an exec.
type Handing struct {
	Handover Handover
	r        *Relay
	screen   Screen
	files    []*os.File
}

// admit runs a client's write to the master, or a resize, whole, unless the
// input is frozen.
func (r *Relay) admit(do func() error) error {
	r.gate.RLock()
	defer r.gate.RUnlock()
	if r.frozen {
		return ErrFrozen
	}
	return do()
}

// freeze stops admitting once the writes in flight have finished, within
// bound: a paste the program takes in pieces is on the master to its end.
func (r *Relay) freeze(bound time.Duration) bool {
	deadline := time.Now().Add(bound)
	for !r.gate.TryLock() {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(time.Millisecond)
	}
	r.frozen = true
	r.gate.Unlock()
	return true
}

func (r *Relay) unfreeze() {
	r.gate.Lock()
	r.frozen = false
	r.gate.Unlock()
}

// HandOver stops the relay at a point nothing moves: client input frozen
// with its writes settled, the listener no longer accepting, the master no
// longer read, and the grid closed once its answers are on the master. Each
// step is bounded by bound, past which the relay runs on and the error says
// what was in flight. Handing names the descriptors for the exec, and
// resumes the relay when the exec fails.
func (r *Relay) HandOver(bound time.Duration) (*Handing, error) {
	r.mu.Lock()
	listener, served := r.listener, r.served
	r.mu.Unlock()
	accepting, ok := listener.(interface {
		SetDeadline(time.Time) error
		File() (*os.File, error)
	})
	if !ok {
		return nil, errors.New("the program's terminal is not served on a unix socket yet")
	}
	if !r.freeze(bound) {
		return nil, errors.New("a client's input is still being written to the program")
	}
	past := time.Unix(1, 0)
	_ = accepting.SetDeadline(past)
	<-served
	// The pump ends the grid as it stops: the cluster held back for a mark
	// goes over with the seed.
	r.Grid().Keep()
	_ = r.pty.Master.SetReadDeadline(past)
	r.mu.Lock()
	pumped, answered := r.pumped, r.answered
	r.mu.Unlock()
	<-pumped
	grid := r.Grid()
	held, err := grid.Stop()
	h := &Handing{r: r, screen: Screen{Version: ScreenVersion, Size: grid.Size(), Seed: grid.Seed(), Held: held, Tail: r.lastTail()}}
	if err != nil {
		return nil, r.stopped(h, fmt.Errorf("the program's screen could not be closed: %w", err))
	}
	select {
	case <-answered:
	case <-time.After(bound):
		return nil, r.stopped(h, errors.New("an answer to the program's query is still being written to it"))
	}
	if err := h.keepFiles(accepting); err != nil {
		return nil, r.stopped(h, err)
	}
	return h, nil
}

// stopped resumes a relay HandOver stopped and did not hand over.
func (r *Relay) stopped(h *Handing, why error) error {
	if err := h.Resume(); err != nil {
		return errors.Join(why, err)
	}
	return why
}

// keepFiles makes the screen file and keeps it, the listener and the master
// across the exec.
func (h *Handing) keepFiles(listener interface{ File() (*os.File, error) }) error {
	raw, err := json.Marshal(h.screen)
	if err != nil {
		return err
	}
	screen, err := memfile.New("rewake-screen", raw)
	if err != nil {
		return fmt.Errorf("the program's screen could not be kept for the next image: %w", err)
	}
	h.files = append(h.files, screen)
	listening, err := listener.File()
	if err != nil {
		return err
	}
	h.files = append(h.files, listening)
	if h.Handover.Master, err = keep(h.r.pty.Master, true); err != nil {
		return err
	}
	if h.Handover.Listener, err = keep(listening, true); err != nil {
		return err
	}
	h.Handover.Screen, err = keep(screen, true)
	return err
}

// Resume runs the relay again on this image, from the seed it handed over:
// the exec failed, and the next image is this one.
func (h *Handing) Resume() error {
	for _, file := range h.files {
		_ = file.Close()
	}
	h.files = nil
	_, _ = keep(h.r.pty.Master, false)
	r := h.r
	if err := r.seed(h.screen); err != nil {
		return err
	}
	r.mu.Lock()
	listener, more := r.listener, r.more
	r.mu.Unlock()
	if accepting, ok := listener.(interface{ SetDeadline(time.Time) error }); ok {
		_ = accepting.SetDeadline(time.Time{})
		go r.Serve(listener, more)
	}
	r.unfreeze()
	return nil
}

// seed gives the relay a grid seeded from screen and reads the master again.
func (r *Relay) seed(screen Screen) error {
	grid, err := terminal.NewSeededGrid(screen.Size, screen.Seed, screen.Held)
	if err != nil {
		return fmt.Errorf("the program's screen could not be drawn again: %w", err)
	}
	grid.SetDefaults(terminal.ParseDefaults(r.defaults))
	r.grid.Store(grid)
	r.mu.Lock()
	r.tail = append([]byte(nil), screen.Tail...)
	r.pumped, r.answered = make(chan struct{}), make(chan struct{})
	r.mu.Unlock()
	_ = r.pty.Master.SetReadDeadline(time.Time{})
	go r.Pump()
	go r.Answer()
	return nil
}

// Adopt is the relay an earlier image of this process handed over, reading
// the master again, and the listener at path, which it serves once the
// caller calls Serve. Every descriptor named is the caller's no more.
func Adopt(h Handover, path, defaults string, watch func([]byte)) (*Relay, *state.Listener, error) {
	master := os.NewFile(uintptr(h.Master), "master")
	listening := os.NewFile(uintptr(h.Listener), "listener")
	screenFile := os.NewFile(uintptr(h.Screen), "screen")
	raw, err := io.ReadAll(screenFile)
	_ = screenFile.Close()
	var screen Screen
	if err == nil {
		err = json.Unmarshal(raw, &screen)
	}
	if err == nil && screen.Version != ScreenVersion {
		err = fmt.Errorf("it is version %d; this build takes %d", screen.Version, ScreenVersion)
	}
	if err != nil {
		_ = master.Close()
		_ = listening.Close()
		return nil, nil, fmt.Errorf("the program's screen handed over does not read: %w", err)
	}
	_, _ = keep(master, false)
	listener, err := state.Inherit(path, listening)
	if err != nil {
		_ = master.Close()
		return nil, nil, err
	}
	r := &Relay{pty: terminal.AdoptPlain(master), defaults: defaults, watch: watch}
	if err := r.seed(screen); err != nil {
		_ = listener.Close()
		_ = master.Close()
		return nil, nil, err
	}
	return r, listener, nil
}

// lastTail is a copy of the program's last output.
func (r *Relay) lastTail() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]byte(nil), r.tail...)
}

// keep says whether file goes to the next image, clearing or setting its
// close-on-exec, and answers its descriptor. It goes through the raw
// descriptor: Fd would make the master blocking for good, and the next
// image could then not read it with a deadline.
func keep(file *os.File, across bool) (int, error) {
	raw, err := file.SyscallConn()
	if err != nil {
		return 0, err
	}
	flag := uintptr(syscall.FD_CLOEXEC)
	if across {
		flag = 0
	}
	var fd int
	var errno syscall.Errno
	if err := raw.Control(func(d uintptr) {
		fd = int(d)
		_, _, errno = syscall.Syscall(syscall.SYS_FCNTL, d, syscall.F_SETFD, flag)
	}); err != nil {
		return 0, err
	}
	if errno != 0 {
		return 0, fmt.Errorf("a descriptor could not be kept across the exec: %w", errno)
	}
	return fd, nil
}
