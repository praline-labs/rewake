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

	"github.com/praline-labs/rewake/internal/proc"
)

// maxHeld bounds what one main holds at once. A registration past it is
// refused, never one already held pushed out: a grant that loses its record
// is one nobody can confirm.
const maxHeld = 256

// Authority holds the grants of one main run, in memory only: a record on
// disk is one a sandboxed worker could write.
//
// A grant is held while its task is open, not for a fixed time: a worker
// resumed cold asks for it again (Reconfirm), and the answer has to be here
// for as long as the task may still be worked on.
type Authority struct {
	// Self is the wrapper; a registration comes from a process below it.
	Self int
	// Lifetime is how long a grant whose task has not been found yet stays
	// confirmable: the time a task carrying one may wait for delivery, with
	// some to spare. A registration is made before its letter is written, and
	// a letter that never was leaves nothing that could close it.
	Lifetime time.Duration
	// Open says whether the task a grant came with is still open — delivered
	// or on its way, or read and not yet reported on — and whether its letter
	// was found at all. Nil holds every grant for Lifetime.
	Open func(Grant) (open, found bool)
	// Owns says whether a run holds a session name now. Nil takes the run a
	// question names as the holder.
	Owns func(name, epoch string) bool

	listener *net.UnixListener
	mu       sync.Mutex
	held     map[string]heldGrant
	done     chan struct{}
	once     sync.Once
}

type heldGrant struct {
	grant Grant
	at    time.Time
	// delivered is a grant confirmed to its recipient's wrapper once: only
	// such a grant can have reached a conversation a resume continues.
	delivered bool
	// thread is the conversation the confirmation named: a resume of it,
	// and of no other, may have the grant again. Kept here because the copy
	// on disk that names it is one a worker could write.
	thread string
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
		case opConfirm, opReconfirm:
			confirm := a.confirm
			if asked.Op == opReconfirm {
				confirm = a.reconfirm
			}
			grant, err := confirm(conn, asked.Grant, asked.Thread)
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
		return fmt.Errorf("this session holds %d grants whose tasks are open, the most it keeps; wait for some of those tasks to be reported on, or take some back", maxHeld)
	}
	a.held[grant.ID] = heldGrant{grant: grant, at: time.Now()}
	return nil
}

// confirm answers for a grant to whoever runs as this user: an abstract
// address has no file mode to keep others out. It takes the grant for
// delivered, into the conversation named, only from the wrapper of the run it
// was granted to, and keeps the first conversation named: a worker reads its
// letter before delivery, and it or any other process of this user could
// otherwise name a conversation of its own choosing for a later resume to
// take the grant into.
func (a *Authority) confirm(conn *net.UnixConn, asked Grant, thread string) (Grant, error) {
	peer, err := peerOf(conn)
	if err != nil || int(peer.Uid) != os.Getuid() {
		return Grant{}, errors.New("asked by another user")
	}
	delivering := askedByRun(conn, asked.ToEpoch) == nil
	a.mu.Lock()
	defer a.mu.Unlock()
	a.prune()
	held, ok := a.held[asked.ID]
	if !ok || held.grant.To != asked.To || held.grant.ToEpoch != asked.ToEpoch {
		return Grant{}, fmt.Errorf("this session registered no grant with message %s for that run", asked.ID)
	}
	if delivering {
		held.delivered = true
		if held.thread == "" {
			held.thread = thread
		}
		a.held[asked.ID] = held
	}
	return held.grant, nil
}

// prune forgets the grants whose tasks are closed, and those whose letter was
// never found once the wait of a message is over. Call with the lock held.
func (a *Authority) prune() {
	for id, held := range a.held {
		if a.Open != nil {
			if open, found := a.Open(held.grant); found {
				if !open {
					delete(a.held, id)
				}
				continue
			}
		}
		if time.Since(held.at) > a.Lifetime {
			delete(a.held, id)
		}
	}
}
