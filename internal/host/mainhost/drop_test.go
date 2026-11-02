package mainhost

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/host/stream"
	"github.com/praline-labs/rewake/internal/host/switching"
	"github.com/praline-labs/rewake/internal/host/terminal"
)

// stillHost is a host as a drop finds it: main's terminal relayed and its
// door served, a shell ready on a socket the test holds the other end of,
// and two children that run until the test ends. Its exec is the test's:
// execs gets the handover it would have carried.
type stillHost struct {
	*Host
	path  string
	shell net.Conn
	execs chan Handover
}

func newStillHost(t *testing.T) *stillHost {
	t.Helper()
	h := newHost(Config{Dir: t.TempDir()}, make(chan os.Signal, 8))
	h.phase, h.ref = running, &MainRef{PID: 1}
	pty, err := terminal.OpenPlain(terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	grid, err := terminal.NewGrid(terminal.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	h.main = &mainRun{pid: child(t), pty: pty, relay: stream.New(pty, grid, "", nil), done: make(chan struct{})}
	go h.main.relay.Pump()
	go h.main.relay.Answer()
	short, err := os.MkdirTemp("", "mainhost-")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(short, "host.sock")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	go h.main.relay.Serve(listener, map[byte]func(net.Conn){modeControl: h.door})
	served(t, path)

	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM|syscall.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	ours, theirs := os.NewFile(uintptr(fds[0]), "host"), os.NewFile(uintptr(fds[1]), "shell")
	conn, _ := net.FileConn(ours)
	other, _ := net.FileConn(theirs)
	_, _ = ours.Close(), theirs.Close()
	stderr, errEnd, _ := os.Pipe()
	_ = errEnd.Close()
	done := make(chan struct{})
	close(done)
	p := &shellProc{gen: 1, pid: child(t), conn: conn, errPipe: stderr, digested: done, errDone: make(chan struct{}), hello: true, given: true, ready: true}
	go p.keepErr()
	h.gen, h.shell = 1, p
	h.follow(p, newLines(conn), false)
	h.watchParent()
	h.watchUpdates()

	s := &stillHost{Host: h, path: path, shell: other, execs: make(chan Handover, 1)}
	was := execImage
	execImage = func(_ string, _ []string, env []string) error {
		for _, entry := range env {
			if raw, ok := strings.CutPrefix(entry, HandoverEnv+"="); ok {
				var handed Handover
				_ = json.Unmarshal([]byte(raw), &handed)
				s.execs <- handed
			}
		}
		return errors.New("no exec in a test")
	}
	t.Cleanup(func() {
		execImage = was
		h.stopParent()
		_ = listener.Close()
		_ = conn.Close()
		_ = other.Close()
		_ = pty.Close()
		_ = h.main.relay.Grid().Close()
		_ = os.RemoveAll(short)
	})
	return s
}

// child is a process of the test's that runs until the test ends.
func child(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	return cmd.Process.Pid
}

// served waits until the door's relay accepts: a mode no one serves is
// closed at once.
func served(t *testing.T, path string) {
	t.Helper()
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	_, _ = conn.Write([]byte{'z'})
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("the relay does not serve: %v", err)
	}
}

// drop is a leader's drop as the loop takes it.
func (s *stillHost) drop(t *testing.T) (string, time.Duration) {
	t.Helper()
	reply := make(chan string, 1)
	started := time.Now()
	if s.take(ev{kind: kDrop, park: &switching.Park{Room: s.cfg.Dir}, drop: reply}) {
		t.Fatal("the drop ended the host")
	}
	return <-reply, time.Since(started)
}

// dial opens the door in the control mode.
func (s *stillHost) dial(t *testing.T) net.Conn {
	t.Helper()
	conn, err := net.Dial("unix", s.path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	_, _ = conn.Write([]byte{modeControl})
	return conn
}

// A door's handler that has posted waits on the loop's answer: the loop
// that joined it before taking its event would wait out doorWait.
func TestADoorsHandlerIsJoinedOnlyOnceItsEventIsTaken(t *testing.T) {
	digested := make(chan struct{})
	close(digested)
	h := &Host{phase: running, events: make(chan ev, 64), shell: &shellProc{hello: true, given: true, ready: true, digested: digested}}
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()
	defer func() { _ = client.Close() }()
	done := make(chan struct{})
	go func() {
		h.door(server)
		close(done)
	}()
	reply := make(chan error, 1)
	go func() {
		if err := writeLine(client, Hello{Hello: clientCLI, V: Version}); err != nil {
			reply <- err
			return
		}
		if err := writeLine(client, Message{T: kindParts}); err != nil {
			reply <- err
			return
		}
		_, err := newLines(client).next()
		reply <- err
	}()
	var queued ev
	select {
	case queued = <-h.events:
	case <-time.After(time.Second):
		t.Fatal("the door posted nothing")
	}
	if queued.kind != kDoor {
		t.Fatalf("the door posted %d", queued.kind)
	}
	select {
	case <-done:
		t.Fatal("the door's handler ended before the loop answered it")
	case <-time.After(100 * time.Millisecond):
	}
	h.handle(queued)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("the door's handler did not end once its event was taken")
	}
	if err := <-reply; err != nil {
		t.Fatal(err)
	}
}

// A parts request posted at the door as the drop begins is answered by the
// drain, and the drop reaches its exec well within its bound, carrying what
// the shell had begun to say.
func TestAPartsRequestDuringTheDropIsAnsweredBeforeTheExec(t *testing.T) {
	s := newStillHost(t)
	conn := s.dial(t)
	_ = writeLine(conn, Hello{Hello: clientCLI, V: Version})
	_ = writeLine(conn, Message{T: kindParts})
	for deadline := time.Now().Add(5 * time.Second); len(s.events) == 0; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the door posted nothing")
		}
	}
	_, _ = s.shell.Write([]byte(`{"t":"rea`))
	time.Sleep(50 * time.Millisecond)
	why, took := s.drop(t)
	if !strings.Contains(why, "no exec in a test") || took > 2*time.Second {
		t.Fatalf("the drop answered %q after %s", why, took)
	}
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	line, err := newLines(conn).next()
	var answer Message
	if err != nil || json.Unmarshal(line, &answer) != nil || answer.T != kindParts {
		t.Fatalf("the door answered %q, %v", line, err)
	}
	handed := <-s.execs
	if handed.Version != HandoverVersion || handed.Main.PID != s.main.pid || handed.Shell.PID != s.Host.shell.pid ||
		string(handed.Shell.Unread) != `{"t":"rea` || handed.Park == nil {
		t.Fatalf("the handover carried %+v", handed)
	}
	// The exec failed: the host runs on, its shell read again.
	_, _ = s.shell.Write([]byte("dy\"}\n"))
	select {
	case e := <-s.events:
		if e.kind != kMsg || e.msg.T != kindReady {
			t.Fatalf("after the drop the shell's line came as %+v", e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the shell is not read after a drop that did not exec")
	}
}

// A close the shell said as the drop began is taken by the drain, as
// always, and the drop is refused: the exec never cuts a close off.
func TestACloseQueuedDuringTheDropRefusesIt(t *testing.T) {
	s := newStillHost(t)
	// Main's end, which the close starts, finds main gone at once.
	close(s.main.done)
	_ = writeLine(s.shell, Message{T: kindClose})
	time.Sleep(50 * time.Millisecond)
	why, _ := s.drop(t)
	if why != "rewake is closing" || s.phase != closing {
		t.Fatalf("the drop answered %q in phase %s", why, s.phase)
	}
	select {
	case handed := <-s.execs:
		t.Fatalf("the host was handed over while closing: %+v", handed)
	default:
	}
}

// A door client stopped in its hello holds no drop: its handler's read is
// cut, and the client reads an end it names as a switch.
func TestADoorClientStoppedInItsHelloDoesNotHoldTheDrop(t *testing.T) {
	s := newStillHost(t)
	conn := s.dial(t)
	_, _ = conn.Write([]byte(`{"hello":`))
	time.Sleep(50 * time.Millisecond)
	why, took := s.drop(t)
	if !strings.Contains(why, "no exec in a test") || took > time.Second {
		t.Fatalf("the drop answered %q after %s", why, took)
	}
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := conn.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("the cut client read %v", err)
	}
}

