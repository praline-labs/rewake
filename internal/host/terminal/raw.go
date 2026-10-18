package terminal

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"unsafe"
)

// Plain is a pseudo-terminal with no emulator behind it: the space host runs
// a worker on it and relays its master to whoever attaches, whose own
// terminal draws what the worker writes (docs/rules/space.md). The master is
// the caller's to read; nothing here reads it.
type Plain struct {
	Master *os.File
	side   *Terminal
}

// OpenPlain opens a pair for one program, the program's side sized first.
func OpenPlain(size Size) (*Plain, error) {
	master, side, err := openPair(size)
	if err != nil {
		return nil, err
	}
	return &Plain{Master: master, side: &Terminal{master: master, side: side}}, nil
}

// Prepare makes cmd run on the terminal, as Terminal.Prepare does: its side
// as the standard streams and controlling terminal, in a session of its own.
func (p *Plain) Prepare(cmd *exec.Cmd) error { return p.side.Prepare(cmd) }

// ReleaseSide closes this process's copy of the side once the program holds
// its own.
func (p *Plain) ReleaseSide() error { return p.side.ReleaseSide() }

// Resize sets the program's window size; the kernel signals SIGWINCH to its
// foreground process group when it changes.
func (p *Plain) Resize(size Size) error { return setSize(p.Master, size) }

// Size is the window size the program sees.
func (p *Plain) Size() (Size, error) { return getSize(p.Master) }

// Close closes the master: the program's side hangs up.
func (p *Plain) Close() error {
	return errors.Join(p.side.ReleaseSide(), p.Master.Close())
}

// SizeOf reads the window size of a terminal this process holds, the
// person's own for an attach.
func SizeOf(f *os.File) (Size, error) { return getSize(f) }

// MakeRaw puts the terminal f in raw mode, as cfmakeraw(3) does, and returns
// what restores the mode it had: every key reaches the attached program,
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
