package terminal

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

// The pseudo-terminal is opened with the standard library alone: rewake runs
// on Linux only, where a pair is /dev/ptmx, two ioctls to unlock and name the
// other side, and an open of /dev/pts/<n>.

// Size is a terminal's size in columns and rows.
type Size struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}

// maxSide is the most a side may measure: the kernel's window size holds
// each side in sixteen bits.
const maxSide = 1<<16 - 1

func (s Size) check() error {
	if s.Cols < 1 || s.Rows < 1 || s.Cols > maxSide || s.Rows > maxSide {
		return fmt.Errorf("a terminal of %dx%d: each side is 1 to %d", s.Cols, s.Rows, maxSide)
	}
	return nil
}

// openPair opens a pseudo-terminal pair, the program's side sized before any
// program can read its size. The master is opened through os.OpenFile so the
// runtime polls it: a read blocked on it ends when it is closed, and writes
// honor a deadline.
func openPair(size Size) (*os.File, *os.File, error) {
	if err := size.check(); err != nil {
		return nil, nil, err
	}
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("could not open a pseudo-terminal: %w", err)
	}
	// Every failure below closes what was opened before it returns: a
	// descriptor left to a finalizer is one a retry under a tight limit
	// cannot open.
	var number uint32
	unlock := int32(0)
	if err := ioctl(master, syscall.TIOCSPTLCK, unsafe.Pointer(&unlock)); err != nil {
		_ = master.Close()
		return nil, nil, fmt.Errorf("could not unlock the pseudo-terminal: %w", err)
	}
	if err := ioctl(master, syscall.TIOCGPTN, unsafe.Pointer(&number)); err != nil {
		_ = master.Close()
		return nil, nil, fmt.Errorf("could not name the pseudo-terminal: %w", err)
	}
	// O_NOCTTY: the side is the program's controlling terminal, never this
	// process's, which may have none and must not take one.
	side, err := os.OpenFile("/dev/pts/"+strconv.FormatUint(uint64(number), 10), os.O_RDWR|syscall.O_NOCTTY|syscall.O_CLOEXEC, 0)
	if err != nil {
		_ = master.Close()
		return nil, nil, fmt.Errorf("could not open the pseudo-terminal's side: %w", err)
	}
	if err := setSize(master, size); err != nil {
		_ = side.Close()
		_ = master.Close()
		return nil, nil, err
	}
	return master, side, nil
}

// winsize is struct winsize of <asm-generic/termios.h>.
type winsize struct {
	rows, cols, xpixel, ypixel uint16
}

// setSize sets the window size; the kernel signals SIGWINCH to the
// terminal's foreground process group when it changes.
func setSize(f *os.File, size Size) error {
	if err := size.check(); err != nil {
		return err
	}
	ws := winsize{rows: uint16(size.Rows), cols: uint16(size.Cols)}
	if err := ioctl(f, syscall.TIOCSWINSZ, unsafe.Pointer(&ws)); err != nil {
		return fmt.Errorf("could not size the pseudo-terminal: %w", err)
	}
	return nil
}

// getSize reads the window size, as the program sees it.
func getSize(f *os.File) (Size, error) {
	var ws winsize
	if err := ioctl(f, syscall.TIOCGWINSZ, unsafe.Pointer(&ws)); err != nil {
		return Size{}, fmt.Errorf("could not read the pseudo-terminal's size: %w", err)
	}
	return Size{Cols: int(ws.cols), Rows: int(ws.rows)}, nil
}

// ioctl runs one ioctl on f through its raw descriptor, so the runtime's
// poller keeps owning it.
func ioctl(f *os.File, request uintptr, arg unsafe.Pointer) error {
	raw, err := f.SyscallConn()
	if err != nil {
		return err
	}
	var errno syscall.Errno
	if err := raw.Control(func(fd uintptr) {
		_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, fd, request, uintptr(arg))
	}); err != nil {
		return err
	}
	if errno != 0 {
		return errno
	}
	return nil
}

// Environ is env as a program on this terminal gets it. The program is drawn
// by the emulator, never by the person's terminal directly, so TERM and
// COLORTERM name the capable terminal the emulator plays, never the one
// inherited from main's command: that one may be a pipe's or a
// graphics-capable one the emulator does not play. LC_CTYPE is set to C.UTF-8 when the
// locale in effect names no UTF-8, since a program in the C locale draws a
// wide glyph as question marks; an LC_ALL the person set is theirs and stays.
func Environ(env []string) []string {
	values := map[string]string{}
	out := make([]string, 0, len(env)+3)
	for _, kv := range env {
		name, value, _ := strings.Cut(kv, "=")
		values[name] = value
		if name == "TERM" || name == "COLORTERM" {
			continue
		}
		out = append(out, kv)
	}
	out = append(out, "TERM=xterm-256color", "COLORTERM=truecolor")
	if values["LC_ALL"] != "" || namesUTF8(values["LC_CTYPE"]) || values["LC_CTYPE"] == "" && namesUTF8(values["LANG"]) {
		return out
	}
	kept := out[:0]
	for _, kv := range out {
		if !strings.HasPrefix(kv, "LC_CTYPE=") {
			kept = append(kept, kv)
		}
	}
	return append(kept, "LC_CTYPE=C.UTF-8")
}

func namesUTF8(locale string) bool {
	lower := strings.ToLower(locale)
	return strings.Contains(lower, "utf-8") || strings.Contains(lower, "utf8")
}
