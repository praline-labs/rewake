package terminal

import (
	"errors"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"
)

// Plain is a pseudo-terminal whose screen nothing here keeps: a space host
// runs a worker on it, and the host mode main's harness, each feeding what the
// program writes to a Grid (docs/rules/space.md, docs/v2/host-mode.md). The
// master is the caller's to read; nothing here reads it.
type Plain struct {
	Master *os.File
	sideMu sync.Mutex
	side   *os.File
}

// OpenPlain opens a pair for one program, the program's side sized first.
func OpenPlain(size Size) (*Plain, error) {
	master, side, err := openPair(size)
	if err != nil {
		return nil, err
	}
	return &Plain{Master: master, side: side}, nil
}

// Prepare makes cmd run on the terminal: the terminal's side as its standard
// streams and its controlling terminal, in a session of its own, with the
// environment Environ gives. Start it after Prepare, then call ReleaseSide,
// so the master sees the program's end when its last process closes the
// side.
func (p *Plain) Prepare(cmd *exec.Cmd) error {
	p.sideMu.Lock()
	defer p.sideMu.Unlock()
	if p.side == nil {
		return errors.New("the terminal's side was already released")
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = p.side, p.side, p.side
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	// Ctty names a descriptor of the child: its standard input, the side.
	cmd.SysProcAttr.Setsid, cmd.SysProcAttr.Setctty, cmd.SysProcAttr.Ctty = true, true, 0
	env := cmd.Env
	if env == nil {
		env = os.Environ()
	}
	cmd.Env = Environ(env)
	return nil
}

// ReleaseSide closes this process's copy of the side once the program holds
// its own: until then the program's end cannot be seen.
func (p *Plain) ReleaseSide() error {
	p.sideMu.Lock()
	defer p.sideMu.Unlock()
	if p.side == nil {
		return nil
	}
	err := p.side.Close()
	p.side = nil
	return err
}

// Resize sets the program's window size; the kernel signals SIGWINCH to its
// foreground process group when it changes.
func (p *Plain) Resize(size Size) error { return setSize(p.Master, size) }

// Size is the window size the program sees.
func (p *Plain) Size() (Size, error) { return getSize(p.Master) }

// Close closes the master: the program's side hangs up.
func (p *Plain) Close() error {
	return errors.Join(p.ReleaseSide(), p.Master.Close())
}

// SizeOf reads the window size of a terminal this process holds, the
// person's own for the host mode.
func SizeOf(f *os.File) (Size, error) { return getSize(f) }

// MakeRaw puts the terminal f in raw mode, as cfmakeraw(3) does, and returns
// what restores the mode it had: every key reaches the shown program,
// ^C and ^Z among them, and nothing is echoed twice.
func MakeRaw(f *os.File) (restore func() error, err error) {
	var old syscall.Termios
	if err := ioctl(f, syscall.TCGETS, unsafe.Pointer(&old)); err != nil {
		return nil, err
	}
	raw := old
	raw.Iflag &^= syscall.IGNBRK | syscall.BRKINT | syscall.PARMRK | syscall.ISTRIP | syscall.INLCR | syscall.IGNCR | syscall.ICRNL | syscall.IXON
	raw.Oflag &^= syscall.OPOST
	raw.Lflag &^= syscall.ECHO | syscall.ECHONL | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
	raw.Cflag &^= syscall.CSIZE | syscall.PARENB
	raw.Cflag |= syscall.CS8
	raw.Cc[syscall.VMIN], raw.Cc[syscall.VTIME] = 1, 0
	if err := ioctl(f, syscall.TCSETS, unsafe.Pointer(&raw)); err != nil {
		return nil, err
	}
	return func() error { return ioctl(f, syscall.TCSETS, unsafe.Pointer(&old)) }, nil
}

// IsTerminal says whether f is a terminal.
func IsTerminal(f *os.File) bool {
	var mode syscall.Termios
	return ioctl(f, syscall.TCGETS, unsafe.Pointer(&mode)) == nil
}
