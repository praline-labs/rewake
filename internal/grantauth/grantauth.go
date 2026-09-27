// Package grantauth makes a grant something the worker receiving it cannot
// forge (docs/grants.md#who-can-grant).
//
// Everything a message carries lives in the state directory, which a
// sandboxed Codex worker can write: a letter with grantDirs dropped in its own
// mailbox, or a send run with main's variables, would otherwise pass for
// main's. So a grant stands only on what main's wrapper — a process no
// session's sandbox reaches into — holds in memory. `rewake send` registers
// it there over the wrapper's socket, and the wrapper takes it only from a
// process running below itself. The receiving wrapper asks the sending one to
// confirm the message before it is handed over, and believes the answer only
// from the process the sender's record names, in its own namespaces: a
// listener put in the socket's place by a sandboxed process lives in the
// sandbox's namespaces and cannot leave them.
//
// The receiving side keeps what it was granted the same way when its harness
// applies a grant through a hook of its own (keeper.go): in the wrapper's
// memory, answered only to a process below the wrapper.
package grantauth

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"slices"
	"strconv"
	"syscall"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/proc"
)

// Grant is what main registered for one message.
type Grant struct {
	ID      string   `json:"id"`
	To      string   `json:"to"`
	ToEpoch string   `json:"toEpoch"`
	Dirs    []string `json:"dirs,omitempty"`
	Broad   []string `json:"broad,omitempty"`
	Git     bool     `json:"git,omitempty"`
}

// Same says whether two grants give the same thing to the same run.
func (g Grant) Same(other Grant) bool {
	return g.ID == other.ID && g.To == other.To && g.ToEpoch == other.ToEpoch && g.Git == other.Git &&
		sameSet(g.Dirs, other.Dirs) && sameSet(g.Broad, other.Broad)
}

func sameSet(a, b []string) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}

// Operations of the protocol: one JSON line each way.
const (
	opRegister = "register"
	opConfirm  = "confirm"
	// opDecide is a hook call to the keeper of a worker's grants.
	opDecide = "decide"
)

type request struct {
	Op    string          `json:"op"`
	Grant Grant           `json:"grant"`
	Call  json.RawMessage `json:"call,omitempty"`
}

