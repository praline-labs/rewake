package adapter

// Capability names one of the things an adapter may offer
// (docs/v2/design-api.md#the-capabilities). The names are what a run reports
// live and what the session record keeps.
type Capability string

// The capabilities. Launch is required; every other one is optional.
const (
	LaunchCapability        Capability = "launch"
	WakeCapability          Capability = "wake"
	ToolTransportCapability Capability = "tool-transport"
	TurnBoundaryCapability  Capability = "turn-boundary"
	TelemetryCapability     Capability = "telemetry"
	ControlCapability       Capability = "control"
	PermissionsCapability   Capability = "permissions"
	PersonUICapability      Capability = "person-ui"
)

// Offer is what an adapter offers: static, known from the catalog before any
// launch. Whether one run of it can do what is offered is that run's live set,
// which only the run can prove (docs/v2/design-api.md#two-levels-offered-and-live).
// The zero value offers Launch alone, which every adapter has.
type Offer struct {
	// Wake says a notice can reach a running session.
	Wake bool
	// ToolTransport is the transport the harness's own process carries the
	// mail tool's calls under, as a call's binding names it; empty where the
	// adapter carries no tool.
	ToolTransport string
	TurnBoundary  TurnBoundaryOffer
	// Telemetry says the run reports its state.
	Telemetry   bool
	Control     ControlOffer
	Permissions PermissionsOffer
	// PersonUI says the session's mail state is shown to the person
	// (stage 6).
	PersonUI bool
}

// TurnBoundaryOffer is what an adapter says of its turn ends.
type TurnBoundaryOffer struct {
	// Reports says every turn's end of the session's own conversation
	// reaches the core. Only then can a session owe a report.
	Reports bool
	// HoldsEnd says an end can be held open while the core decides, so the
	// core can keep a task open across an interim end. An adapter that
	// reports without it says so here rather than leave the limit to be
	// discovered (docs/v2/design-api.md#turnboundary).
	HoldsEnd bool
}

// ControlOffer is the verbs a main may send a running session, each offered on
// its own; one not offered is refused before a request is written
// (docs/remote-control.md).
type ControlOffer struct {
	Compact, Interrupt, Clear bool
	// CompactFocus says a compaction may carry a focus: text the summary is
	// to keep. Where the harness cannot pass one for a single compaction, a
	// focus is a wrong call, refused before sending.
	CompactFocus bool
	// InterruptTrace says what the interrupted session's model is shown of
	// an interrupt, completing "whoever waits on it reads stopped, and …" in
	// the answer to rewake interrupt.
	InterruptTrace string
}

// Any says at least one verb is offered: the run then has a control
// directory for the requests to arrive in.
func (c ControlOffer) Any() bool { return c.Compact || c.Interrupt || c.Clear }

// PermissionsOffer is what an adapter does with grants (docs/grants.md).
type PermissionsOffer struct {
	// Dir says a directory granted with a task is taken into the running
	// session's writable roots and taken back once the task is reported on.
	Dir bool
	// Git says the Git metadata of the session's own checkout can be opened
	// for a message.
	Git bool
	// ReachesWrapper says the commands a session runs reach its wrapper's
	// socket, so a main there can register a grant: a grant is confirmed
	// only by the wrapper of the main that sent it
	// (docs/grants.md#who-can-grant).
	ReachesWrapper bool
}
