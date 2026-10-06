package cli

import "encoding/json"

// hookTurnEnd is a turn end decoded from a hook's or a notify program's
// payload: the end itself, and what only such a payload says beside it.
type hookTurnEnd struct {
	turnResult
	// Thread is the conversation the turn ended in, when the payload names
	// one: a Claude Code hook's session_id.
	Thread string
	// Holdable says the harness lets this turn end be held and the model
	// asked once more: a Claude Code Stop hook whose stop_hook_active is
	// false. The call after a hold says true and is never held again, so a
	// turn is held at most once (docs/turn-outcomes.md).
	Holdable bool
}

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
	event := text("type")
	if event != "" && event != "agent-turn-complete" && event != "task_complete" {
		return hookTurnEnd{}, false
	}
	hook := text("hook_event_name")
	if hook != "" && hook != "Stop" && hook != "StopFailure" {
		return hookTurnEnd{}, false
	}
	result := hookTurnEnd{turnResult: turnResult{ID: text("turn-id")}}
	if hook != "" {
		// Only a hook's payload: Codex's notify has no session_id, and its
		// conversation reaches the report through the gateway instead.
		result.Thread = text("session_id")
	}
	result.Holdable = hook == "Stop" && string(fields["stop_hook_active"]) == "false"
	if result.ID == "" {
		result.ID = text("turn_id")
	}
	if result.ID != "" {
		result.ID = text("thread-id") + "/" + result.ID
	}
	for _, key := range []string{"last_assistant_message", "last-assistant-message", "last_agent_message"} {
		if _, present := fields[key]; present {
			result.Text = text(key)
			break
		}
	}
	raw, hasError := fields["error"]
	result.Failed = hook == "StopFailure" || hasError && string(raw) != "null" && string(raw) != `""`
	if result.Failed {
		if hook == "StopFailure" && result.Text != "" {
			return result, true
		}
		result.Text = text("error_details")
		if result.Text == "" {
			result.Text = text("error")
			if result.Text == "" && hasError && string(raw) != "null" {
				var reason struct {
					Message string `json:"message"`
				}
				if json.Unmarshal(raw, &reason) == nil && reason.Message != "" {
					result.Text = reason.Message
				} else {
					result.Text = string(raw)
				}
			}
		}
	}
	return result, true
}
