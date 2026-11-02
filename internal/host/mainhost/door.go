package mainhost

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/praline-labs/rewake/internal/infra/parts"
)

// door serves one client of the host socket's control mode: a hello, one
// request, its answer and the end. Version 1 takes a restart of a part, from
// any session of the room — the socket's folder is the user's alone — and
// answers once the part is back or has failed (docs/shell.md#the-hosts-channel);
// and parts, answered with the parts the host runs. A switch's drop stops it
// before its post, or lets it write the answer the loop gave (doors).
func (h *Host) door(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if !h.doors.enter(conn) {
		return
	}
	posted := false
	defer func() {
		if !posted {
			h.doors.leave(conn)
		}
	}()
	l := newLines(conn)
	hello, err := takeHello(conn, l)
	if err != nil {
		return
	}
	if hello.Hello != clientCLI {
		_ = writeLine(conn, Message{T: kindFailed, Why: fmt.Sprintf("the door takes a hello from %q, not %q", clientCLI, hello.Hello)})
		return
	}
	line, err := l.next()
	if err != nil {
		return
	}
	_ = conn.SetReadDeadline(time.Time{})
	var msg Message
	if json.Unmarshal(line, &msg) != nil || msg.T != kindRestart && msg.T != kindParts {
		_ = writeLine(conn, Message{T: kindFailed, Why: fmt.Sprintf("the door takes %q or %q in version %d, not %.200q", kindRestart, kindParts, Version, line)})
		return
	}
	reply := make(chan Message, 1)
	h.doors.answering.Add(1)
	defer h.doors.answering.Done()
	h.post(ev{kind: kDoor, msg: msg, reply: reply})
	h.doors.leave(conn)
	posted = true
	select {
	case answer := <-reply:
		_ = writeWithin(conn, answer, doorWrite)
	case <-time.After(doorWait):
		_ = writeLine(conn, Message{T: kindFailed, Why: "the host did not finish the restart within " + doorWait.String()})
	}
}

// doorWrite bounds the write of the door's answer: the client waits for it
// in a read, and a switch's drop waits for it to be written.
const doorWrite = time.Second

// doors are the door's handlers as a switch's drop sees them: those before
// their post, which a read deadline stops, and those past it, answering,
// which only write what the loop gave them.
type doors struct {
	mu        sync.Mutex
	stopped   bool
	posting   map[net.Conn]struct{}
	answering sync.WaitGroup
}

// enter counts a handler before its post; false once a drop stopped the
// door, the client then reading an end.
func (d *doors) enter(conn net.Conn) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopped {
		return false
	}
	if d.posting == nil {
		d.posting = map[net.Conn]struct{}{}
	}
	d.posting[conn] = struct{}{}
	return true
}

// leave counts a handler out once it posted or returned without posting.
func (d *doors) leave(conn net.Conn) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.posting, conn)
}

// stop takes no new handler and cuts the reads of those before their post.
func (d *doors) stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stopped = true
	for conn := range d.posting {
		_ = conn.SetReadDeadline(time.Unix(1, 0))
	}
}

// still says no handler is before its post.
func (d *doors) still() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.posting) == 0
}

// resume takes handlers again after a drop that did not exec.
func (d *doors) resume() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stopped = false
}

// Ask asks the host whose socket is path to restart part, and returns the
// generation of the shell now running. A refusal of the version, or the
// host's failure, is the error.
func Ask(path, part string) (int, error) {
	answer, err := ask(path, Message{T: kindRestart, Part: part})
	if err != nil {
		return 0, err
	}
	return answer.Generation, nil
}

// Running asks the host whose socket is path for the parts it runs: the
// shell drawing now, with its digest and contracts; none while no shell draws.
func Running(path string) ([]parts.Part, error) {
	answer, err := ask(path, Message{T: kindParts})
	if err != nil {
		return nil, err
	}
	return answer.Running, nil
}

// ask sends the door one request and reads its answer.
func ask(path string, request Message) (Message, error) {
	conn, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil {
		return Message{}, ErrNoHost
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(doorWait + 5*time.Second))
	if _, err := conn.Write([]byte{modeControl}); err != nil {
		return Message{}, err
	}
	if err := writeLine(conn, Hello{Hello: clientCLI, V: Version}); err != nil {
		return Message{}, err
	}
	if err := writeLine(conn, request); err != nil {
		return Message{}, err
	}
	line, err := newLines(conn).next()
	if errors.Is(err, io.EOF) {
		return Message{}, errors.New("the host closed the request unanswered: a switch of the room's build may be under way; ask again once it has ended")
	}
	if err != nil {
		return Message{}, fmt.Errorf("the host did not answer: %w", err)
	}
	var answer struct {
		Message
		Refused string `json:"refused"`
	}
	if err := json.Unmarshal(line, &answer); err != nil {
		return Message{}, fmt.Errorf("the host's answer does not read: %.200q", line)
	}
	switch {
	case answer.Refused != "":
		return Message{}, fmt.Errorf("the host refused: %s", answer.Refused)
	case answer.T == kindDone || answer.T == kindParts && request.T == kindParts:
		return answer.Message, nil
	case answer.Why != "":
		return Message{}, fmt.Errorf("%s", answer.Why)
	}
	return Message{}, fmt.Errorf("the host answered %.200q", line)
}

// ErrNoHost is no host answering at the room's socket: main runs without
// rewake's shell, or no main runs.
var ErrNoHost = fmt.Errorf("no host answers in this room")
