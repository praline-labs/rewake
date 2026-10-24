package cli

import (
	"fmt"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
)

// A harness whose contract reports no turn's end — no harness of a release,
// only a test's — cannot carry what a turn's end owes: a
// session of a reporting role there would owe reports nobody sends, and a task
// or a question to any session there would wait for an answer that never
// comes. So both are refused before anything is claimed or written, exit 1
// (docs/rules/adapter-api.md#the-capabilities). Once a session runs, a task
// or a question to it is decided by its live TurnBoundary.

// reportsTurns says a harness reports every turn's end. A var, so the tests
// whose sessions stand on a harness of no particular contract can take one
// that reports; the tests of the refusals put it back.
var reportsTurns = harnessReportsTurns

func harnessReportsTurns(h adapter.Harness) bool {
	return h != nil && h.Offers().TurnBoundary.Reports
}

// refuseReportingRole refuses a launch of a role that owes reports on a
// harness that reports no turn's end.
func refuseReportingRole(h adapter.Harness, part role.Role) error {
	if part.Silent || reportsTurns(h) {
		return nil
	}
	return &FailedError{Message: fmt.Sprintf(
		"a %s session owes a report at each turn's end, and %s has no turn boundary in this rewake: it is told when a notice arrives, never when a turn ends. Open the %s worker on a harness that reports its turns.",
		part.ID, h.Title(), part.ID)}
}

// refuseUnreported refuses a task or a question to a session whose turn
// boundary is not live: the end of its turn reaches nobody, so nothing would
// answer it. The decision reads the live set alone; what the harness offers
// only picks the words — a harness with no boundary at all, or a run of one
// that has not made its boundary live.
func refuseUnreported(catalog adapter.Catalog, session registry.Session, kind mail.Kind) error {
	if kind != mail.Task && kind != mail.Question {
		return nil
	}
	if session.HasLive(string(adapter.TurnBoundaryCapability)) {
		return nil
	}
	what := "task"
	if kind == mail.Question {
		what = "question"
	}
	found, _ := catalog.Find(session.Harness)
	title := harnessTitle(found, session.Harness)
	if !reportsTurns(found) {
		return &FailedError{Message: fmt.Sprintf(
			"%s runs %s, which has no turn boundary in this rewake, so the end of its turn reaches nobody and a %s to it would never be answered; nothing was sent. Send it a note with --notify, or the %s to a session on a harness that reports its turns.",
			session.Name, title, what, what)}
	}
	return &FailedError{Message: fmt.Sprintf(
		"%s's turn boundary is not live: its run has not proven, or has lost, the report of its turns' ends, so the end of its turn reaches nobody and a %s to it would never be answered; nothing was sent. Send it a note with --notify, or the %s to another session; rewake list --json shows what each run has live.",
		session.Name, what, what)}
}
