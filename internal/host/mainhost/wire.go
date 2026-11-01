package mainhost

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/praline-labs/rewake/internal/infra/parts"
)

// The host's channel (docs/shell.md#the-hosts-channel): lines of JSON, one
// message each, over the shell's inherited socket or a connection to the
// room's host socket in the control mode. A client's first line is a hello
// naming what it is and the version it speaks; a hello of another version is
// answered with the refusal and the end. The refusal's shape and the hello's
// are the one part no version changes, so a client of any version reads why.

// Version is the version of the channel's messages, raised with any change
// of them.
const Version = 1

// maxLine bounds one line: a shell's handed-over state carries the face's
// drafts, which a person may have pasted into.
const maxLine = 4 << 20

// Who a hello names.
const (
	clientShell = "shell"
	clientCLI   = "cli"
)

// Hello is a client's first line. A shell's says the contracts it loaded,
// which the host keeps for the plan of an update: an older shell says none,
// and an older host ignores the field.
type Hello struct {
	Hello     string     `json:"hello"`
	V         int        `json:"v"`
	Contracts *Contracts `json:"contracts,omitempty"`
}

// Contracts are what a shell requires of the parts beside it, and Face what
// the face compiled into it speaks and requires; absent when the shell runs
// a face from files, which follows them.
type Contracts struct {
	Requires map[string]string `json:"requires"`
	Face     *FaceContracts    `json:"face,omitempty"`
}

// FaceContracts are a compiled face's.
type FaceContracts struct {
	Speaks   map[string]string `json:"speaks"`
	Requires map[string]string `json:"requires"`
}

// Refusal is the answer to a hello the host does not take.
type Refusal struct {
	Refused string `json:"refused"`
	Speaks  []int  `json:"speaks"`
}

// refusalOf is the refusal of a client speaking v.
func refusalOf(v int) Refusal {
	return Refusal{Refused: fmt.Sprintf("this host speaks %d, the client %d", Version, v), Speaks: []int{Version}}
}

// MainRef is where a shell finds main: its wrapper's pid, which proves the
// shell main's to rewake room, and the socket of main's stream.
type MainRef struct {
	PID    int    `json:"pid"`
	Socket string `json:"socket"`
}

// Message is every message after the hello; T names its kind and the kind
// says which of the other fields it carries.
type Message struct {
	T string `json:"t"`
	// welcome
	Generation int    `json:"generation,omitempty"`
	Why        string `json:"why,omitempty"`
	// welcome and main
	Main *MainRef `json:"main,omitempty"`
	// go and released: what one shell hands the next, opaque to the host.
	State json.RawMessage `json:"state,omitempty"`
	// ready
	Colors string `json:"colors,omitempty"`
	Cols   int    `json:"cols,omitempty"`
	Rows   int    `json:"rows,omitempty"`
	// main-ended
	Code *int `json:"code,omitempty"`
	// restart, reload and reloaded
	Part string `json:"part,omitempty"`
	// parts, the door's answer: the parts the host runs.
	Running []parts.Part `json:"running,omitempty"`
}

// The kinds of message.
const (
	// host to shell
	kindWelcome   = "welcome"
	kindMain      = "main"
	kindGo        = "go"
	kindRelease   = "release"
	kindClosing   = "closing"
	kindMainEnded = "main-ended"
	// shell to host
	kindReady    = "ready"
	kindClose    = "close"
	kindLost     = "lost"
	kindReleased = "released"
	// shell or the room's door to host
	kindRestart = "restart"
	// the room's door to host, and the host's answer: the parts it runs
	kindParts = "parts"
	// host to shell: reload a part in place; the shell's answer
	kindReload   = "reload"
	kindReloaded = "reloaded"
	// host to the door, or to a shell for a kind it does not take
	kindDone   = "done"
	kindFailed = "failed"
	kindError  = "error"
)

// lines reads one connection's lines.
type lines struct{ r *bufio.Reader }

func newLines(r io.Reader) *lines { return &lines{r: bufio.NewReaderSize(r, 64<<10)} }

// next is the next line, without its end.
func (l *lines) next() ([]byte, error) {
	var line []byte
	for {
		part, err := l.r.ReadSlice('\n')
		line = append(line, part...)
		switch {
		case err == nil:
			return line[:len(line)-1], nil
		case errors.Is(err, bufio.ErrBufferFull):
			if len(line) > maxLine {
				return nil, fmt.Errorf("a line over %d bytes", maxLine)
			}
		default:
			return nil, err
		}
	}
}

// takeHello reads a client's hello: the hello when it speaks this version;
// otherwise the refusal is written and an error returned.
func takeHello(conn net.Conn, l *lines) (Hello, error) {
	line, err := l.next()
	if err != nil {
		return Hello{}, err
	}
	var hello Hello
	if err := json.Unmarshal(line, &hello); err != nil || hello.Hello == "" {
		_ = writeLine(conn, refusalOf(0))
		return Hello{}, fmt.Errorf("the first line is no hello: %q", line)
	}
	if hello.V != Version {
		_ = writeLine(conn, refusalOf(hello.V))
		return Hello{}, fmt.Errorf("a client of version %d", hello.V)
	}
	return hello, nil
}

// writeLine writes v as one line, within a bound: a client that stops
// reading is cut off rather than hold the host.
func writeLine(conn net.Conn, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err = conn.Write(append(data, '\n'))
	return err
}
