package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/control"
	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/role"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// steerModel is what rewake compact and rewake interrupt answer.
type steerModel struct {
	Session      string `json:"session"`
	Action       string `json:"action"`
	Focus        string `json:"focus,omitempty"`
	Outcome      string `json:"outcome"`
	Reason       string `json:"reason,omitempty"`
	Detail       string `json:"detail,omitempty"`
	TokensBefore *int64 `json:"tokensBefore,omitempty"`
	TokensAfter  *int64 `json:"tokensAfter,omitempty"`
}

// steerLimits bounds a request. The plugin polls four times a second, so a
// request nobody took within the pickup limit has nobody to take it. A
// compaction is a request to the model and may take a minute; it stays under
// the two minutes an agent's shell call is usually given.
var steerLimits = map[string]control.Limits{
	control.Compact:   {Pickup: 5 * time.Second, Outcome: 90 * time.Second, Poll: 50 * time.Millisecond},
	control.Interrupt: {Pickup: 5 * time.Second, Outcome: 10 * time.Second, Poll: 50 * time.Millisecond},
}

// findHarness looks a harness up; replaceable in tests, where no harness
// without a focus is registered yet.
var findHarness = harness.Find

func handleCompact(ctx *Context, call Call) error   { return steer(ctx, call, control.Compact) }
func handleInterrupt(ctx *Context, call Call) error { return steer(ctx, call, control.Interrupt) }

// steer sends one control request to a session and reports its outcome.
// Everything that makes the call wrong is refused before anything is written:
// who is asking, whom, and what the target's harness can carry out.
func steer(ctx *Context, call Call, action string) error {
	usage := func(format string, args ...any) error {
		return &UsageError{Command: call.Command, Message: fmt.Sprintf(format, args...)}
	}
	target := ""
	if len(call.Positionals) > 0 {
		target = strings.TrimSpace(call.Positionals[0])
	}
	if target == "" {
		return usage("rewake %s needs the session: its address from rewake list.", action)
	}
	focus := ""
	if len(call.Positionals) > 1 {
		if focus = strings.TrimSpace(call.Positionals[1]); focus == "" {
			return usage("the focus is empty; leave it out to compact without one.")
		}
	}
	dir, err := state.Dir()
	if err != nil {
		return usage("%v", err)
	}
	self, _, err := ownRun(dir)
	if err != nil {
		return usage("only a main session may %s another, and this is not one: %v.", action, err)
	}
	if self.Role != role.Main.ID {
		return usage("only a main session may %s another; %s is a %s session. Ask the main to do it.", action, self.Name, self.Role)
	}
	session, err := registry.Lookup(dir, target)
	switch {
	case errors.Is(err, registry.ErrNotFound):
		return unknownSessionFor(call.Command, dir, target)
	case errors.Is(err, registry.ErrUnusableName):
		return usage("%q is not a session address; take one from rewake list.", target)
	case err != nil:
		return failf("could not read the record of %s: %v", target, err)
	}
	if session.Name == self.Name {
		return usage("a session cannot %s itself: its own turn is running this command.", action)
	}
	found, _ := findHarness(session.Harness)
	steerable, ok := found.(harness.Steerable)
	if !ok {
		return usage("%s is a %s session, which does not take rewake %s yet.", session.Name, harnessTitle(found, session.Harness), action)
	}
	if focus != "" && !steerable.CompactFocus() {
		return usage("focus not supported by %s: it has no way to pass a focus for one compaction. Run rewake compact %s without it.", harnessTitle(found, session.Harness), session.Name)
	}

	// An Esc or Ctrl+C on the calling session ends this shell call with
	// SIGTERM, and SIGKILL 1.5 s later (docs/research-claude-control.md); the
	// request is withdrawn on the way out rather than left for a stalled target
	// to carry out later, unseen. SIGKILL cannot be caught.
	asking, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	model := steerModel{Session: session.Name, Action: action, Focus: focus}
	answer, err := control.Ask(asking, registry.ControlFor(dir, session.Name, session.Epoch()),
		control.Request{Action: action, Focus: focus, From: self.Name}, steerLimits[action])
	switch {
	case errors.Is(err, control.ErrNoControl):
		answer = control.Answer{Outcome: control.Refused, Reason: control.NoControl}
	case err != nil:
		return failf("could not ask %s: %v", session.Name, err)
	}
	model.Outcome, model.Reason, model.Detail = answer.Outcome, answer.Reason, answer.Detail
	model.TokensBefore, model.TokensAfter = answer.TokensBefore, answer.TokensAfter
	line := steerLine(model)
	if model.Outcome == control.Done {
		return printValue(ctx, model, func() []string { return []string{line} })
	}
	if ctx.JSON {
		_ = printValue(ctx, model, func() []string { return nil })
		return &FailedError{Message: ""}
	}
	return &FailedError{Message: line}
}

// steerLine is the line a caller reads: the outcome, and when it is not done,
// what to do next.
func steerLine(model steerModel) string {
	what := map[string]string{control.Compact: "the compaction", control.Interrupt: "the interrupt"}[model.Action]
	detail := ""
	if model.Detail != "" {
		detail = " (" + model.Detail + ")"
	}
	switch model.Outcome {
	case control.Done:
		if model.Action == control.Interrupt {
			return fmt.Sprintf("Rewake: interrupted the turn of %s; whoever waits on it reads stopped, and its next notice says you interrupted it.", model.Session)
		}
		line := "Rewake: compacted " + model.Session
		if model.TokensBefore != nil && model.TokensAfter != nil {
			line += fmt.Sprintf(": %d tokens before, %d after", *model.TokensBefore, *model.TokensAfter)
		}
		return line + "."
	case control.Refused:
		return fmt.Sprintf("Rewake: %s refused %s: %s%s. %s", model.Session, what, model.Reason, detail, steerNext[model.Reason])
	}
	return fmt.Sprintf("Rewake: %s failed on %s%s.", what, model.Session, detail)
}

// steerNext is the next action after each refusal.
var steerNext = map[string]string{
	control.InTurn:           "A compaction never waits for the turn to end: ask again once rewake list shows the session idle, or interrupt it first.",
	control.NoTurn:           "There is nothing to interrupt: rewake list shows the session idle.",
	control.CompactionOff:    "The session runs with compaction switched off; rewake cannot change that.",
	control.NothingToCompact: "The conversation is too short to compact; nothing was done.",
	control.NotAnswering:     "Its rewake plugin is not loaded or not answering — --bare, function hooks switched off, another harness version; such a session shows interruptions unobserved in rewake list.",
	control.CutShort:         "This call was interrupted before the session took the request, which is withdrawn; nothing was done: ask again.",
	control.NoControl:        "It was started by an earlier rewake, which takes no requests, or its wrapper could not make the directory: restart that session with the current rewake.",
	control.Withdrawn:        "The session took it in the instant this command gave up waiting, and did nothing: ask again.",
	control.Busy:             "Wait for that request to end, then ask again.",
}

func harnessTitle(found harness.Harness, id string) string {
	if found == nil {
		return id
	}
	return found.Title()
}
