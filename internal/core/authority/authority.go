// Package authority is the run's address, where its wrapper takes the
// decision on a report addressed to the run, verified (decide.go).
//
// Everything a command could send it lives in the state directory or the
// environment, which a sandboxed worker can write, so the wrapper believes
// only what the kernel says of the process at the other end: it takes a
// question only from a command running below itself, in its own namespaces,
// and the asker believes the answer only from the process the run names. A
// listener put in the address's place by a sandboxed process lives in the
// sandbox's namespaces and cannot leave them.
package authority

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"syscall"
	"time"

	"github.com/praline-labs/rewake/internal/infra/proc"
)

// request is one question, a JSON line each way.
type request struct {
	Op string `json:"op"`
	// Decide is a decision asked of the recipient run's wrapper (decide.go).
	Decide *decideAsk `json:"decide,omitempty"`
}

type response struct {
	Decided *decided `json:"decided,omitempty"`
	Error   string   `json:"error,omitempty"`
}

// Errors a caller tells apart.
var (
	// ErrUnreachable is a wrapper that could not be asked: nothing answered
	// at its socket, or the answer did not come in time. It may pass.
	ErrUnreachable = errors.New("the wrapper could not be reached")
	// ErrNotConfirmed is an answer that settles it: the wrapper that answered
	// is not the run's, or its answer does not read.
	ErrNotConfirmed = errors.New("not confirmed")
)

// exchangeLimit bounds one conversation with a wrapper.
const exchangeLimit = 2 * time.Second

// Expect is the wrapper an answer has to come from: the process the run
// names, as it started.
type Expect struct {
	PID   int
	Start uint64
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
		return refuse("the socket is served by process %d, not the run's wrapper %d", peer.Pid, e.PID)
	}
	if !proc.Alive(e.PID, e.Start) {
		return refuse("process %d is not the run's wrapper as it started", e.PID)
	}
	if err := sameNamespaces(e.PID); err != nil {
		if errors.Is(err, errOwnNamespaces) {
			return fmt.Errorf("%w: %v", ErrUnreachable, err)
		}
		return refuse("the run's wrapper runs in other namespaces than this one, as a sandboxed process would")
	}
	return nil
}

// errOwnNamespaces is this process failing to read its own namespaces:
// nothing learned about the other one.
var errOwnNamespaces = errors.New("cannot read this process's namespaces")

// sameNamespaces says whether a process shares this one's mount, user and
// PID namespaces. A sandbox starts its commands in namespaces of their own,
// and neither end of a decision is to be one of them: not the wrapper that
// answers, nor the command that asks — a sandboxed process the session
// itself started runs below its wrapper too.
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

// maxLine bounds one line either way.
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
