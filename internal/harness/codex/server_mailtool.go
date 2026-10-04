package codex

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/praline-labs/rewake/internal/bridge/endpoint"
	"github.com/praline-labs/rewake/internal/channel"
	"github.com/praline-labs/rewake/internal/harness/codex/gateway"
)

// The owned server's side of the mail tool's injection check: once at start,
// on the real server before the terminal connects, and at every thread
// request the gateway passes (mailtool_inject.go).

// checkTool runs the check at start. A refusal refuses the launch, and the
// record goes as after any failed start.
func (s *serverSession) checkTool(ctx context.Context) error {
	if s.tool == nil {
		return nil
	}
	result := s.tool.check(ctx, s.tool.cwd, nil)
	if result.refusal != "" {
		return errors.New("rewake: not starting: the mail tool's entry could not be shown to be rewake's alone: " + result.refusal + "; rewake changes none of your files")
	}
	s.noteReadsOff(result.readsOff)
	return nil
}

// threadCheck is the gateway's check of a thread request, or nil for a run
// without the tool. What it lets through the gateway tells as a selection
// or another thread (selection).
func (s *serverSession) threadCheck(ctx context.Context) func(string, json.RawMessage) string {
	if s.tool == nil {
		return nil
	}
	return func(method string, params json.RawMessage) string {
		result, applies := s.tool.checkThread(ctx, method, params)
		if !applies {
			return ""
		}
		if result.refusal != "" {
			return "rewake: this conversation was not opened: " + result.refusal + ". The session goes on; its other conversations are unaffected."
		}
		s.noteReadsOff(result.readsOff)
		return ""
	}
}

// selectionSteps are the channel's events of the gateway's selection steps.
var selectionSteps = map[string]channel.Kind{
	gateway.SelectionAdmitted: channel.SelectionAdmitted,
	gateway.SelectionSelected: channel.Selected,
	gateway.SelectionFailed:   channel.SelectionFailed,
	gateway.ThreadAdmitted:    channel.ThreadAdmitted,
}

// selection tells the gateway's selection steps to the channel, each
// stamped when the gateway took it; nil for a run without the tool.
func (s *serverSession) selection(tell func(channel.Event)) func(gateway.Selection) {
	if s.tool == nil || tell == nil {
		return nil
	}
	return func(step gateway.Selection) {
		if kind, ok := selectionSteps[step.Step]; ok {
			tell(channel.Event{Kind: kind, Thread: step.Thread, At: endpoint.Stamp()})
		}
	}
}

// noteReadsOff keeps the first reason reads went off: a run whose tool
// could not read whole in one thread reads in the shell in all of them.
func (s *serverSession) noteReadsOff(reason string) {
	if reason != "" {
		s.readsOff.CompareAndSwap(nil, &reason)
	}
}

// ToolReadsOff says why the run's tool may not read, or "".
func (s *serverSession) ToolReadsOff() string {
	if reason := s.readsOff.Load(); reason != nil {
		return *reason
	}
	return ""
}
