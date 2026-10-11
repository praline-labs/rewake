package endpoint

import (
	"net"
	"time"

	"github.com/praline-labs/rewake/internal/core/channel"
	"github.com/praline-labs/rewake/internal/infra/boottime"
)

// The endpoint's share of the channel record
// (docs/mail-bridge-channel.md#the-tool-observation): it sees the transport
// named and ended, every refused transport hello and every ticket a child
// validates.
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

// named makes peer the transport's process and the transport the live one:
// the channel's hello, a generation of its own, after the close of the one it
// replaces. The tool transport's lifecycle is the channel's: naming a process
// is the capability going live; naming another or none, or the endpoint
// closing, ends the generation before
// (docs/mail-bridge-channel.md#the-tool-observation). Both are stamped under
// the lock that names the process, so their times keep the order of the
// namings, and told outside it, so a slow keeper holds no request up.
func (e *Endpoint) named(peer transportPeer) {
	e.mu.Lock()
	e.transport = peer
	events := e.generationEnds(peer.pid != 0 && !e.closed)
	e.mu.Unlock()
	e.publish(events)
}

// closedLive ends the live generation as the endpoint closes. The transport
// stays named: a call Close let run still answers its process.
func (e *Endpoint) closedLive() {
	e.mu.Lock()
	events := e.generationEnds(false)
	e.mu.Unlock()
	e.publish(events)
}

// generationEnds closes the live generation and, when next, opens another,
// returning the events stamped now. The caller holds e.mu.
func (e *Endpoint) generationEnds(next bool) []channel.Event {
	var events []channel.Event
	if e.live != 0 {
		// Whether the harness lives and is not ending is the wrapper's to
		// say, after the event: the endpoint only knows the transport ended.
		events = append(events, channel.Event{Kind: channel.Closed, Generation: e.live, At: Stamp()})
		e.live = 0
	}
	if next {
		e.generation++
		e.live = e.generation
		events = append(events, channel.Event{Kind: channel.Hello, Generation: e.live, At: Stamp()})
	}
	return events
}

// publish tells events already stamped, outside the lock.
func (e *Endpoint) publish(events []channel.Event) {
	if e.cfg.Channel == nil {
		return
	}
	for _, event := range events {
		e.cfg.Channel(event)
	}
}

// refused tells of a transport's hello the endpoint refused, and whether it
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
