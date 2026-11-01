package mainhost

import (
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/praline-labs/rewake/internal/infra/parts"
)

// door serves one client of the host socket's control mode: a hello, one
// request, its answer and the end. Version 1 takes a restart of a part, from
// any session of the room — the socket's folder is the user's alone — and
// answers once the part is back or has failed (docs/shell.md#the-hosts-channel);
// and parts, answered with the parts the host runs.
func (h *Host) door(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
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
	h.post(ev{kind: kDoor, msg: msg, reply: reply})
	select {
	case answer := <-reply:
		_ = writeLine(conn, answer)
	case <-time.After(doorWait):
		_ = writeLine(conn, Message{T: kindFailed, Why: "the host did not finish the restart within " + doorWait.String()})
	}
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