type response struct {
	Grant  *Grant          `json:"grant,omitempty"`
	Output json.RawMessage `json:"output,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// Errors a caller tells apart.
var (
	// ErrUnreachable is a wrapper that could not be asked: nothing answered
	// at its socket, or the answer did not come in time. It may pass.
	ErrUnreachable = errors.New("the wrapper could not be reached")
	// ErrNotConfirmed is an answer that settles it: the wrapper that answered
	// is not the one that could grant, or it holds no such grant.
	ErrNotConfirmed = errors.New("not confirmed")
)

// exchangeLimit bounds one conversation with a wrapper.
const exchangeLimit = 2 * time.Second

// Register asks this session's wrapper to hold a grant. It refuses unless the
// caller runs below it and it serves a main session.
func Register(path string, grant Grant) error {
	conn, err := dial(path)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	if err := servesCaller(conn); err != nil {
		return err
	}
	answer, err := exchange(conn, request{Op: opRegister, Grant: grant})
	if err != nil {
		return err
	}
	if answer.Error != "" {
		return fmt.Errorf("%w: %s", ErrNotConfirmed, answer.Error)
	}
	return nil
}

// servesCaller checks that the listener at the other end of conn is a
// process above the caller: its own session's wrapper. A listener that bound
// the address first would otherwise take the registration, and send would
// report a grant that no wrapper can confirm.
func servesCaller(conn *net.UnixConn) error {
	peer, err := peerOf(conn)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	if int(peer.Uid) != os.Getuid() {
		return fmt.Errorf("%w: the socket is served by another user", ErrNotConfirmed)
	}
	if err := proc.Default.Descends(os.Getpid(), int(peer.Pid)); err != nil {
		return fmt.Errorf("%w: the socket is served by process %d, which is not this session's wrapper: %v", ErrNotConfirmed, peer.Pid, err)
	}
	return nil
}

// Expect is the wrapper a confirmation has to come from: the process the
// sender's record names, as it started.
type Expect struct {
	PID   int
	Start uint64
}

// Confirm asks the sending session's wrapper for the grant of one message to
// one run, and returns what it holds.
func Confirm(path string, expect Expect, id, to, toEpoch string) (Grant, error) {
	conn, err := dial(path)
	if err != nil {
		return Grant{}, err
	}
	defer func() { _ = conn.Close() }()
	if err := expect.answeredBy(conn); err != nil {
		return Grant{}, err
	}
	answer, err := exchange(conn, request{Op: opConfirm, Grant: Grant{ID: id, To: to, ToEpoch: toEpoch}})
	if err != nil {
		return Grant{}, err
	}
	// Asked again after the answer: a pid given up and taken again while
	// the question was out is not the wrapper that was checked.
	if !proc.Alive(expect.PID, expect.Start) {
		return Grant{}, fmt.Errorf("%w: the sending session's wrapper ended while it answered", ErrNotConfirmed)
	}
	if answer.Error != "" {
		return Grant{}, fmt.Errorf("%w: %s", ErrNotConfirmed, answer.Error)
	}
	if answer.Grant == nil {
		return Grant{}, fmt.Errorf("%w: the answer named no grant", ErrNotConfirmed)
	}
	return *answer.Grant, nil
}

// namespaces reads a process's namespaces; a test stands a sandboxed
// wrapper in for the real one through it.
var namespaces = proc.Default.Namespaces

// answeredBy checks the process listening at the other end of conn.
func (e Expect) answeredBy(conn *net.UnixConn) error {
	peer, err := peerOf(conn)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	refuse := func(format string, args ...any) error {
		return fmt.Errorf("%w: "+format, append([]any{ErrNotConfirmed}, args...)...)
	}
	if int(peer.Uid) != os.Getuid() {
		return refuse("the socket is served by another user")
	}
	if int(peer.Pid) != e.PID || e.PID <= 0 {
		return refuse("the socket is served by process %d, not the sending session's wrapper %d", peer.Pid, e.PID)
	}
	if !proc.Alive(e.PID, e.Start) {
		return refuse("process %d is not the sending session's wrapper as it started", e.PID)
	}
	if err := sameNamespaces(e.PID); err != nil {
		if errors.Is(err, errOwnNamespaces) {
			return fmt.Errorf("%w: %v", ErrUnreachable, err)
		}
		return refuse("the sending session's wrapper runs in other namespaces than this one, as a sandboxed process would")
	}
	return nil
}

// errOwnNamespaces is this process failing to read its own namespaces:
// nothing learned about the other one.
var errOwnNamespaces = errors.New("cannot read this process's namespaces")

// sameNamespaces says whether a process shares this one's mount, user and
// PID namespaces. A sandbox starts its commands in namespaces of their own,
// and neither end of a grant is to be one of them: not the wrapper that
// confirms a grant, nor the command that registers one — a sandboxed
// process main itself started runs below main's wrapper too.
func sameNamespaces(pid int) error {
	own, err := namespaces("self")
	if err != nil {
		return fmt.Errorf("%w: %v", errOwnNamespaces, err)
	}
	theirs, err := namespaces(strconv.Itoa(pid))
	if err != nil {
		return err
	}
	if theirs != own {
		return errors.New("other namespaces than this process")
	}
	return nil
}

func dial(path string) (*net.UnixConn, error) {
	conn, err := net.DialTimeout("unix", path, exchangeLimit)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	unix, ok := conn.(*net.UnixConn)
	if !ok {
		_ = conn.Close()
		return nil, fmt.Errorf("%w: %s is not a unix socket", ErrUnreachable, path)
	}
	_ = unix.SetDeadline(time.Now().Add(exchangeLimit))
	return unix, nil
}

func exchange(conn *net.UnixConn, asked request) (response, error) {
	if err := json.NewEncoder(conn).Encode(asked); err != nil {
		return response{}, fmt.Errorf("%w: %v", ErrUnreachable, err)
	}
	line, err := bufio.NewReader(io.LimitReader(conn, maxLine)).ReadBytes('\n')
	if err != nil {
		return response{}, fmt.Errorf("%w: no answer: %v", ErrUnreachable, err)
	}
	var answer response
	if err := json.Unmarshal(line, &answer); err != nil {
		return response{}, fmt.Errorf("%w: an answer that does not parse", ErrNotConfirmed)
	}
	return answer, nil
}

// maxLine bounds one line either way: eight directories and their names.
const maxLine = 64 << 10

// peerOf reads the credentials of the process at the other end of a unix
// socket, as the kernel recorded them when it connected or listened.
func peerOf(conn *net.UnixConn) (*syscall.Ucred, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return nil, err
	}
	var cred *syscall.Ucred
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		cred, credErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil {
		return nil, err
	}
	return cred, credErr
}
