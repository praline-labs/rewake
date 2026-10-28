/*
Package mirror is the model of a session's conversation that rewake's own
face draws (docs/face.md): the items, the turn's state and the inputs back,
built from the neutral events an adapter hands over and served to the face as
JSON lines. It knows no harness, and it lives in memory only: nothing of it
is ever written to disk, a letter, a log or the room (docs/project.md).

Every byte figure is bounded, so whatever the session says the model, a
snapshot of it and a face's queue fit their bounds (mirror.schema.json).
*/
package mirror

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"sync"
)

// Model is one session's conversation as the face draws it.
type Model struct {
	mu      sync.Mutex
	order   []*entry
	byID    map[string]*entry
	bytes   int
	dropped int
	turn    TurnState
	parts   map[string]*assembly
	// born numbers the assemblies, so the oldest goes first past a bound.
	born    int
	clients map[*client]struct{}
	inputs  func(context.Context, Input) error
	// notices numbers rewake's own notices, their ids.
	notices int
}

// entry is an item with its texts kept within their bounds.
type entry struct {
	item         Item
	text, output clip
	size         int
}

// assembly is a settle being put together from its parts.
type assembly struct {
	turn  string
	block ItemKind
	next  int
	of    int
	text  clip
	size  int
	born  int
}

// itemOverhead is what an item's envelope and words take besides its texts.
const itemOverhead = 4 * shortField

// New is an empty model, its turn idle.
func New() *Model {
	return &Model{byID: map[string]*entry{}, parts: map[string]*assembly{}, clients: map[*client]struct{}{}, turn: TurnStateIdle}
}

// ErrNoInputs refuses an input of a session that takes none.
var ErrNoInputs = errors.New("this session takes no input from the face")

// Take names what carries the face's inputs to the harness; until it is
// called, every input is refused.
func (m *Model) Take(inputs func(context.Context, Input) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inputs = inputs
}

// Input carries one input of the face to the harness.
func (m *Model) Input(ctx context.Context, input Input) error {
	m.mu.Lock()
	inputs := m.inputs
	m.mu.Unlock()
	if inputs == nil {
		return ErrNoInputs
	}
	return inputs(ctx, input)
}

// Apply takes events in the order the adapter saw them.
func (m *Model) Apply(events ...Event) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, event := range events {
		m.apply(event)
	}
	m.trim()
}

func (m *Model) apply(e Event) {
	id := short(e.ID)
	switch e.Kind {
	case EventPrompt, EventNotice:
		kind := ItemKindPrompt
		if e.Kind == EventNotice {
			kind = ItemKindNotice
		}
		status := ItemStatusSettled
		if e.Status == ItemStatusRefused {
			status = ItemStatusRefused
		}
		if m.byID[id] == nil {
			m.add(&entry{item: Item{ID: id, Turn: word(e.Turn), Kind: kind, Status: status, Origin: word(e.Origin)}, text: newClip(textMax, e.Text)})
		}
	case EventDelta:
		m.delta(id, e)
	case EventSettle:
		m.settle(id, e)
	case EventTool:
		if m.byID[id] == nil {
			m.add(&entry{item: Item{ID: id, Turn: word(e.Turn), Kind: ItemKindTool, Status: ItemStatusRunning, Name: word(e.Name)}, text: clip{max: MaxFieldBytes}, output: clip{max: MaxFieldBytes}})
		}
	case EventInput, EventResult:
		if at := m.byID[id]; at != nil && at.item.Kind == ItemKindTool {
			if e.Kind == EventInput {
				at.text.set(e.Text)
			} else {
				at.output.set(e.Text)
				at.item.Status = toolEnd(e.Status)
			}
			m.changed(at)
		}
	case EventTurn:
		m.turnTo(e)
	case EventLost:
		m.forget(id)
		m.gap(m.byID[id])
	case EventResync:
		for id := range m.parts {
			m.forget(id)
		}
		for _, at := range m.order {
			m.gap(at)
		}
		m.turn = TurnStateUnknown
		m.send(Line{Op: LineOpTurn, Turn: &m.turn})
		if e.Lost > 0 {
			m.notices++
			m.add(&entry{item: Item{ID: "notice-" + strconv.Itoa(m.notices), Kind: ItemKindNotice, Status: ItemStatusSettled}, text: newClip(textMax, lostNotice(e.Lost))})
		}
	}
}

// textMax bounds a prompt's, a reply's or a notice's text.
const textMax = MaxItemBytes - itemOverhead

func (m *Model) delta(id string, e Event) {
	at := m.byID[id]
	if at == nil {
		m.add(&entry{item: Item{ID: id, Turn: word(e.Turn), Kind: block(e.Block), Status: ItemStatusStreaming}, text: newClip(textMax, e.Text)})
		return
	}
	if at.item.Status != ItemStatusStreaming || e.Text == "" {
		return
	}
	if at.text.add(e.Text) {
		m.resize(at)
		m.send(Line{Op: LineOpAppend, ID: &at.item.ID, Text: &e.Text})
		return
	}
	m.changed(at)
}

