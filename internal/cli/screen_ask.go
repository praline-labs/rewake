package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/praline-labs/rewake/internal/core/screen"
)

// How long rewake screen waits for the shell to draw a window or for a space
// host to answer, and the most of a frame it reads: 400 by 200 cells in
// runs, far under it.
const (
	screenDial   = 2 * time.Second
	screenAnswer = 10 * time.Second
	screenMax    = 4 << 20
)

// errNoShell is no shell answering at its socket: none runs, or main has no
// terminal.
var errNoShell = errors.New("no shell is running")

// shellAnswer is a line from the shell: a frame, or why it drew none.
type shellAnswer struct {
	screen.Frame
	Error string `json:"error"`
}

// askShell asks the shell at socket for the window whose pane is named: main,
// or a space's id (docs/screen.md#the-request).
func askShell(socket, pane string) (*screen.Frame, error) {
	conn, err := net.DialTimeout("unix", socket, screenDial)
	if err != nil {
		return nil, errNoShell
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(screenAnswer))
	request, _ := json.Marshal(map[string]string{"pane": pane})
	if _, err := conn.Write(append(request, '\n')); err != nil {
		return nil, fmt.Errorf("the shell did not take the request: %w", err)
	}
	var answer shellAnswer
	if err := readLine(conn, &answer); err != nil {
		return nil, fmt.Errorf("the shell did not answer: %w", err)
	}
	if answer.Error != "" {
		return nil, errors.New(answer.Error)
	}
	frame := answer.Frame
	frame.Source = screen.SourceShell
	return &frame, nil
}

// askHost asks a space host for its worker's screen, the pane alone: the
// snapshot mode of its socket.
func askHost(socket string) (*screen.Frame, error) {
	conn, err := net.DialTimeout("unix", socket, screenDial)
	if err != nil {
		return nil, fmt.Errorf("the worker's space host is not answering (%v)", err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(screenAnswer))
	if _, err := conn.Write([]byte{'s'}); err != nil {
		return nil, err
	}
	var frame screen.Frame
	if err := readLine(conn, &frame); err != nil {
		return nil, fmt.Errorf("the space host did not answer: %w", err)
	}
	frame.Source = screen.SourceHost
	return &frame, nil
}

func readLine(conn net.Conn, into any) error {
	line, err := bufio.NewReader(io.LimitReader(conn, screenMax)).ReadBytes('\n')
	if err != nil {
		return err
	}
	return json.Unmarshal(line, into)
}
