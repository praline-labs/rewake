package cli

import (
	"fmt"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/role"
)

// A harness whose contract reports no turn's end — in stage 3, one that offers
// Launch and Wake only — cannot carry what a turn's end owes: a
// session of a reporting role there would owe reports nobody sends, and a task
// or a question to any session there would wait for an answer that never
// comes. So both are refused before anything is claimed or written, exit 1
// (docs/v2/stage3-steps-adapters.md, S9). S11 decides the same by the live
// TurnBoundary.

// reportsTurns says a harness reports every turn's end. A var, so the tests
// whose sessions stand on a harness of no particular contract can take one
// that reports; the tests of the refusals put it back.
var reportsTurns = harnessReportsTurns

func harnessReportsTurns(h adapter.Harness) bool {
	reporting, ok := h.(adapter.TurnReportingHarness)
	return ok && reporting.ReportsTurns()
}

// refuseReportingRole refuses a launch of a role that owes reports on a
// harness that reports no turn's end.
func refuseReportingRole(h adapter.Harness, part role.Role) error {
	if part.Silent || reportsTurns(h) {
		return nil
	}
	// No role flag is the default role, which reports.
	name := part.ID
	if name == "" {
		name = role.Default().ID
	}
	return &FailedError{Message: fmt.Sprintf(
		"a %s session owes a report at each turn's end, and %s has no turn boundary in this rewake: it is told when a notice arrives, never when a turn ends. The %s role is available on %s from stage 4; start this session with --main, or run the %s role on another harness.",
		name, h.Title(), name, h.Title(), name)}
}

// refuseUnreported refuses a task or a question to a session on a harness
// that reports no turn's end.
func refuseUnreported(session registry.Session, kind inbox.Kind) error {
	if kind != inbox.Task && kind != inbox.Question {
		return nil
	}
	found, _ := findHarness(session.Harness)
	if reportsTurns(found) {
		return nil
	}
	what := "task"
	if kind == inbox.Question {
		what = "question"
	}
	return &FailedError{Message: fmt.Sprintf(
		"%s runs %s, which has no turn boundary in this rewake, so the end of its turn reaches nobody and a %s to it would never be answered; nothing was sent. Send it a note with --notify, or the %s to a session on another harness; %s reports its turns from stage 4.",
		session.Name, harnessTitle(found, session.Harness), what, what, harnessTitle(found, session.Harness))}
}
