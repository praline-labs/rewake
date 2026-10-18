package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/praline-labs/rewake/internal/core/authority"
	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/person"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// decide settles a report whose outcome is unknown, or lists what can be
// settled (docs/v2/stage3-decision.md#the-command). The command finds the
// subject and who may decide it, then hands the decision to part 2's
// transaction — through the recipient run's wrapper, verified, or in this
// process as the person — and renders what the mailbox holds afterwards. It
// keeps no state machine of its own.

func decideGroup() Group {
	return Group{
		Title:   "DECIDE AN UNKNOWN",
		Summary: "When nobody can tell whether a report reached its recipient, the sender's mailbox stops on it until the recipient run, or the person outside every session, says whether it arrived.",
		Commands: []*Command{{
			Name:           "decide",
			Args:           "<name> [report]",
			MaxPositionals: 2,
			Summary:        "List the reports of a stopped session that a decision can settle, or settle one: it arrived, or it did not and goes again.",
			Options: []Option{
				{Flag: "--arrived", Summary: "The report reached its recipient: the stop it caused is settled and nothing is sent again."},
				{Flag: "--not-arrived", Summary: "It did not: the same report goes again under a new id, and the stop it caused is settled."},
				jsonOption,
			},
			Examples: []string{
				"rewake decide api",
				"rewake decide api --json",
				"rewake decide api 1790370984617481194-8d4ddd85c8f1 --arrived",
				"rewake decide api 1790370984617481194-8d4ddd85c8f1 --not-arrived --json",
			},
			Next: []string{"rewake inbox --awaited"},
			Notes: []string{
				"The name is the stopped session, the report the id its stop note printed. With no report it lists, one line each, the reports that can be decided — the recipient and its run, what each answers, which read failed and why, a decision already made and how far its effects went — then what no decision settles. The list changes nothing.",
				"Decides only the recipient run of the report, from a command or tool call below its own wrapper — its wrapper writes the decision, verified — or the person, from a process outside every session, unverified. A compaction keeps the run; a restart of the name does not. A shell whose sandbox cannot reach the wrapper decides through the rewake tool's decide: a wrapper that cannot be reached is never taken for the person.",
				"A decision is final and never replaced: the same words again say it was recorded before and finish what it fixed; the other switch is refused. Delivery stays unproven either way: a task or question it answers is closed by the decision, never marked delivered.",
				"Exit 0 recorded, now or before, even when the session stays stopped on other reports, which the answer names; 1 the mailbox busy, the tool call's deadline past before the record, the subject changed, the outcome proven since, an opposite decision, something it stands on unreadable, the wrapper unreachable or its answer lost, or an effect that failed after the record (recorded: true; the next barrier does it); 2 a wrong call — no switch or both, a report that is no subject, a caller placed in a session that may not decide.",
			},
			Access:  decideAccess,
			Handler: handleDecide,
		}},
	}
}

// decideAccess classifies decide by its form: a report with exactly one
// choice decides and needs the lease; the list, and any form the handler
// refuses before it reads, write nothing.
func decideAccess(call Call) access {
	if _, _, choice, err := decideWords(call); err == nil && choice != "" {
		return accessChange
	}
	return accessView
}

// decideWords reads a call's session, report and choice; the list form has
// neither of the last two.
func decideWords(call Call) (name, report, choice string, err error) {
	usage := func(message string) error { return &UsageError{Command: call.Command, Message: message} }
	arrived, notArrived := call.Switch("arrived"), call.Switch("not-arrived")
	switch {
	case len(call.Positionals) == 0:
		return "", "", "", usage("decide needs the stopped session's name, and to decide, the report id its stop note printed.")
	case !state.ValidName(call.Positionals[0]):
		return "", "", "", usage(fmt.Sprintf("%q is no session name.", call.Positionals[0]))
	case arrived && notArrived:
		return "", "", "", usage("--arrived and --not-arrived exclude each other: give the one that is true.")
	case len(call.Positionals) == 1 && (arrived || notArrived):
		return "", "", "", usage("a choice needs the report it is about: rewake decide <name> <report> --arrived | --not-arrived.")
	case len(call.Positionals) == 2 && !arrived && !notArrived:
		return "", "", "", usage("deciding needs --arrived or --not-arrived; without the report, decide lists what can be decided.")
	}
	name = call.Positionals[0]
	if len(call.Positionals) == 1 {
		return name, "", "", nil
	}
	choice = mail.ChoiceArrived
	if notArrived {
		choice = mail.ChoiceNotArrived
	}
	return name, call.Positionals[1], choice, nil
}

