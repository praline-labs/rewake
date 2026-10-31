package mirror

import (
	"bytes"
	"encoding/json"
	"sync"
)

// client is one face's queue of lines. The model never waits on a face: a
// face that falls past its bound is cut off, and reconnects for a fresh
// snapshot rather than be given a gap.
type client struct {
	mu       sync.Mutex
	lines    [][]byte
	size     int
	maxLines int
	maxBytes int
	gone     bool
	wake     chan struct{}
}

func (c *client) push(line []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gone {
		return
	}
	if len(c.lines)+1 > c.maxLines || c.size+len(line) > c.maxBytes {
		c.gone, c.lines, c.size = true, nil, 0
	} else {
		c.lines = append(c.lines, line)
		c.size += len(line)
	}
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

// take is what is queued, and whether the face is still served.
func (c *client) take() ([][]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	lines := c.lines
	c.lines, c.size = nil, 0
	return lines, !c.gone
}

func (c *client) end() {
	c.mu.Lock()
	c.gone = true
	c.mu.Unlock()
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

// encode is a line as it goes out, newline included; HTML is not escaped,
// so a text's size is what clip counted.
func encode(line Line) []byte {
	var b bytes.Buffer
	encoder := json.NewEncoder(&b)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(line); err != nil {
		// Nothing a line holds fails to encode; an empty line is skipped
		// by the face.
		return []byte("\n")
	}
	return b.Bytes()
}

// send queues a change for every face; called with the model's lock held, so
// the faces see the changes in the model's order.
func (m *Model) send(line Line) {
	if len(m.clients) == 0 {
		return
	}
	encoded := encode(line)
	for c := range m.clients {
		c.push(encoded)
	}
}

// Snapshot is the model as a face connecting now gets it.
func (m *Model) Snapshot() []Line {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snapshotLocked()
}

func (m *Model) snapshotLocked() []Line {
	inputs := []InputKind{}
	if m.inputs != nil {
		inputs = append(inputs, m.takes...)
	}
	begin := Line{Op: LineOpBegin, Items: ptr(len(m.order)), Dropped: ptr(m.dropped), Turn: ptr(m.turn), Mode: m.mode}
	switch {
	case m.turn == TurnStateWorking && m.since > 0:
		begin.Since = ptr(m.since)
	case m.turn == TurnStateIdle && m.worked != nil:
		begin.Took, begin.At = ptr(m.worked.took), ptr(m.worked.at)
		if m.worked.stopped {
			begin.Stopped = ptr(true)
		}
	}
	lines := []Line{{Op: LineOpHello, V: ptr(Version), Inputs: inputs}, begin}
	for _, at := range m.order {
		item := at.whole()
		lines = append(lines, Line{Op: LineOpItem, Item: &item})
	}
	return append(lines, Line{Op: LineOpEnd})
}

// subscribe is a new face's queue, which starts with the snapshot: taken
// under the model's lock with the subscription, so no change falls between
// the two. The queue's bound counts from the snapshot's end.
func (m *Model) subscribe() *client {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := &client{wake: make(chan struct{}, 1)}
	for _, line := range m.snapshotLocked() {
		encoded := encode(line)
		c.lines = append(c.lines, encoded)
		c.size += len(encoded)
	}
	c.maxLines, c.maxBytes = len(c.lines)+MaxClientOps, c.size+MaxModelBytes
	m.clients[c] = struct{}{}
	return c
}

func (m *Model) unsubscribe(c *client) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.clients, c)
	c.end()
}

// End tells every face the session ended.
func (m *Model) End() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.turn = TurnStateEnded
	m.send(Line{Op: LineOpTurn, Turn: &m.turn})
}
