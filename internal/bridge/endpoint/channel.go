package endpoint

import (
	"encoding/json"
	"errors"
	"net"
	"time"

	"github.com/praline-labs/rewake/internal/boottime"
	"github.com/praline-labs/rewake/internal/channel"
)

// The endpoint's share of the channel record
// (docs/mail-bridge-channel.md#the-tool-observation): it sees every server's
// hello and close, every refused hello, every ticket a child validates and
// every call refused for want of an observation. It tells each to the
// wrapper with the time the event happened and keeps nothing of it.

// opReport is a server telling its run's wrapper something only it saw.
const opReport = "report"

// reportCannotStart is the one thing a server reports: its call's command
// could not start. The class is the endpoint's, never the server's text.
const reportCannotStart = "cannot-start"

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

// SetPrimary names what says which thread the run's gateway holds as
// primary, "" while it holds none; known only once the backend is.
func (e *Endpoint) SetPrimary(primary func() string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.primary = primary
}

// bind tells the thread the first request of a Codex server's connection
// names — a server serves one thread for its life — and refuses a request
// that is not the primary thread's at once: the gateway forwards no other
// thread's items, so waiting for its observation could only time out and
// take a sub-agent's call for the conversation's fault
// (docs/mail-bridge-channel-codex.md#conversation-connections).
func (e *Endpoint) bind(generation uint64, thread string) error {
	e.mu.Lock()
	first := thread != "" && generation != 0 && !e.bound[generation]
	if first {
		if e.bound == nil {
			e.bound = map[uint64]bool{}
		}
		e.bound[generation] = true
	}
	primary := e.primary
	e.mu.Unlock()
	if first {
		e.tell(channel.Event{Kind: channel.Bound, Generation: generation, Thread: thread})
	}
	if primary == nil {
		return nil
	}
	if held := primary(); thread == "" || thread != held {
		return errors.New("the call is another thread's than the conversation this run holds, a nested agent's among them, and does not run through the tool")
	}
	return nil
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

// report takes what a server reports, with its connection's generation:
// on Codex only the conversation's own server's report fails its channel.
func (e *Endpoint) report(payload json.RawMessage, generation uint64) error {
	var what string
	if json.Unmarshal(payload, &what) != nil || what != reportCannotStart {
		return errors.New("a report this endpoint does not take")
	}
	e.tell(channel.Event{Kind: channel.CannotStart, Generation: generation})
	return nil
}

// Report tells the run's wrapper that a call's command could not start. It
// waits for no answer: the call answers its model either way, and a wrapper
// that is gone is told nothing.
func (c *Client) Report() {
	c.mu.Lock()
	failed := c.failed
	c.next++
	id := c.next
	c.mu.Unlock()
	if failed != nil {
		return
	}
	payload, _ := json.Marshal(reportCannotStart)
	encoded, _ := json.Marshal(request{ID: id, Op: opReport, Payload: payload})
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(shortExchange))
	_, _ = c.conn.Write(append(encoded, '\n'))
}
