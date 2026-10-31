package mirror

// Event is one thing an adapter saw of its session's conversation, in the
// neutral words every adapter uses: the model is built of these alone, and
// knows no harness.
type Event struct {
	Kind EventKind
	// ID names the item: a prompt's, a block's, a tool call's.
	ID string
	// Turn is the harness's turn the event belongs to, when it names one.
	Turn string
	// Block is what a delta or a settle streams: ItemKindText or
	// ItemKindThinking.
	Block ItemKind
	// Text is a delta's text, a settle's part, a prompt's text, a tool's
	// input or its result, a notice.
	Text string
	// Origin is who typed a prompt.
	Origin string
	// Name is a tool's name.
	Name string
	// Status is how a tool call ended: done, error or refused; error on a
	// notice telling a failure or an interrupt; refused on a prompt.
	Status ItemStatus
	// State is the turn's new state.
	State TurnState
	// Part and Of place a settle's part among the block's parts, from 0.
	Part, Of int
	// Lost counts the events a resync stands for.
	Lost int
	// Permit is how a result's call was let run or refused, where the
	// adapter could tell.
	Permit *Permit
	// Mode is the session's permission mode, on a mode event.
	Mode PermissionMode
}

// EventKind is what an event says.
type EventKind string

// The events. A block streams as deltas, then settles whole in parts, so a
// delta lost on the way heals at the block's end; lost names a block whose
// settle was dropped, and resync stands for whatever else was.
const (
	EventPrompt EventKind = "prompt"
	EventDelta  EventKind = "delta"
	EventSettle EventKind = "settle"
	EventTool   EventKind = "tool"
	EventInput  EventKind = "input"
	EventResult EventKind = "result"
	EventTurn   EventKind = "turn"
	EventLost   EventKind = "lost"
	EventResync EventKind = "resync"
	EventNotice EventKind = "notice"
	EventMode   EventKind = "mode"
)

// shortField bounds an id, a turn, a name or an origin: they are words, and
// a longer one is cut rather than let an item past its bound.
const shortField = 256

func short(text string) string {
	cut, _ := prefixWithin(text, shortField)
	return cut
}
