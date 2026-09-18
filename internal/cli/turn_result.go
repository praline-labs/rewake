package cli

import (
	"encoding/json"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
)

type turnResult struct {
	Boundary *inbox.ReadBoundary
	Text     string
	Failed   bool
	Stopped  bool
	ID       string
}

func completedTurn(payload []byte) (turnResult, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(payload, &fields) != nil {
		return turnResult{}, false
	}
	text := func(key string) string { var s string; _ = json.Unmarshal(fields[key], &s); return s }
	// Inherited hooks also run in children; their completion cannot settle the parent.
	if text("agent_id") != "" {
		return turnResult{}, false
	}
	event := text("type")
	if event != "" && event != "agent-turn-complete" && event != "task_complete" {
		return turnResult{}, false
	}
	hook := text("hook_event_name")
	if hook != "" && hook != "Stop" && hook != "StopFailure" {
		return turnResult{}, false
	}
	result := turnResult{ID: text("turn-id")}
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
