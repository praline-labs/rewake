package mirror

// WireEvent is an event as an adapter's harness side sends it through the
// feed (feed.ts), in the same neutral words on every adapter's wire: the
// adapter decodes its batch's events into this and hands them to Decode, so
// none maps the fields by hand.
type WireEvent struct {
	Kind   EventKind  `json:"kind"`
	ID     string     `json:"id,omitempty"`
	Turn   string     `json:"turn,omitempty"`
	Block  ItemKind   `json:"block,omitempty"`
	Text   string     `json:"text,omitempty"`
	Origin string     `json:"origin,omitempty"`
	Name   string     `json:"name,omitempty"`
	Status ItemStatus `json:"status,omitempty"`
	State  TurnState  `json:"state,omitempty"`
	Part   int        `json:"part,omitempty"`
	Of     int        `json:"of,omitempty"`
	Lost   int        `json:"lost,omitempty"`
	Permit *Permit    `json:"permit,omitempty"`
	// Mode is the session's permission mode in its harness's own name,
	// which Decode turns into the model's word by the adapter's table.
	Mode string `json:"mode,omitempty"`
}

// sentKinds are the events the model takes from a wire.
var sentKinds = map[EventKind]bool{
	EventPrompt: true, EventDelta: true, EventSettle: true, EventTool: true, EventInput: true, EventResult: true,
	EventTurn: true, EventMode: true, EventNotice: true, EventLost: true, EventResync: true, EventTaken: true,
}

// sentStates are the states a turn event may say; unknown and ended are the
// model's own.
var sentStates = map[TurnState]bool{TurnStateWorking: true, TurnStateIdle: true, TurnStateCompacting: true}

// Decode turns a batch's events into the model's. An event of a kind the
// model does not take, or a turn event with no state it knows, is left out,
// never guessed at; a mode modes does not name leaves the mode unsaid. modes
// is nil for a harness that says no permission mode.
func Decode(sent []WireEvent, modes map[string]PermissionMode) []Event {
	events := make([]Event, 0, len(sent))
	for _, e := range sent {
		if !sentKinds[e.Kind] || (e.Kind == EventTurn && !sentStates[e.State]) {
			continue
		}
		events = append(events, Event{
			Kind: e.Kind, ID: e.ID, Turn: e.Turn, Block: e.Block, Text: e.Text, Origin: e.Origin, Name: e.Name,
			Status: e.Status, State: e.State, Part: e.Part, Of: e.Of, Lost: e.Lost, Permit: e.Permit, Mode: modes[e.Mode],
		})
	}
	return events
}
