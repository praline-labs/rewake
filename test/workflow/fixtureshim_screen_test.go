package workflow

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

// fixtureScreen draws on the fixture's terminal when it has one, as a space
// host's or a host mode's pseudo-terminal: its session's name, then each
// line typed into it. The terminal's own echo shows the keys as they come;
// the line after says they reached the program.
func fixtureScreen() {
	if !onTerminal(os.Stdin) {
		return
	}
	fmt.Printf("fixture %s\n", os.Getenv("REWAKE_SESSION"))
	typed := bufio.NewReader(os.Stdin)
	for {
		line, err := typed.ReadString('\n')
		if err != nil {
			return
		}
		fmt.Printf("got %s\n", strings.TrimRight(line, "\r\n"))
	}
}

func onTerminal(f *os.File) bool {
	var mode syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), syscall.TCGETS, uintptr(unsafe.Pointer(&mode)))
	return errno == 0
}
