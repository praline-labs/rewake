package endpoint

import (
	"net"
	"time"

	"github.com/praline-labs/rewake/internal/boottime"
	"github.com/praline-labs/rewake/internal/channel"
)

// The endpoint's share of the channel record
// (docs/mail-bridge-channel.md#the-tool-observation): it sees every server's
// hello and close, every refused hello and every ticket a child validates.
// It tells each to the wrapper with the time the event happened and keeps
// nothing of it.

// tell passes one event to the wrapper, stamped now.
func (e *Endpoint) tell(event channel.Event) {
	if e.cfg.Channel == nil {
		return
	}
	event.At = Stamp()
	e.cfg.Channel(event)
}

// Stamp is the time of an event now, on both clocks.
func Stamp() channel.Stamp {
	return channel.Stamp{Boot: boottime.Now(), Wall: time.Now()}
}

// connected opens a server connection's generation; its close tells the
// wrapper with the same number, so an older server's EOF after a newer hello
// is known for what it is.
func (e *Endpoint) connected() (uint64, func()) {
	e.mu.Lock()
	e.generation++
	generation := e.generation
	e.mu.Unlock()
	e.tell(channel.Event{Kind: channel.Hello, Generation: generation})
	return generation, func() {
		// Whether the harness lives and is not ending is the wrapper's to
		// say, after the event: the endpoint only knows the connection ended.
		e.tell(channel.Event{Kind: channel.Closed, Generation: generation})
	}
}

// refused tells of a server's hello the endpoint refused, and whether it
// came from the harness's tree: a stray process's refusal changes nothing.
func (e *Endpoint) refused(conn *net.UnixConn) {
	descendant := false
	if peer, err := peerOf(conn); err == nil {
		e.mu.Lock()
		roots := e.roots
		e.mu.Unlock()
		descendant = e.below(int(peer.Pid), roots) == nil
	}
	e.tell(channel.Event{Kind: channel.HelloRefused, Descendant: descendant})
}
