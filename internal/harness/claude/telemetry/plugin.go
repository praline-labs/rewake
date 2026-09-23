package telemetry

import "math"

// Events the function-hooks plugin sends (internal/harness/claude/plugin.go).
// The plugin hears what no hook says: the end of a turn a person interrupted,
// which runs no Stop (docs/research-claude-control.md).
const (
	// PluginReady says the plugin loaded in this session. Without it an
	// interruption goes unheard, and the snapshot says so.
	PluginReady = "plugin.ready"
	// TurnStart and TurnComplete bound every turn of the session itself,
	// interrupted or not; a subagent's never reach the wrapper.
	TurnStart    = "turn.start"
	TurnComplete = "turn.complete"
	// SessionMeasure is the context fill the harness measured at a turn's end.
	SessionMeasure = "session.measure"
)

// ReasonAborted is the turn.complete reason of a turn a person interrupted
// with Esc or Ctrl+C. The others — answer, refusal, error — end in a Stop or
// StopFailure hook, which reports them.
const ReasonAborted = "aborted"

// pluginInput names the fields the plugin sends. The plugin itself picks them
// out of the harness's event and never touches the prompt or the answer; this
// decoder names them again so that nothing else is read even if it did.
type pluginInput struct {
	Event   string `json:"plugin_event"`
	Turn    string `json:"turn_id"`
	Reason  string `json:"reason"`
	Context *struct {
		Tokens  *float64 `json:"tokens"`
		Window  *float64 `json:"window"`
		Percent *float64 `json:"percent"`
	} `json:"context"`
}

// DecodePlugin reads what the plugin handed `rewake observe`. It answers false
// for anything that is not one of the plugin's events.
func DecodePlugin(raw []byte) (Event, bool) {
	var input pluginInput
	if !decodeTolerant(raw, &input) {
		return Event{}, false
	}
	event := Event{Kind: input.Event}
	switch input.Event {
	case PluginReady, TurnStart:
		event.Turn = input.Turn
	case TurnComplete:
		if input.Reason == "" {
			return Event{}, false
		}
		event.Turn, event.Reason = input.Turn, input.Reason
	case SessionMeasure:
		if input.Context == nil {
			return Event{}, false
		}
		context := &Context{Used: tokenCount(input.Context.Tokens), Window: tokenCount(input.Context.Window)}
		if context.Window != nil && *context.Window <= 0 {
			context.Window = nil
		}
		if percent := input.Context.Percent; percent != nil && *percent >= 0 && *percent <= 100 {
			rounded := int(math.Round(*percent))
			context.Percent = &rounded
		}
		if context.Used != nil && *context.Used == 0 {
			// As on the status line: zero tokens is a context not measured
			// yet — before the first response or after a compaction.
			context.Used, context.Percent = nil, nil
		}
		if context.Used == nil && context.Window == nil && context.Percent == nil {
			return Event{}, false
		}
		event.Context = context
	default:
		return Event{}, false
	}
	return event, true
}

// tokenCount takes a token count the way JSON carries it, as a number, and drops
// one no count can be.
func tokenCount(value *float64) *int64 {
	if value == nil || *value < 0 || *value > math.MaxInt64/2 || *value != math.Trunc(*value) {
		return nil
	}
	whole := int64(*value)
	return &whole
}