// A child's exit that a drop's exec cut off before the loop took it stays
// unreaped, for the next image's waiter to find, with its code.
func TestAnExitTheExecCutOffIsFoundByTheNextWaiter(t *testing.T) {
	cmd := exec.Command("sh", "-c", "exit 7")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	_ = cmd.Process.Release()
	waitExit(pid)
	// The first image's waiter saw it; the second's sees it again.
	waitExit(pid)
	if !hasExited(pid) {
		t.Fatal("an exit the loop did not take is not seen again")
	}
	if code := reap(pid); code != 7 {
		t.Fatalf("the exit reaped as %d, want 7", code)
	}
}

// A signal the parked image caught is the adopted loop's first event: a
// window closed while the host was parked ends everything through decide.
func TestASignalCaughtWhileParkedEndsTheAdoptedHost(t *testing.T) {
	h := newHost(Config{}, make(chan os.Signal, 1))
	h.phase, h.stopUpdates = running, func() {}
	h.replay = []os.Signal{syscall.SIGHUP}
	done := make(chan int, 1)
	go func() { done <- h.loop() }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the host did not end on the hangup it caught while parked")
	}
	if !h.lost || h.phase != closing {
		t.Fatalf("the hangup was taken as lost %v, phase %s", h.lost, h.phase)
	}
}

// The loop publishes why it cannot be handed over after each event, and the
// shell's digest taken must be one: with no other event since, an idle host
// would answer a switch busy reading its shell's parts for good.
func TestTheDigestTakenWakesTheLoopForASwitch(t *testing.T) {
	s := newStillHost(t)
	p := s.Host.shell
	p.digested = make(chan struct{})
	s.cfg.Shell = filepath.Join(t.TempDir(), "no-shell")
	s.publish()
	if kind, why := (participant{s.Host}).Answer(); kind != switching.Busy || !strings.Contains(why, "parts") {
		t.Fatalf("before the digest: %s %s", kind, why)
	}
	go s.digest(p)
	timeout := time.After(5 * time.Second)
	for woken := false; !woken; {
		select {
		case e := <-s.events:
			s.take(e)
			woken = e.kind == kDigested
		case <-timeout:
			t.Fatal("the digest taken did not wake the loop")
		}
	}
	s.publish()
	if kind, why := (participant{s.Host}).Answer(); kind != switching.Ready {
		t.Fatalf("after the digest: %s %s", kind, why)
	}
}