// settle puts a block's parts together and replaces the item with the whole
// of it once the last comes. A part out of order leaves what streamed so far,
// marked as a gap.
func (m *Model) settle(id string, e Event) {
	a := m.parts[id]
	if e.Part == 0 {
		m.forget(id)
		m.born++
		a = &assembly{turn: e.Turn, block: block(e.Block), of: e.Of, text: clip{max: textMax}, size: itemOverhead, born: m.born}
		m.parts[id] = a
		m.bytes += a.size
	}
	if a == nil || e.Part != a.next || e.Of != a.of {
		m.forget(id)
		m.gap(m.byID[id])
		return
	}
	a.text.add(e.Text)
	m.bytes -= a.size
	a.size = a.text.size() + itemOverhead
	m.bytes += a.size
	a.next++
	if a.next < a.of {
		return
	}
	m.forget(id)
	at := m.byID[id]
	if at == nil {
		m.add(&entry{item: Item{ID: id, Turn: word(a.turn), Kind: a.block, Status: ItemStatusSettled}, text: a.text})
		return
	}
	at.text, at.item.Status = a.text, ItemStatusSettled
	m.changed(at)
}

// turnTo moves the turn. Any turn event ends what was being put together of
// that turn, and a turn's end leaves a block still streaming as a gap: its
// settle was lost.
func (m *Model) turnTo(e Event) {
	for id, a := range m.parts {
		if a.turn == e.Turn {
			m.forget(id)
			m.gap(m.byID[id])
		}
	}
	// A turn that stopped working leaves nothing in flight: a call the engine
	// refused before any hook saw it never gets its result.
	if e.State != TurnStateWorking {
		for _, at := range m.order {
			if inFlight(at.item.Status) && (at.item.Turn == nil || *at.item.Turn == e.Turn) {
				m.gap(at)
			}
		}
	}
	m.turn = e.State
	m.send(Line{Op: LineOpTurn, Turn: &m.turn})
}

func inFlight(s ItemStatus) bool { return s == ItemStatusStreaming || s == ItemStatusRunning }

// gap marks an item still in flight as partly lost.
func (m *Model) gap(at *entry) {
	if at == nil || !inFlight(at.item.Status) {
		return
	}
	at.item.Status = ItemStatusGap
	m.changed(at)
}

func (m *Model) add(at *entry) {
	at.size = at.text.size() + at.output.size() + itemOverhead
	m.order = append(m.order, at)
	m.byID[at.item.ID] = at
	m.bytes += at.size
	item := at.whole()
	m.send(Line{Op: LineOpAdd, Item: &item})
}

// changed sends an item whole after a change other than an append.
func (m *Model) changed(at *entry) {
	m.resize(at)
	item := at.whole()
	m.send(Line{Op: LineOpSet, Item: &item})
}

// resize counts an item's new size in the model's.
func (m *Model) resize(at *entry) {
	size := at.text.size() + at.output.size() + itemOverhead
	m.bytes += size - at.size
	at.size = size
}

// forget drops a settle being put together, and its bytes.
func (m *Model) forget(id string) {
	if a := m.parts[id]; a != nil {
		m.bytes -= a.size
		delete(m.parts, id)
	}
}

// trim holds the model within its bounds, counting every byte it keeps:
// items and settles being put together alike. The oldest items no longer in
// flight go first, then the oldest settles being put together, each leaving
// its item a gap, and last the oldest items in flight, so a stream that never
// settles cannot outgrow the bounds either.
func (m *Model) trim() {
	for len(m.order)+len(m.parts) > MaxModelItems || m.bytes > MaxModelBytes {
		i := slices.IndexFunc(m.order, func(at *entry) bool { return !inFlight(at.item.Status) })
		switch {
		case i >= 0:
			m.drop(i)
		case len(m.parts) > 0:
			m.dropOldestAssembly()
		case len(m.order) > 0:
			m.drop(0)
		default:
			return
		}
	}
}

func (m *Model) drop(i int) {
	at := m.order[i]
	m.order = slices.Delete(m.order, i, i+1)
	delete(m.byID, at.item.ID)
	m.bytes -= at.size
	m.dropped++
	m.send(Line{Op: LineOpDrop, ID: &at.item.ID, Dropped: &m.dropped})
}

// dropOldestAssembly drops the settle put together longest ago; its item, if
// any, is left a gap with what streamed.
func (m *Model) dropOldestAssembly() {
	var oldest *assembly
	var id string
	for at, a := range m.parts {
		if oldest == nil || a.born < oldest.born {
			oldest, id = a, at
		}
	}
	m.forget(id)
	m.gap(m.byID[id])
}

// whole is the item as a line carries it.
func (at *entry) whole() Item {
	item := at.item
	item.Text = at.text.String()
	if at.item.Kind == ItemKindTool && (at.output.size() > 0 || at.item.Status != ItemStatusRunning) {
		output := at.output.String()
		item.Output = &output
	}
	if at.text.cut || at.output.cut {
		item.Cut = ptr(true)
	}
	return item
}

func block(kind ItemKind) ItemKind {
	if kind == ItemKindThinking {
		return kind
	}
	return ItemKindText
}

func toolEnd(status ItemStatus) ItemStatus {
	switch status {
	case ItemStatusError, ItemStatusRefused:
		return status
	}
	return ItemStatusDone
}

// word is an optional short field, absent when empty.
func word(text string) *string {
	if text == "" {
		return nil
	}
	return ptr(short(text))
}

func ptr[T any](value T) *T { return &value }

func lostNotice(n int) string {
	if n == 1 {
		return "1 event was lost while the link was slow; the native face has it."
	}
	return strconv.Itoa(n) + " events were lost while the link was slow; the native face has them."
}
