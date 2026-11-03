package workflow

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// drawOnMain has main draw as a Claude Code main in its turn does, a frame
// every millisecond on its pty, which the host reads and relays to every
// client of main's stream; the stop ends it. run is main's wrapper's run,
// "pid.start.boot".
func drawOnMain(run string) (func(), error) {
	var pid int
	if _, err := fmt.Sscan(strings.Split(run, ".")[0], &pid); err != nil {
		return nil, fmt.Errorf("no pid in main's run %q", run)
	}
	tty, err := os.Readlink(fmt.Sprintf("/proc/%d/fd/0", pid))
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(tty, os.O_WRONLY, 0)
	if err != nil {
		return nil, err
	}
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		defer func() { _ = f.Close() }()
		frame := strings.Repeat("=", 100) + "\r\n"
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			case <-time.After(time.Millisecond):
			}
			_, _ = fmt.Fprintf(f, "\x1b[H frame %d\r\n%s", i, strings.Repeat(frame, 20))
		}
	}()
	return func() { close(stop); <-done }, nil
}

// leaveUnread is a client of main's raw stream that stops reading while main
// draws and then leaves, main's bytes unread in its socket, as a shell busy
// giving its screen back, then killed or exiting, does: the host's next write
// to it fails with a broken pipe (docs/traps.md).
func leaveUnread(iso *Isolation, within time.Duration) error {
	conn, err := net.Dial("unix", filepath.Join(iso.Room(iso.HomeRoom()), "sock", "host.sock"))
	if err != nil {
		return err
	}
	if _, err := conn.Write([]byte{'b'}); err != nil {
		_ = conn.Close()
		return err
	}
	time.Sleep(within)
	return conn.Close()
}
