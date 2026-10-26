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

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/control"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// answerModel is what rewake answer answers.
type answerModel struct {
	Session string   `json:"session"`
	Request string   `json:"request"`
	Choices []string `json:"choices"`
	Outcome string   `json:"outcome"`
	Reason  string   `json:"reason,omitempty"`
	Detail  string   `json:"detail,omitempty"`
}

// answerLimits bounds an answer: the worker's wrapper takes it as it takes a
// compaction, then waits up to fifteen seconds for its mod to settle the
// request with it.
var answerLimits = control.Limits{Pickup: steerPickup, Outcome: 20 * time.Second, Poll: 50 * time.Millisecond}

// handleAnswer gives a worker's open permission request or question main's
// answer (docs/permission-requests.md). Only a session of role main in the
// room answers, checked as rewake compact checks it: a process that takes on
// main's name and run in its environment could answer too, a limit accepted
// on October 18, 2026.
func handleAnswer(ctx *Context, call Call) error {
	usage := func(format string, args ...any) error {
		return &UsageError{Command: call.Command, Message: fmt.Sprintf(format, args...)}
	}
	if len(call.Positionals) < 3 {
		return usage("rewake answer needs the worker, the request and a choice, as the worker's notify names them.")
	}
	target, id := strings.TrimSpace(call.Positionals[0]), strings.TrimSpace(call.Positionals[1])
	choices := call.Positionals[2:]
	dir, err := state.Dir()
	if err != nil {
		return usage("%v", err)
	}
	self, _, err := ownRun(dir)
	if err != nil {
		return usage("only a main session answers a worker's request, and this is not one: %v. Run it from the main session's shell.", err)
	}
	if self.Role != role.Main.ID {
		return usage("only a main session answers a worker's request; %s is a %s session.", self.Name, self.Role)
	}
	session, err := registry.Lookup(dir, target)
	switch {
	case errors.Is(err, registry.ErrNotFound):
		return unknownSessionFor(call.Command, dir, target)
	case errors.Is(err, registry.ErrUnusableName):
		return usage("%q is not a session address; take one from the worker's notify.", target)
	case err != nil:
		return failf("could not read the record of %s: %v", target, err)
	}
	if session.Name == self.Name {
		return usage("main's own requests are answered at its screen.")
	}
	if !session.HasLive(string(adapter.ControlCapability)) {
		return failf("%s's control is not live: its run has not proven, or has lost, the way to carry an answer into it; ask the person to answer it in that worker's pane, which main's shell shows, or let the request expire.", session.Name)
	}
	asking, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	request := control.Request{ID: control.NewID(), Action: control.Reply, From: self.Name, Ask: id, Choices: choices}
	answer, err := control.Ask(asking, registry.ControlFor(dir, session.Name, session.Epoch()), request, answerLimits)
	switch {
	case errors.Is(err, control.ErrNoControl):
		answer = control.Answer{Outcome: control.Refused, Reason: control.NoControl}
	case err != nil:
		return failf("could not answer %s: %v", session.Name, err)
	}
	model := answerModel{Session: session.Name, Request: id, Choices: choices, Outcome: answer.Outcome, Reason: answer.Reason, Detail: answer.Detail}
	line := answerLine(model)
	if model.Outcome == control.Done {
		return printValue(ctx, model, func() []string { return []string{line} })
	}
	if ctx.JSON {
		_ = printValue(ctx, model, func() []string { return nil })
		return &FailedError{Message: ""}
	}
	return &FailedError{Message: line}
}

// answerLine is the line a caller reads.
func answerLine(model answerModel) string {
	detail := ""
	if model.Detail != "" {
		detail = " (" + model.Detail + ")"
	}
	switch {
	case model.Outcome == control.Done:
		return fmt.Sprintf("Rewake: %s took your answer to request %s.", model.Session, model.Request)
	case model.Reason == control.NotAChoice:
		return fmt.Sprintf("Rewake: not an answer to request %s of %s%s; its notify lists the choices.", model.Request, model.Session, detail)
	case model.Outcome == control.Failed:
		return fmt.Sprintf("Rewake: %s did not confirm your answer to request %s%s; rewake inbox --awaited or its screen shows what it does next.", model.Session, model.Request, detail)
	}
	reason := model.Reason
	if reason == "" {
		reason = model.Outcome
	}
	return fmt.Sprintf("Rewake: %s did not take your answer to request %s: %s%s.", model.Session, model.Request, reason, detail)
}
