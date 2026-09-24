// Package telemetry carries what a Claude Code session says about itself to the
// wrapper that launched it.
//
// Claude Code has no channel a wrapper can listen on, but it runs commands we
// name: hooks at points of a session's life, and the status line command on
// every change of model, effort or context. Both hand their command a JSON
// object on stdin. A short-lived rewake process reads that object, keeps the
// few fields the collector needs, and sends them to the wrapper in one
// datagram; the wrapper folds them into a snapshot and publishes it with its
// own heartbeat.
//
// The senders run inside the harness's machinery, one of them in front of every
// prompt, so they never wait: no answer is read, nothing is retried, and a
// wrapper that is gone or busy costs them nothing but a failed send.
//
// What arrives on stdin can carry conversation text — the prompt, the last
// reply, a compaction summary. The decoders name the fields they want and
// nothing else, so that text is never kept, sent or used.
package telemetry

import (
	"encoding/json"
	"errors"
)

// Event names sent by hooks, plus the one the status-line tap sends.
const (
	SessionStart     = "SessionStart"
	UserPromptSubmit = "UserPromptSubmit"
	Stop             = "Stop"
	StopFailure      = "StopFailure"
	PreCompact       = "PreCompact"
	PostCompact      = "PostCompact"
	Notification     = "Notification"
	SessionEnd       = "SessionEnd"
	StatusLine       = "StatusLine"
)

// HookEvents are the hooks the adapter registers for telemetry, in the order a
// session meets them.
var HookEvents = []string{SessionStart, UserPromptSubmit, PreCompact, PostCompact, Notification, Stop, StopFailure, SessionEnd}

// maxDatagram bounds one event on the wire. An event is a handful of short
// fields; one that does not fit is not one the collector should read.
const maxDatagram = 2048

// Event is one observation, as it travels from a sender to the wrapper. Every
// field but the kind is optional, and absent means "this sender did not say",
// never zero.
type Event struct {
	// At is when the sending process started, on the machine's boot clock
	// (clock.go). Hooks run in the background, so two of them can arrive out
	// of order; the collector compares these to keep an older event from
	// overwriting a newer one. Only the collector compares them, and only
	// with each other.
	At      int64    `json:"at"`
	Kind    string   `json:"kind"`
	Session string   `json:"session,omitempty"`
	Source  string   `json:"source,omitempty"`
	Trigger string   `json:"trigger,omitempty"`
	Notice  string   `json:"notice,omitempty"`
	Model   string   `json:"model,omitempty"`
	Effort  string   `json:"effort,omitempty"`
	Context *Context `json:"context,omitempty"`
	// Turn and Reason come from the plugin: the harness's id of a turn, and
	// how turn.complete says it ended.
	Turn   string `json:"turn,omitempty"`
	Reason string `json:"reason,omitempty"`
	// By is the session that interrupted the turn with `rewake interrupt`,
	// or that asked for the compaction with `rewake compact`.
	By string `json:"by,omitempty"`
	// Request is the id of that compaction's control request.
	Request string `json:"request,omitempty"`
	// Limit is the auto-compact window the plugin found configured
	// (window.go); nil when it did not say.
	Limit *Limit `json:"limit,omitempty"`
}

// Context is the fill of the context window as the status line reports it.
// Used and Percent are nil before the first response and after a compaction:
// the harness itself does not know them then.
type Context struct {
	Used    *int64 `json:"used,omitempty"`
	Window  *int64 `json:"window,omitempty"`
	Percent *int   `json:"percent,omitempty"`
}

func (e Event) encode() ([]byte, error) {
	raw, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	if len(raw) > maxDatagram {
		return nil, errors.New("event too large")
	}
	return raw, nil
}

func decodeEvent(raw []byte) (Event, bool) {
	var event Event
	if len(raw) > maxDatagram || json.Unmarshal(raw, &event) != nil || event.Kind == "" {
		return Event{}, false
	}
	return event, true
}
