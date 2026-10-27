package host

import (
	"fmt"
	"os"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
)

// A boundary proven only once the harness runs — one a part loaded inside the
// harness proves by saying hello — cannot be awaited before the start, as a
// boundary proven by the run's own service is (awaitBoundary).
// The launch refuses on a cause it knows the proof will not come, starts the
// harness otherwise, and the run's availability waits for the proof instead
// (docs/claude-turns.md#readiness-and-admission).

// afterStartBound is adapter.AfterStartBound, a variable so a test can
// shorten it.
var afterStartBound = adapter.AfterStartBound

// owesAfterStart says the session owes reports on a boundary its run proves
// only after the start.
func owesAfterStart(session registry.Session, h adapter.Harness) bool {
	offer := h.Offers().TurnBoundary
	return mail.OwesReports(session.Role) && offer.Reports && offer.AfterStart
}

// refuseWithheld refuses, before the harness starts, a launch of a role that
// owes reports whose run says it will not prove its turn boundary, naming the
// cause; a run with no boundary in its plan says so too. A silent role goes
// on, each cause said once on stderr.
func refuseWithheld(session registry.Session, h adapter.Harness, plan adapter.LaunchPlan) error {
	var causes []adapter.Withheld
	if withholder, ok := plan.Service.(adapter.Withholder); ok {
		causes = withholder.Withheld()
	}
	part := role.Of(session.Role)
	for _, withheld := range causes {
		if withheld.Capability == adapter.TurnBoundaryCapability && !part.Silent && h.Offers().TurnBoundary.Reports {
			return fmt.Errorf("a %s session owes a report at each turn's end, and %s; nothing was started", part.ID, withheld.Cause)
		}
	}
	for _, withheld := range causes {
		_, _ = fmt.Fprintln(os.Stderr, "rewake: "+withheld.Cause)
	}
	if owesAfterStart(session, h) && plan.TurnBoundary == nil {
		return fmt.Errorf("a %s session owes a report at each turn's end, and this run of %s has no turn boundary to report them; nothing was started", part.ID, h.Title())
	}
	return nil
}

// availableAfterStart is what the run's availability waits for besides the
// harness itself, asked as the harness starts: for a session that owes
// reports on a boundary proven after the start, the boundary live, or
// AfterStartBound gone since that moment; for any
// other, nothing. Past the bound the run is available with its boundary not
// live, and a task to it is refused until the proof comes.
func availableAfterStart(session registry.Session, h adapter.Harness, live *liveSet) func() bool {
	if !owesAfterStart(session, h) {
		return func() bool { return true }
	}
	started, bound := time.Now(), afterStartBound
	return func() bool {
		return live.has(adapter.TurnBoundaryCapability) || time.Since(started) >= bound
	}
}
