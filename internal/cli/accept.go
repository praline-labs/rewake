package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/control"
	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// acceptGroup is the one command a person runs rather than an agent: when a
// Codex launch that resumed ends up in another conversation, deliveries into
// it wait for the person's word (docs/delivery-conversation.md).
func acceptGroup() Group {
	return Group{
		Title:   "ACCEPT A CONVERSATION",
		Summary: "The person only, from a shell outside any session.",
		Commands: []*Command{{
			Name:           "accept",
			Args:           "<name> <conversation>",
			MaxPositionals: 2,
			Summary:        "Let deliveries go into the conversation a session's terminal selected, when its launch asked to resume another.",
			Options:        []Option{jsonOption},
			Examples:       []string{"rewake accept worker-codex 01a0ee8e-873a-7813-be31-cc02d0e1b4a7"},
			Next:           []string{"rewake list"},
			Notes: []string{
				"A Codex session launched with resume keeps its deliveries pending until the conversation it asked for is resumed; if the terminal went on in another one — its resume refused, say, because another program holds the conversation — the senders read pending, main is told, and the terminal shows a warning naming this command. Resuming the intended conversation with /resume lets them go too; /new alone does not.",
				"The conversation must be the one the terminal has selected now, in the session's current run: an acceptance of one seen earlier is refused. Mail waiting for the session is read in the accepted conversation, not before.",
				"Refused inside a session: the person accepts, not an agent.",
				"Exit 0 accepted; 1 refused (nothing held, not the selected conversation, no conversation selected, not answering, no control directory); 2 a wrong call — inside a session, no such session, a harness that holds nothing.",
			},
			Handler: handleAccept,
		}},
	}
}

// acceptModel is what rewake accept answers.
type acceptModel struct {
	Session      string `json:"session"`
	Conversation string `json:"conversation"`
	Outcome      string `json:"outcome"`
	Reason       string `json:"reason,omitempty"`
	Detail       string `json:"detail,omitempty"`
}

func handleAccept(ctx *Context, call Call) error {
	usage := func(format string, args ...any) error {
		return &UsageError{Command: call.Command, Message: fmt.Sprintf(format, args...)}
	}
	if len(call.Positionals) < 2 {
		return usage("rewake accept needs the session and the conversation, as the warning or rewake list --json names them.")
	}
	target, conversation := strings.TrimSpace(call.Positionals[0]), strings.TrimSpace(call.Positionals[1])
	if target == "" || conversation == "" {
		return usage("rewake accept needs the session and the conversation, as the warning or rewake list --json names them.")
	}
	if name := os.Getenv(state.SessionEnv); name != "" {
		return usage("the person accepts a conversation, not a session: this shell belongs to %s. Run it from a terminal outside any rewake session.", name)
	}
	dir, err := state.Dir()
	if err != nil {
		return usage("%v", err)
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
	found, _ := findHarness(session.Harness)
	if accepting, ok := found.(harness.Accepting); !ok || !accepting.AcceptsConversation() {
		return usage("%s is a %s session, which holds no deliveries for its conversation.", session.Name, harnessTitle(found, session.Harness))
	}
	asking, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	// The control directory is the current run's: an acceptance cannot reach
	// a run that ended, and the wrapper checks the conversation is the one
	// selected now.
	request := control.Request{ID: control.NewID(), Action: control.Accept, Conversation: conversation}
	answer, err := control.Ask(asking, registry.ControlFor(dir, session.Name, session.Epoch()), request, steerLimits[control.Accept])
	switch {
	case errors.Is(err, control.ErrNoControl):
		answer = control.Answer{Outcome: control.Refused, Reason: control.NoControl}
	case err != nil:
		return failf("could not ask %s: %v", session.Name, err)
	}
	model := acceptModel{Session: session.Name, Conversation: conversation, Outcome: answer.Outcome, Reason: answer.Reason, Detail: answer.Detail}
	line := acceptLine(model)
	if model.Outcome == control.Done {
		return printValue(ctx, model, func() []string { return []string{line} })
	}
	if ctx.JSON {
		_ = printValue(ctx, model, func() []string { return nil })
		return &FailedError{Message: ""}
	}
	return &FailedError{Message: line}
}

func acceptLine(model acceptModel) string {
	detail := ""
	if model.Detail != "" {
		detail = " (" + model.Detail + ")"
	}
	switch {
	case model.Outcome == control.Done:
		return fmt.Sprintf("Rewake: %s takes deliveries in %s now; the mail waiting for it goes there.", model.Session, model.Conversation)
	case model.Reason == control.NothingHeld:
		return fmt.Sprintf("Rewake: nothing to accept: %s holds no deliveries for its launch's conversation%s.", model.Session, detail)
	case model.Reason == control.NotSelected:
		return fmt.Sprintf("Rewake: not accepted: %s is not the conversation %s has selected%s; accept the one selected now, as rewake list --json names it.", model.Conversation, model.Session, detail)
	case model.Reason == control.NoSelection:
		return fmt.Sprintf("Rewake: not accepted: %s has no conversation selected for delivery%s; select one in its terminal first.", model.Session, detail)
	case model.Outcome == control.Refused:
		return fmt.Sprintf("Rewake: not accepted: %s is %s%s.", model.Session, model.Reason, detail)
	}
	return fmt.Sprintf("Rewake: the acceptance of %s failed%s.", model.Session, detail)
}
