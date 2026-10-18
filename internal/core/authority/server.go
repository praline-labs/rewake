package authority

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/praline-labs/rewake/internal/infra/proc"
)

// Authority is the run's address and the wrapper's answers there: every
// wrapper takes the decisions on reports addressed to its run (decide.go).
type Authority struct {
	// Self is the wrapper; a question comes from a process below it.
	Self int

	listener *net.UnixListener
	mu       sync.Mutex
	decide   DecideHandler
	deciding int
	done     chan struct{}
	once     sync.Once
}

// Listen binds the run's address, an abstract unix socket: nothing on disk
// for anyone to replace or leave behind. An address already bound is
// somebody else's, and this wrapper then answers for no decision.
func Listen(address string, self int) (*Authority, error) {
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: address, Net: "unix"})
	if err != nil {
		return nil, err
	}
	return &Authority{Self: self, listener: listener, done: make(chan struct{})}, nil
}

// Serve answers until the context ends or Close is called.
func (a *Authority) Serve(ctx context.Context) {
	acceptLoop(ctx, a.listener, a.done, a.Close, a.answer)
}

// acceptLoop hands each connection to answer until the context ends or done
// closes.
func acceptLoop(ctx context.Context, listener *net.UnixListener, done chan struct{}, closer func(), answer func(*net.UnixConn)) {
	go func() {
		select {
		case <-ctx.Done():
			closer()
		case <-done:
		}
	}()
	for {
		conn, err := listener.AcceptUnix()
		if err != nil {
			select {
			case <-done:
				return
			default:
			}
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		go answer(conn)
	}
}

// Close stops answering.
func (a *Authority) Close() {
	a.once.Do(func() {
		close(a.done)
		_ = a.listener.Close()
	})
}

func (a *Authority) answer(conn *net.UnixConn) {
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(exchangeLimit))
	line, err := bufio.NewReader(io.LimitReader(conn, maxLine)).ReadBytes('\n')
	if err != nil {
		return
	}
	var asked request
	var answer response
	switch {
	case json.Unmarshal(line, &asked) != nil:
		answer.Error = "a request that does not parse"
	case asked.Op == opDecide:
		reply := a.answerDecide(conn, asked.Decide)
		answer.Decided = &reply
	default:
		answer.Error = "an operation this wrapper does not know: " + asked.Op
	}
	_ = json.NewEncoder(conn).Encode(answer)
}

// fromBelow refuses a caller that is not a command this wrapper's session
// runs, outside any sandbox.
func (a *Authority) fromBelow(conn *net.UnixConn) error {
	peer, err := peerOf(conn)
	if err != nil {
		return fmt.Errorf("cannot tell who asked: %v", err)
	}
	if int(peer.Uid) != os.Getuid() {
		return errors.New("asked by another user")
	}
	if err := proc.Default.Descends(int(peer.Pid), a.Self); err != nil {
		return fmt.Errorf("only a command this session runs asks for its decisions: %v", err)
	}
	if err := sameNamespaces(int(peer.Pid)); err != nil {
		return fmt.Errorf("a command in a sandbox of its own does not ask for a decision, even one this session started: %v", err)
	}
	return nil
}
