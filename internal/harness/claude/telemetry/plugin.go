package telemetry

import (
	"math"
	"unicode/utf8"

	"github.com/iiiokojiadbi/rewake/internal/control"
	statedir "github.com/iiiokojiadbi/rewake/internal/state"
)

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
	// CompactAsked says a main's `rewake compact` is about to compact the
	// session, with the request and who asked. The module waits for this
	// report to be sent before it asks the host to compact, and datagrams
	// reach the collector in the order they were sent, so it arrives ahead of
	// every hook of that compaction (docs/remote-control.md).
	CompactAsked = "compact.asked"
	// CompactRefused says the host refused that compaction, or it failed: no
	// PostCompact of it follows. Started says whether the host had begun it,
	// so that a PreCompact of it may have run.
	CompactRefused = "compact.refused"
	// CompactEnded is how a compaction a main asked for ended: its end after
	// the command was answered started or requested, or the final answer
	// itself — a refusal, a failure — for a command whose wait ended first.
	// main's wrapper sends it to that main as a letter while main's record of
	// the request is open (docs/remote-control-letter.md).
	CompactEnded = "compact.ended"
)

// maxDetail bounds the host's error text an outcome carries, so the event
// fits one datagram.
const maxDetail = 400

// ReasonAborted is the turn.complete reason of a turn a person interrupted
// with Esc or Ctrl+C. The others — answer, refusal, error — end in a Stop or
// StopFailure hook, which reports them.
const ReasonAborted = "aborted"

// pluginInput names the fields the plugin sends. The plugin itself picks them
// out of the harness's event and never touches the prompt or the answer; this
// decoder names them again so that nothing else is read even if it did.
type pluginInput struct {
	Event  string `json:"plugin_event"`
	Turn   string `json:"turn_id"`
	Reason string `json:"reason"`
	// By names the session whose `rewake interrupt` aborted the turn, empty
	// for a turn a person stopped; or the one whose `rewake compact` asked for
	// a compaction.
	By string `json:"by"`
	// Request is that compaction's control request id.
	Request string `json:"request"`
	// Started is set on a refusal the host gave after PreCompact.
	Started bool `json:"started"`
	// Outcome, Detail and the tokens end a compaction, or give the final
	// answer to its request; Reason is then the refusal's.
	Outcome      string   `json:"outcome"`
	Detail       string   `json:"detail"`
	TokensBefore *float64 `json:"tokensBefore"`
	TokensAfter  *float64 `json:"tokensAfter"`
	Context      *struct {
		Tokens  *float64 `json:"tokens"`
		Window  *float64 `json:"window"`
		Percent *float64 `json:"percent"`
	} `json:"context"`
	// Limit is what the plugin read of the auto-compact window: the variable
	// as the harness's environment holds it, the settings key as the
	// harness's settings hold it.
	Limit *struct {
		Env      string   `json:"env"`
		Settings *float64 `json:"settings"`
	} `json:"limit"`
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
		if input.Reason == ReasonAborted && statedir.ValidName(input.By) {
			event.By = input.By
		}
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
	case CompactAsked, CompactRefused:
		if !control.ValidID.MatchString(input.Request) {
			return Event{}, false
		}
		event.Request = input.Request
		event.Started = input.Event == CompactRefused && input.Started
		if input.Event == CompactAsked {
			if !statedir.ValidName(input.By) {
				return Event{}, false
			}
			event.By = input.By
		}
	case CompactEnded:
		if !control.ValidID.MatchString(input.Request) || !statedir.ValidName(input.By) {
			return Event{}, false
		}
		switch input.Outcome {
		case control.Done, control.Refused, control.Failed:
		default:
			return Event{}, false
		}
		event.Request, event.By, event.Outcome = input.Request, input.By, input.Outcome
		event.Reason, event.Detail = bounded(input.Reason), bounded(input.Detail)
		event.TokensBefore, event.TokensAfter = tokenCount(input.TokensBefore), tokenCount(input.TokensAfter)
	default:
		return Event{}, false
	}
	if input.Limit != nil && (event.Kind == PluginReady || event.Kind == SessionMeasure) {
		event.Limit = &Limit{Env: envWindow(input.Limit.Env), Settings: settingsWindow(input.Limit.Settings)}
	}
	return event, true
}

// bounded cuts a text to maxDetail bytes, on a rune boundary.
func bounded(text string) string {
	if len(text) <= maxDetail {
		return text
	}
	cut := maxDetail
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + "…"
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
