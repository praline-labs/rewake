package grantauth

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

	"github.com/iiiokojiadbi/rewake/internal/proc"
)

// maxHeld bounds what one main holds at once. A registration past it is
// refused, never one already held pushed out: a grant that loses its record
// is one nobody can confirm.
const maxHeld = 256

// Authority holds the grants of one main run, in memory only: a record on
// disk is one a sandboxed worker could write.
type Authority struct {
	// Self is the wrapper; a registration comes from a process below it.
	Self int
	// Lifetime is how long a grant stays confirmable: the time a task
	// carrying one may wait for delivery, with some to spare.
	Lifetime time.Duration

	listener *net.UnixListener
	mu       sync.Mutex
	held     map[string]heldGrant
	done     chan struct{}
	once     sync.Once
}

type heldGrant struct {
	grant Grant
	at    time.Time
}

// Listen binds the run's address, an abstract unix socket: nothing on disk
// for anyone to replace or leave behind. An address already bound is
// somebody else's, and this wrapper then answers for no grant.
func Listen(address string, self int, lifetime time.Duration) (*Authority, error) {
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: address, Net: "unix"})
	if err != nil {
		return nil, err
	}
	return &Authority{Self: self, Lifetime: lifetime, listener: listener, held: map[string]heldGrant{}, done: make(chan struct{})}, nil
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

// Close stops answering, and every grant it held goes with it.
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
	if json.Unmarshal(line, &asked) != nil {
		answer.Error = "a request that does not parse"
	} else {
		switch asked.Op {
		case opRegister:
			if err := a.register(conn, asked.Grant); err != nil {
				answer.Error = err.Error()
			}
		case opConfirm:
			grant, err := a.confirm(conn, asked.Grant)
			if err != nil {
				answer.Error = err.Error()
			} else {
				answer.Grant = &grant
			}
		default:
			answer.Error = "an operation this wrapper does not know: " + asked.Op
		}
	}
	_ = json.NewEncoder(conn).Encode(answer)
}

// register takes a grant from a process running below this wrapper: this
// session's own `rewake send`. Nothing else may add one.
func (a *Authority) register(conn *net.UnixConn, grant Grant) error {
	peer, err := peerOf(conn)
	if err != nil {
		return fmt.Errorf("cannot tell who asked: %v", err)
	}
	if int(peer.Uid) != os.Getuid() {
		return errors.New("asked by another user")
	}
	if err := proc.Default.Descends(int(peer.Pid), a.Self); err != nil {
		return fmt.Errorf("only a command this session runs registers its grants: %v", err)
	}
	if err := sameNamespaces(int(peer.Pid)); err != nil {
		return fmt.Errorf("a command in a sandbox of its own does not register a grant, even one this session started: %v", err)
	}
	if grant.ID == "" || grant.To == "" || grant.ToEpoch == "" || len(grant.Dirs) == 0 && !grant.Git {
		return errors.New("a grant names its message, its recipient's run and what it grants")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.prune()
	if held, ok := a.held[grant.ID]; ok {
		if held.grant.Same(grant) {
			return nil
		}
		return fmt.Errorf("message %s already carries another grant", grant.ID)
	}
	if len(a.held) >= maxHeld {
		return fmt.Errorf("this session holds %d grants waiting for delivery, the most it keeps; wait for some to be delivered or to expire", maxHeld)
	}
	a.held[grant.ID] = heldGrant{grant: grant, at: time.Now()}
	return nil
}

// confirm answers for a grant to whoever runs as this user: an abstract
// address has no file mode to keep others out.
func (a *Authority) confirm(conn *net.UnixConn, asked Grant) (Grant, error) {
	if peer, err := peerOf(conn); err != nil || int(peer.Uid) != os.Getuid() {
		return Grant{}, errors.New("asked by another user")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.prune()
	held, ok := a.held[asked.ID]
	if !ok || held.grant.To != asked.To || held.grant.ToEpoch != asked.ToEpoch {
		return Grant{}, fmt.Errorf("this session registered no grant with message %s for that run", asked.ID)
	}
	return held.grant, nil
}

// prune forgets what has outlived the wait of its message. Call with the
// lock held.
func (a *Authority) prune() {
	for id, held := range a.held {
		if time.Since(held.at) > a.Lifetime {
			delete(a.held, id)
		}
	}
}