func handleDecide(ctx *Context, call Call) error {
	name, report, choice, err := decideWords(call)
	if err != nil {
		return err
	}
	dir, err := state.Dir()
	if err != nil {
		return &UsageError{Command: call.Command, Message: err.Error()}
	}
	if choice == "" {
		return listDecisions(ctx, dir, name)
	}
	// Through the tool the decision runs under a receipt, bound to the call
	// before the wrapper is asked: a call whose child dies then says it held
	// an operation, never that nothing ran, and retry asks again.
	if ctx.scope != nil && !ctx.journaling {
		return journaled(ctx, call, handleDecide)
	}
	return decideReport(ctx, call, dir, name, report, choice)
}

// The ways a decision is reached, replaceable in tests.
var (
	askWrapper     = authority.Decide
	classifyPerson = person.Classify
	runsBelow      = belowRun
)

// decideReport makes one decision through whoever may make it and answers
// what the mailbox holds afterwards.
func decideReport(ctx *Context, call Call, dir, name, report, choice string) error {
	subject, err := mail.FindSubject(dir, name, report)
	if err != nil {
		return decisionRefusal(ctx, call, dir, name, report, err)
	}
	ask := mail.DecideRequest{Sender: name, Report: report, Choice: choice, Expect: subject}
	path := mail.DecisionPath(dir, name, report)
	if err := decisionDeadline(ctx); err != nil {
		return err
	}
	// The check above is made before any lock wait; the writer makes it
	// again after both, just before the record, by the call's own bound.
	if ctx.scope != nil {
		ask.DeadlineBoot = ctx.scope.ticket.DeadlineBoot
	}
	var answer mail.DecideAnswer
	if run, ok := authority.RunOf(subject.RecipientRun); ok && runsBelow(run) {
		answer, err = askWrapper(state.AuthorityAddress(dir, subject.RecipientRun), run, ask, path)
		if errors.Is(err, authority.ErrUnreachable) {
			return unfinished(ctx, fmt.Sprintf("Rewake: the wrapper of %s (run %s) could not be reached from this process (%v), so nothing was decided. A sandboxed shell cannot reach it: decide through the rewake tool's decide with the same words. A wrapper out of reach is never taken for the person.",
				subject.Recipient, subject.RecipientRun, err))
		}
	} else {
		root, rootErr := state.RootForRoom(dir)
		if rootErr != nil {
			return failf("Rewake: %v; nothing was decided.", rootErr)
		}
		if err := classifyPerson(root); err != nil {
			return personRefusal(call, subject, err)
		}
		ask.By = mail.DecidedBy{Kind: mail.ByPerson}
		answer, err = mail.Decide(context.Background(), dir, ask)
	}
	if err != nil {
		return decisionRefusal(ctx, call, dir, name, report, err)
	}
	return printDecided(ctx, dir, name, answer, nil)
}

// belowRun says this process runs below the wrapper a run names: that pid of
// this boot, started when the run says, above it in an unbroken chain. A pid
// taken again by another process is not the run. The wrapper checks the same
// from the connection before it decides; this only picks whom to ask.
func belowRun(run authority.Run) bool {
	boot, err := registry.CurrentBoot()
	if err != nil || boot != run.Boot {
		return false
	}
	start, err := proc.StartTime(run.PID)
	if err != nil || start != run.Start {
		return false
	}
	return proc.Default.Descends(os.Getpid(), run.PID) == nil
}

// decisionDeadline refuses to ask once a tool call's answer is no longer
// wanted: the decision would be made where the model never learns of it.
func decisionDeadline(ctx *Context) error {
	if ctx.op == nil {
		return nil
	}
	return beforeCommit(ctx, ctx.op.record.Token, "the decision was asked")
}

// unfinished is a failure after which the same words may still decide:
// through the tool it leaves the operation open for retry.
func unfinished(ctx *Context, message string) error {
	if ctx.op != nil {
		return &unfinishedError{message: message + fmt.Sprintf(" Ask again with: rewake retry %s", ctx.op.record.Token)}
	}
	return &FailedError{Message: message}
}

// personRefusal answers a caller that may not decide as the person: placed
// in a session, a wrong call; its identity unreadable, a refusal.
func personRefusal(call Call, subject mail.Subject, err error) error {
	var below *person.BelowError
	if errors.As(err, &below) {
		where := fmt.Sprintf("process %d above it was started inside a session", below.PID)
		if below.Session != "" {
			where = "this process runs below the wrapper of " + below.Session
		}
		return &UsageError{Command: call.Command, Message: fmt.Sprintf("Rewake: report %s of %s is decided by %s (run %s) or by the person; %s. Nothing was decided.",
			subject.Report, subject.Sender, subject.Recipient, subject.RecipientRun, where)}
	}
	return failf("Rewake: %v. Nothing was decided.", err)
}
