package mail

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// What `decide <name>` lists (docs/v2/stage3-decision.md#the-command): the
// subjects of a mailbox — the reports an open stop is about that a decision
// can settle, and those decided already, a decision outliving its journal —
// and the occurrences nothing can decide, each with why. It reads only the
// sender's mailbox, with no lock, lease or effect, as the decisions' own view
// does (Q10 of docs/v2/stage3-decision-build.md): a record that does not read
// is listed as undecidable, never skipped, and hides no subject that reads.

// DecisionListing is one mailbox's subjects and undecidable occurrences.
type DecisionListing struct {
	Subjects    []SubjectView
	Undecidable []Undecidable
}

// SubjectView is one subject: open, with the occurrences about it, or decided.
type SubjectView struct {
	Subject Subject
	// Answers are the messages the report answers. AnswersUnknown says the
	// records that would name them are gone or do not read, which is not a
	// report known to answer nothing.
	Answers        []string
	AnswersUnknown bool
	// Causes say, per open occurrence about the report, which path did not
	// read and why.
	Causes []string
	// Decided is the decision on the report and its effects so far; nil for
	// an open subject.
	Decided *DecisionView
	// Durable says the decision may act: false while its record is not yet
	// made durable, when it disposes nothing yet.
	Durable bool
}

// Undecidable is a cause no decision can settle, or a record that does not
// read: the path to repair and why.
type Undecidable struct {
	Path, Why string
}

// ListDecisions reads what `decide <name>` lists.
func ListDecisions(dir, name string) (DecisionListing, error) {
	w := live(dir)
	var listing DecisionListing
	decided := map[string]*SubjectView{}
	views, err := Decisions(dir, name)
	if err != nil {
		listing.Undecidable = append(listing.Undecidable, Undecidable{Path: decisionsRoot(dir, name), Why: err.Error()})
	}
	for _, view := range views {
		path := DecisionPath(dir, name, view.Report)
		if view.Decision == nil {
			decided[view.Report] = &SubjectView{}
			listing.Undecidable = append(listing.Undecidable, Undecidable{Path: path, Why: fmt.Sprintf("the decision does not read, so it settles nothing and nothing can replace it: %v", view.Err)})
			continue
		}
		subject := SubjectView{Subject: view.Decision.Subject, Decided: &view, Durable: w.durable(path, *view.Decision)}
		subject.Answers, subject.AnswersUnknown = w.answersOf(name, view)
		listing.Subjects = append(listing.Subjects, subject)
		decided[view.Report] = &subject
	}
	open, _, err := w.openOccurrences(name)
	if err != nil {
		listing.Undecidable = append(listing.Undecidable, Undecidable{Path: stopsPath(dir, name), Why: err.Error()})
	}
	// What a decision settles is what the stop's one reader takes as settled
	// (stopState): the exact occurrences a durable decision names. Any other
	// occurrence still stops the mailbox, whatever report it is about.
	disposed, err := w.disposals(name)
	if err != nil {
		listing.Undecidable = append(listing.Undecidable, Undecidable{Path: decisionsRoot(dir, name), Why: err.Error()})
	}
	byReport := map[string][]openStop{}
	var reports []string
	for _, stop := range open {
		report := stop.reportOf()
		switch {
		case stop.disposedBy(disposed):
		case stop.unreadable != nil:
			listing.Undecidable = append(listing.Undecidable, Undecidable{Path: stop.path(dir, name), Why: stop.describe(dir, name)})
		case report == "":
			listing.Undecidable = append(listing.Undecidable, Undecidable{Path: stop.path(dir, name), Why: "about no one report, so no decision settles it: " + stop.describe(dir, name)})
		case decided[report] != nil:
			listing.Undecidable = append(listing.Undecidable, Undecidable{Path: stop.path(dir, name), Why: undisposedWhy(dir, name, stop, *decided[report])})
		default:
			if _, seen := byReport[report]; !seen {
				reports = append(reports, report)
			}
			byReport[report] = append(byReport[report], stop)
		}
	}
	for _, report := range reports {
		plan, err := w.subjectOf(name, report)
		if err != nil {
			path, why := stopsPath(dir, name), err
			var refusal *DecisionError
			if errors.As(err, &refusal) {
				why = refusal.Err
				if refusal.Path != "" {
					path = refusal.Path
				}
			}
			listing.Undecidable = append(listing.Undecidable, Undecidable{Path: path, Why: fmt.Sprintf("report %s is no subject: %v", report, why)})
			continue
		}
		view := SubjectView{Subject: plan.subject, Answers: plan.report.InReplyTo}
		for _, stop := range byReport[report] {
			view.Causes = append(view.Causes, stop.describe(dir, name))
		}
		listing.Subjects = append(listing.Subjects, view)
	}
	sort.SliceStable(listing.Subjects, func(i, j int) bool { return listing.Subjects[i].Subject.Report < listing.Subjects[j].Subject.Report })
	return listing, nil
}

