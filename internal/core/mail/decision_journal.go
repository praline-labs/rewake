package mail

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// A journal's own part in a decision (docs/v2/stage3-decision-recovery.md):
// it reads the decision on each report before the report's evidence, closes a
// decided report's step, runs its tail, and records the closure before its
// done rewrite; a resend's journal keeps what came of its report in its done
// form.

// decided says the journal closed report's step by a decision.
func (j TurnJournal) decided(report string) bool {
	return slices.ContainsFunc(j.Decided, func(step DecidedStep) bool { return step.Report == report })
}

// decisionOn is the decision on a report of journal id, empty when there is
// none. A decision that does not read is an unknown for the step, never an
// absence: the report is not published on it.
func (w world) decisionOn(name, id string, journal TurnJournal, report Message) (string, error) {
	d, err := w.decisionOf(name, id, journal, report)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return d.ID, err
}

// decisionOf reads the decision on a report of journal id and holds it to
// the step reading it: one about another journal, operation, run, recipient
// or kind decides nothing of this step and is an unknown. None there is
// os.ErrNotExist.
func (w world) decisionOf(name, id string, journal TurnJournal, report Message) (Decision, error) {
	path := DecisionPath(w.dir, name, report.ID)
	raw, err := w.readFile(path)
	if err != nil {
		return Decision{}, err
	}
	d, err := parseDecision(path, raw)
	if err != nil {
		return Decision{}, unknownRecord(path, err)
	}
	s := d.Subject
	if s.Journal != id || s.Op != journal.Op || s.SenderRun != journal.Epoch || s.Recipient != report.To || s.RecipientRun != report.ToEpoch || s.Kind != report.Kind {
		return Decision{}, unknownRecord(path, fmt.Errorf("the decision %s is about report %s of journal %s, operation %q, on run %s, which is not the step of journal %s, operation %q, reading it", path, s.Report, s.Journal, s.Op, s.SenderRun, id, journal.Op))
	}
	return d, nil
}

// decidedBy holds a step the journal saved as decided to the decision it
// names: that decision is there, reads, is the one named and is about this
// step of this journal. A saved step without it is an unknown, never
// progress, and the tail that would clear the report's waits does not run on
// it.
func (w world) decidedBy(name, id string, journal TurnJournal, step DecidedStep) error {
	path := DecisionPath(w.dir, name, step.Report)
	at := slices.IndexFunc(journal.Reports, func(m Message) bool { return m.ID == step.Report })
	if at < 0 {
		return unknownRecord(path, fmt.Errorf("the journal %s says decision %s closed report %s, which it does not carry", id, step.Decision, step.Report))
	}
	d, err := w.decisionOf(name, id, journal, journal.Reports[at])
	if errors.Is(err, os.ErrNotExist) {
		return unknownRecord(path, fmt.Errorf("the journal %s says decision %s closed report %s, and no decision on it is there", id, step.Decision, step.Report))
	}
	if err != nil {
		return err
	}
	if d.ID != step.Decision {
		return unknownRecord(path, fmt.Errorf("the journal %s says decision %s closed report %s, and the decision on it is %s", id, step.Decision, step.Report, d.ID))
	}
	return nil
}

// closeDecided records, for each step of journal id a decision closed, that
// the journal's tail ran: write-once, after the tail and before the done
// rewrite. A crash replays the tail and finds the closure written; one that
// names another decision or journal is an unknown, never written over.
func (w world) closeDecided(name, id string, journal TurnJournal) error {
	for _, step := range journal.Decided {
		path := DecisionPath(w.dir, name, step.Report) + closedSuffix
		raw, err := w.readFile(path)
		switch {
		case err == nil:
			if err := closureOf(path, raw, step, id, journal); err != nil {
				return inOp(step.Report, err)
			}
			continue
		case !errors.Is(err, os.ErrNotExist):
			return inOp(step.Report, err)
		}
		closure := Closure{Decision: step.Decision, Journal: id, Op: journal.Op, Report: step.Report, Waits: closedWaits(journal, step.Report), At: time.Now()}
		raw, err = json.Marshal(closure)
		if err != nil {
			return err
		}
		if err := w.publish(path, raw); err != nil && !errors.Is(err, state.ErrNameTaken) {
			return inOp(step.Report, err)
		}
	}
	return nil
}

// closureOf holds a closure found written to the step it closes: the
// decision, the journal and its operation, and exactly the waits of the
// journal's tail the report answers. One that says other waits records a
// closure this tail did not make.
func closureOf(path string, raw []byte, step DecidedStep, id string, journal TurnJournal) error {
	closure, err := parseClosure(path, raw)
	if err != nil {
		return unknownRecord(path, err)
	}
	if closure.Decision != step.Decision || closure.Journal != id || closure.Op != journal.Op {
		return unknownRecord(path, fmt.Errorf("the closure %s names decision %s of journal %s, operation %q, not %s of %s, operation %q", path, closure.Decision, closure.Journal, closure.Op, step.Decision, id, journal.Op))
	}
	if !sameWaits(closure.Waits, closedWaits(journal, step.Report)) {
		return unknownRecord(path, fmt.Errorf("the closure %s names waits the tail of %s does not clear for its report", path, id))
	}
	return nil
}

// sameWaits says two lists name the same waits, in the same order.
func sameWaits(a, b []Waiter) bool {
	if len(a) == 0 || len(b) == 0 {
		return len(a) == len(b)
	}
	left, err := json.Marshal(a)
	if err != nil {
		return false
	}
	right, err := json.Marshal(b)
	return err == nil && string(left) == string(right)
}

// closedWaits are the waits of the journal's tail the report answers: those
// of its recipient's run.
func closedWaits(journal TurnJournal, report string) []Waiter {
	waits := []Waiter{}
	at := slices.IndexFunc(journal.Reports, func(m Message) bool { return m.ID == report })
	if at < 0 {
		return waits
	}
	to := journal.Reports[at]
	for _, waiter := range journal.Clear {
		if waiter.Name == to.To && waiter.Epoch == to.ToEpoch {
			waits = append(waits, waiter)
		}
	}
	return waits
}

// resendSummary is what the done form of a resend's journal keeps of its one
// report, nil for any other journal: a name proves a resend installed, and
// only this says what came of it.
func resendSummary(id string, journal TurnJournal) *ResendSummary {
	decision, ok := strings.CutPrefix(id, resendPrefix)
	if !ok || len(journal.Reports) != 1 {
		return nil
	}
	report := journal.Reports[0].ID
	summary := &ResendSummary{Report: report, Decision: decision}
	switch {
	case slices.Contains(journal.Published, report):
		summary.Disposition = DispositionPublished
	case slices.Contains(journal.Moot, report):
		summary.Disposition = DispositionMoot
	case journal.decided(report):
		at := slices.IndexFunc(journal.Decided, func(step DecidedStep) bool { return step.Report == report })
		summary.Disposition, summary.Child = DispositionDecided, journal.Decided[at].Decision
	default:
		return nil
	}
	if summary.check(id) != nil {
		return nil
	}
	return summary
}
