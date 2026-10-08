package cli

import (
	"encoding/json"

	"github.com/praline-labs/rewake/internal/inbox"
)

// hookTurnEnd is a turn end decoded from a Claude Code hook's payload: the
// end itself, and what only such a payload says beside it.
type hookTurnEnd struct {
	inbox.TurnEnd
	// Thread is the conversation the turn ended in: the hook's session_id.
	Thread string
	// Holdable says the harness lets this turn end be held and the model
	// asked once more: a Stop hook whose stop_hook_active is false. The call
	// after a hold says true and is never held again, so a turn is held at
	// most once (docs/turn-outcomes.md).
	Holdable bool
}

// completedTurn decodes a Stop or StopFailure hook's payload; anything else
// is no turn end. The hook carries no turn id, so the end has none.
func completedTurn(payload []byte) (hookTurnEnd, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(payload, &fields) != nil {
		return hookTurnEnd{}, false
	}
	text := func(key string) string { var s string; _ = json.Unmarshal(fields[key], &s); return s }
	// Inherited hooks also run in children; their completion cannot settle the parent.
	if text("agent_id") != "" {
		return hookTurnEnd{}, false
	}
	hook := text("hook_event_name")
	if hook != "Stop" && hook != "StopFailure" {
		return hookTurnEnd{}, false
	}
	result := hookTurnEnd{Thread: text("session_id")}
	result.Holdable = hook == "Stop" && string(fields["stop_hook_active"]) == "false"
	result.Text = text("last_assistant_message")
	result.Failed = hook == "StopFailure"
	if result.Failed && result.Text == "" {
		result.Text = text("error_details")
		if result.Text == "" {
			result.Text = text("error")
		}
	}
	return result, true
}
