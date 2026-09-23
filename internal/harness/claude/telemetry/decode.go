package telemetry

import (
	"encoding/json"
	"errors"
)

// hookInput names the hook fields the collector uses. Nothing else of the
// payload is kept: the decoder skips every other key, so the prompt, the last
// reply and a compaction summary pass through it without being stored.
//
// model is a string here: SessionStart hands the model id alone, while the
// status line hands an object.
type hookInput struct {
	Event   string `json:"hook_event_name"`
	Session string `json:"session_id"`
	Agent   string `json:"agent_id"`
	Source  string `json:"source"`
	Trigger string `json:"trigger"`
	Notice  string `json:"notification_type"`
	Model   string `json:"model"`
	Effort  *struct {
		Level string `json:"level"`
	} `json:"effort"`
}

// statusInput names the status-line fields the collector uses.
type statusInput struct {
	Session string `json:"session_id"`
	Model   *struct {
		ID string `json:"id"`
	} `json:"model"`
	Effort *struct {
		Level string `json:"level"`
	} `json:"effort"`
	Context *struct {
		Used    *int64 `json:"total_input_tokens"`
		Window  *int64 `json:"context_window_size"`
		Percent *int   `json:"used_percentage"`
		// Current is null until a response has been counted; only its
		// presence is read, never its contents beyond that.
		Current *struct {
			Input *int64 `json:"input_tokens"`
		} `json:"current_usage"`
	} `json:"context_window"`
}

// decodeTolerant decodes what it can. A field whose type changed in a new
// harness version is skipped rather than costing every other field, which is
// what encoding/json already does as long as the error is only a type mismatch.
func decodeTolerant(raw []byte, into any) bool {
	err := json.Unmarshal(raw, into)
	var mismatch *json.UnmarshalTypeError
	return err == nil || errors.As(err, &mismatch)
}

// DecodeHook reads a hook's stdin. It answers false for a payload that is not
// one, and for a hook fired inside a subagent, which carries an agent id and
// says nothing about the session itself.
func DecodeHook(raw []byte) (Event, bool) {
	var input hookInput
	if !decodeTolerant(raw, &input) || input.Event == "" || input.Agent != "" {
		return Event{}, false
	}
	event := Event{
		Kind:    input.Event,
		Session: input.Session,
		Source:  input.Source,
		Trigger: input.Trigger,
		Notice:  input.Notice,
		Model:   input.Model,
	}
	if input.Effort != nil {
		event.Effort = input.Effort.Level
	}
	return event, true
}

// DecodeStatus reads the status line's stdin.
//
// A missing effort is sent as missing, and the collector reads that from the
// status line as "this model takes none": the harness leaves the key out for
// such a model rather than sending an empty one.
func DecodeStatus(raw []byte) (Event, bool) {
	var input statusInput
	if !decodeTolerant(raw, &input) {
		return Event{}, false
	}
	event := Event{Kind: StatusLine, Session: input.Session}
	if input.Model != nil {
		event.Model = input.Model.ID
	}
	if input.Effort != nil {
		event.Effort = input.Effort.Level
	}
	if window := input.Context; window != nil {
		context := &Context{}
		if window.Window != nil && *window.Window > 0 {
			context.Window = window.Window
		}
		// Before the first response the counts read 0 and the usage null; a
		// zero there is "not measured yet", not an empty context.
		if window.Current != nil {
			context.Used = window.Used
			context.Percent = window.Percent
		}
		event.Context = context
	}
	return event, true
}