// undisposedWhy says why an occurrence about a decided report still stops
// the mailbox: the decision is not durable yet, so it disposes nothing yet,
// or it does not name the occurrence, and no decision replaces it. Evidence
// resolves it either way, as for any occurrence.
func undisposedWhy(dir, name string, stop openStop, view SubjectView) string {
	switch {
	case view.Decided == nil:
		return fmt.Sprintf("about report %s, whose decision does not read, so nothing disposes it: %s", stop.reportOf(), stop.describe(dir, name))
	case !view.Durable:
		return fmt.Sprintf("about report %s, whose decision %s is not durable yet, so it disposes nothing until the next barrier makes it durable: %s", stop.reportOf(), view.Decided.Decision.ID, stop.describe(dir, name))
	}
	return fmt.Sprintf("about report %s, whose decision %s does not dispose this occurrence and cannot be replaced, so only evidence resolves it: %s", stop.reportOf(), view.Decided.Decision.ID, stop.describe(dir, name))
}

// answersOf is what a decided report answers, from the records that keep it
// validated: the resend a decision on "did not arrive" keeps whole; else the
// closure J's tail wrote, naming the waits it closed; else J itself, in
// either form, holding the report. A decision on "arrived" keeps no message
// of its own, so with J gone and no closure that reads, what the report
// answered is unknown, not none.
func (w world) answersOf(name string, view DecisionView) ([]string, bool) {
	d := *view.Decision
	if answers := resendAnswers(d); answers != nil {
		return answers, false
	}
	if c := view.Closure; c != nil && c.Report == d.Subject.Report {
		answers := []string{}
		for _, wait := range c.Waits {
			answers = append(answers, wait.Messages...)
		}
		return answers, false
	}
	path := filepath.Join(JournalPath(w.dir, name), d.Subject.Journal)
	for _, candidate := range []string{path, path + doneSuffix} {
		journal, err := w.readJournalFile(candidate)
		if err != nil {
			continue
		}
		for _, report := range journal.Reports {
			if report.ID == d.Subject.Report {
				return report.InReplyTo, false
			}
		}
	}
	return nil, true
}

// resendAnswers is what the resend a decision keeps whole answers; nil for
// a decision with no resend that reads.
func resendAnswers(d Decision) []string {
	if journal, err := d.resendJournal(); err == nil {
		return journal.Reports[0].InReplyTo
	}
	return nil
}

// Resent is the id of the report a decision on "did not arrive" sends again,
// fixed when the decision was written; empty for "arrived", or for a resend
// the record does not hold whole.
func (d Decision) Resent() string {
	if journal, err := d.resendJournal(); err == nil {
		return journal.Reports[0].ID
	}
	return ""
}

// Parent is the decision whose resend the report is, from the journal that
// carries it; empty for a report no decision sent.
func (s Subject) Parent() string {
	parent, _ := strings.CutPrefix(s.Journal, resendPrefix)
	if parent == s.Journal {
		return ""
	}
	return parent
}

func decisionsRoot(dir, name string) string { return filepath.Dir(DecisionPath(dir, name, "x")) }
