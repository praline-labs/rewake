package mail

import (
	"path/filepath"
	"slices"
)

// What a decision says of the messages its report answers, for the views of
// the run that waits for the report and of the stopped run that owes it
// (docs/v2/stage3-decision.md#the-records, what every reader consults): a
// report decided durably whose tail has not run leaves the wait owed, and
// says so;
// one whose tail ran is closed by the decision, delivery unproven; a resend
// is sent again by a decision. Like Decisions it reads without a lock and
// changes nothing; a record that does not read gives no mark, since it
// settles and closes nothing.

// DecisionMark is one decision on a report, as a view of a message names it.
type DecisionMark struct {
	Decision string
	Report   string
	Choice   string
	By       DecidedBy
	// Closed says the tail of the report's operation closed the wait by this
	// decision: no report is proven to have reached the waiter.
	Closed bool
	// Resent is the report sent again for "did not arrive", and Resend how
	// far it went.
	Resent string
	Resend ResendView
}

// DecisionOn is the decision on a report from sender's mailbox that answers
// message id of the waiting run (name, epoch), if there is one.
func DecisionOn(dir, sender, id, name, epoch string) (DecisionMark, bool) {
	views, err := Decisions(dir, sender)
	if err != nil {
		return DecisionMark{}, false
	}
	for _, view := range views {
		d := view.Decision
		if d == nil || d.Subject.Recipient != name || d.Subject.RecipientRun != epoch {
			continue
		}
		if view.Closure != nil {
			for _, wait := range view.Closure.Waits {
				if wait.Name == name && wait.Epoch == epoch && slices.Contains(wait.Messages, id) {
					return markOf(view, true), true
				}
			}
			continue
		}
		// A record not yet made durable may not outlive a crash, so it
		// says nothing yet of what it answers.
		durable := live(dir).durable(DecisionPath(dir, sender, view.Report), *d)
		if durable && slices.Contains(reportAnswers(dir, sender, *d), id) {
			return markOf(view, false), true
		}
	}
	return DecisionMark{}, false
}

// ResentBy is the decision whose resend is message id from sender, if any.
func ResentBy(dir, sender, id string) (DecisionMark, bool) {
	if id == "" {
		return DecisionMark{}, false
	}
	views, err := Decisions(dir, sender)
	if err != nil {
		return DecisionMark{}, false
	}
	for _, view := range views {
		if view.Decision != nil && view.Decision.Resent() == id {
			return markOf(view, view.Closure != nil), true
		}
	}
	return DecisionMark{}, false
}

func markOf(view DecisionView, closed bool) DecisionMark {
	d := view.Decision
	return DecisionMark{Decision: d.ID, Report: d.Subject.Report, Choice: d.Choice, By: d.By, Closed: closed, Resent: d.Resent(), Resend: view.Resend}
}

// reportAnswers is what a decided report answers: from the resend the
// decision keeps whole, or, for "arrived", from the journal that carries the
// report, in either form, while it is kept.
func reportAnswers(dir, sender string, d Decision) []string {
	if answers := answersOf(d); answers != nil {
		return answers
	}
	path := filepath.Join(JournalPath(dir, sender), d.Subject.Journal)
	for _, candidate := range []string{path, path + doneSuffix} {
		journal, err := readJournalFile(candidate)
		if err != nil {
			continue
		}
		for _, report := range journal.Reports {
			if report.ID == d.Subject.Report {
				return report.InReplyTo
			}
		}
	}
	return nil
}
