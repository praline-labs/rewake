package terminal

import (
	"encoding/json"
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// The person's own terminal, as main's host in the pane uses it
// (docs/shell.md#who-owns-which-pty): its modes kept at start and given
// back at the end, raw between two shells so a typed key neither echoes nor
// waits, its typed input dropped before a new shell starts, and its size,
// whose ioctl a terminal that hung up refuses.

// Mode is a terminal's line discipline as it was taken.
type Mode struct{ t syscall.Termios }

// MarshalJSON is the modes as bytes, for main's host's next image on the
// same machine (internal/host/mainhost), which gives them back at its end.
func (m Mode) MarshalJSON() ([]byte, error) {
	return json.Marshal(unsafe.Slice((*byte)(unsafe.Pointer(&m.t)), unsafe.Sizeof(m.t)))
}

// UnmarshalJSON takes the modes MarshalJSON gave.
func (m *Mode) UnmarshalJSON(raw []byte) error {
	var b []byte
	if err := json.Unmarshal(raw, &b); err != nil {
		return err
	}
	if uintptr(len(b)) != unsafe.Sizeof(m.t) {
		return fmt.Errorf("a terminal's modes of %d bytes, not %d", len(b), unsafe.Sizeof(m.t))
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(&m.t)), len(b)), b)
	return nil
}

// TakeMode is f's modes now, to give back with Restore.
func TakeMode(f *os.File) (*Mode, error) {
	var m Mode
	if err := ioctl(f, syscall.TCGETS, unsafe.Pointer(&m.t)); err != nil {
		return nil, err
	}
	return &m, nil
}

// Restore sets f's modes back to m.
func (m *Mode) Restore(f *os.File) error {
	return ioctl(f, syscall.TCSETS, unsafe.Pointer(&m.t))
}

// MakeRaw puts f in raw mode with no echo, as cfmakeraw does.
func MakeRaw(f *os.File) error {
	m, err := TakeMode(f)
	if err != nil {
		return err
	}
	t := m.t
	t.Iflag &^= syscall.IGNBRK | syscall.BRKINT | syscall.PARMRK | syscall.ISTRIP | syscall.INLCR | syscall.IGNCR | syscall.ICRNL | syscall.IXON
	t.Oflag &^= syscall.OPOST
	t.Lflag &^= syscall.ECHO | syscall.ECHONL | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
	t.Cflag &^= syscall.CSIZE | syscall.PARENB
	t.Cflag |= syscall.CS8
	t.Cc[syscall.VMIN], t.Cc[syscall.VTIME] = 1, 0
	return ioctl(f, syscall.TCSETS, unsafe.Pointer(&t))
}

// tcflsh is TCFLSH of <asm-generic/ioctls.h>, which package syscall leaves out.
const tcflsh = 0x540B

// FlushInput drops what was typed on f and not read yet.
func FlushInput(f *os.File) error {
	raw, err := f.SyscallConn()
	if err != nil {
		return err
	}
	var errno syscall.Errno
	if err := raw.Control(func(fd uintptr) {
		// TCFLSH takes its queue by value, not through a pointer.
		_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, fd, tcflsh, syscall.TCIFLUSH)
	}); err != nil {
		return err
	}
	if errno != 0 {
		return errno
	}
	return nil
}

// SizeOf is f's window size. A terminal that hung up answers EIO, so a size
// read is also the proof the window is still there.
func SizeOf(f *os.File) (Size, error) {
	var ws winsize
	if err := ioctl(f, syscall.TIOCGWINSZ, unsafe.Pointer(&ws)); err != nil {
		return Size{}, err
	}
	return Size{Cols: int(ws.cols), Rows: int(ws.rows)}, nil
}
