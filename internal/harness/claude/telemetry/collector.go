package telemetry

import (
	"context"
	"errors"
	"net"
	"os"
	"sync"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/sessionstate"
)

// maxCompactionEvents bounds the notification cues kept in a snapshot, the
// same bound the Codex side keeps: a reader that falls further behind than
// this has missed the cue, not the count.
const maxCompactionEvents = 64

// Collector is the wrapper's end: it listens for events and folds them into
// the snapshot the wrapper publishes. It exists for exactly as long as the
// wrapper does, like everything else of a session.
type Collector struct {
	path string

	mu     sync.Mutex
	conn   *net.UnixConn
	done   chan struct{}
	folded state
	// drawn is closed on the first status line; see Drawn.
	drawn     chan struct{}
	drawnOnce sync.Once
}

// NewCollector listens at path once started.
func NewCollector(path string) *Collector {
	return &Collector{path: path, done: make(chan struct{}), drawn: make(chan struct{})}
}

// Drawn is closed once the harness has run its status line for the first time.
//
// It marks the moment a new session can take a cross-session message without
// holding it. Claude Code holds every line that arrives before its interface is
// up, and SessionStart comes too early to say so: live on 2.1.280 it ran about
// 80 ms after the socket appeared and a line sent then was still held, while
// the first status line ran after the interface was up and a line sent then
// was accepted (docs/research.md).
func (c *Collector) Drawn() <-chan struct{} { return c.drawn }

// Path is where the senders write.
func (c *Collector) Path() string { return c.path }

// Start binds the socket and reads until Close. A leftover file from a wrapper
// that died is replaced: its path names that run, so nobody else is using it.
func (c *Collector) Start(ctx context.Context) error {
	if info, err := os.Lstat(c.path); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return errors.New("the telemetry socket path is taken by something that is not a socket")
		}
		_ = os.Remove(c.path)
	}
	conn, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: c.path, Net: "unixgram"})
	if err != nil {
		return err
	}
	// Only this user may write here; the directory already says so, and the
	// socket says it again for a directory somebody loosened.
	if err := os.Chmod(c.path, 0o600); err != nil {
		_ = conn.Close()
		_ = os.Remove(c.path)
		return err
	}
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	go c.read(conn)
	go func() {
		select {
		case <-ctx.Done():
			c.Close()
		case <-c.done:
		}
	}()
	return nil
}

func (c *Collector) read(conn *net.UnixConn) {
	buffer := make([]byte, maxDatagram+1)
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			select {
			case <-c.done:
				return
			default:
			}
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		event, ok := decodeEvent(buffer[:n])
		if !ok {
			continue
		}
		c.mu.Lock()
		c.folded.apply(event, time.Now())
		c.mu.Unlock()
		if event.Kind == StatusLine {
			c.drawnOnce.Do(func() { close(c.drawn) })
		}
	}
}

// Close stops listening and removes the socket.
func (c *Collector) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	select {
	case <-c.done:
		return
	default:
	}
	close(c.done)
	if c.conn != nil {
		_ = c.conn.Close()
		_ = os.Remove(c.path)
		_ = os.RemoveAll(TurnStartPath(c.path))
	}
}

// Thread is the conversation the session's events last named: the harness's
// session_id, which /clear replaces. Empty until an event names
// one, and never an error: without it a delivery is simply not pinned to a
// conversation, and its report carries no threadChanged either way.
func (c *Collector) Thread() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.folded.thread, nil
}

// SessionState is the snapshot the wrapper publishes. It does no filesystem
// work: the wrapper's heartbeat calls it four times a second.
func (c *Collector) SessionState() sessionstate.Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.folded.snapshot()
}
