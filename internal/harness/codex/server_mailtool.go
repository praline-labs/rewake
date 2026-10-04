package codex

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/praline-labs/rewake/internal/bridge/endpoint"
	"github.com/praline-labs/rewake/internal/channel"
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
// without the tool. A thread it admits starts the tool's servers, so it
// opens the channel's hello timer.
func (s *serverSession) threadCheck(ctx context.Context, tell func(channel.Event)) func(string, json.RawMessage) string {
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
		if tell != nil {
			tell(channel.Event{Kind: channel.ThreadAdmitted, At: endpoint.Stamp()})
		}
		return ""
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
