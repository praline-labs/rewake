// Package switching runs the switch of a running room to another build
// (docs/rules/state.md#switching-a-running-room) between its leader, the
// rewake restart rewake command, and the room's long-lived processes, its
// participants: each wrapper, space host and main's host. The lease and the
// mark are infra/build's; this package is the conversation on the leader's
// socket — who can hand over, the drop, and go or resume.
package switching

import (
	"bufio"
	"encoding/json"
	"net"
	"sync"
	"time"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// The answers a participant gives.
const (
	Ready  = "ready"
	Busy   = "busy"
	Cannot = "cannot"
	// Parked is the answer of a participant's parked image: it dropped its
	// lease by an exec and waits for go or resume.
	Parked = "parked"
)

// The leader's orders.
const (
	Drop   = "drop"
	Go     = "go"
	Resume = "resume"
)

// answer is one line a participant sends the leader.
type answer struct {
	Version int    `json:"version"`
	Pid     int    `json:"pid"`
	Start   uint64 `json:"start"`
	Who     string `json:"who"`
	Answer  string `json:"answer"`
	Why     string `json:"why,omitempty"`
}

// order is one line the leader sends a participant.
type order struct {
	Do string `json:"do"`
}

// Park is what a participant's parked image carries across its exec: where
// its leader listens, and what it switches to.
type Park struct {
	Room        string `json:"room"`
	Socket      string `json:"socket"`
	Leader      int    `json:"leader"`
	LeaderStart uint64 `json:"leaderStart"`
	Target      string `json:"target"`
	Staged      string `json:"staged"`
	// Deadline is the dropping mark's, in Unix milliseconds: past it a
	// parked image decides by the room's build alone.
	Deadline int64 `json:"deadline"`
}

func parkOf(room string, mark state.SwitchMark) Park {
	return Park{
		Room: room, Socket: mark.Socket, Leader: mark.Leader, LeaderStart: mark.LeaderStart,
		Target: mark.Target, Staged: mark.Staged, Deadline: mark.Deadline,
	}
}

// Participant is a long-lived process of the room as a switch asks it.
type Participant interface {
	// Who names the process to the person: a session, a space host.
	Who() string
	// Answer is whether it can hand over now: Ready, Busy or Cannot, with
	// why for the last two.
	Answer() (string, string)
	// Drop hands the process over and execs its own image parked with park;
	// it returns only when it did not, with why, and the process goes on.
	Drop(park Park) string
}

// cannot is a participant that cannot hand over.
type cannot struct{ who, why string }

// CannotHandOver is a participant that always answers Cannot: a process
// whose state no image hands over yet.
func CannotHandOver(who, why string) Participant { return cannot{who: who, why: why} }

func (c cannot) Who() string              { return c.who }
func (c cannot) Answer() (string, string) { return Cannot, c.why }
func (c cannot) Drop(Park) string         { return c.why }

// line is one connection carrying JSON lines, its writes ordered.
type line struct {
	conn net.Conn
	mu   sync.Mutex
	in   *bufio.Scanner
}

func newLine(conn net.Conn) *line {
	in := bufio.NewScanner(conn)
	in.Buffer(make([]byte, 0, 4096), 64*1024)
	return &line{conn: conn, in: in}
}

func (l *line) send(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_ = l.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_, err = l.conn.Write(append(data, '\n'))
	return err
}

func (l *line) receive(value any) error {
	if !l.in.Scan() {
		if err := l.in.Err(); err != nil {
			return err
		}
		return net.ErrClosed
	}
	return json.Unmarshal(l.in.Bytes(), value)
}
