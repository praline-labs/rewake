package mail

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// A decision's subject is read on its own (Q1 and Q5 of
// docs/v2/stage3-decision-build.md): the open occurrences about the report,
// the one journal they name and the report's step in it — never through the
// mailbox-wide plan, which stops at the first unreadable journal whether or
// not it has anything to do with this report. A subject that reads is
// decidable while other records of the mailbox are not.

// subjectPlan is a subject as the decision's plan read it.
type subjectPlan struct {
	subject Subject
	journal TurnJournal
	report  Message
	// about are the open occurrences about the report the decision disposes:
	// their records read and nothing beside them leaves their state unknown.
	about []openStop
}

// FindSubject is the subject of report in name's mailbox as it reads now,
// with no lock: what a caller looks up before it asks for a decision, to
// learn whose it is. A decision already made gives its own subject, which
// outlives the journal. The decision itself reads the subject again under
// the lock and refuses one that changed.
func FindSubject(dir, name, report string) (Subject, error) {
	w := live(dir)
	if !boundedID(report) {
		return Subject{}, refuseDecision(DecisionNoSubject, "", "%q is no report id", report)
	}
	path := DecisionPath(dir, name, report)
	d, err := readDecision(w.files, path)
	if err == nil {
		return d.Subject, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return Subject{}, &DecisionError{Outcome: DecisionUnreadable, Path: path, Err: err}
	}
	plan, err := w.subjectOf(name, report)
	return plan.subject, err
}

// subjectOf reads report's subject in name's mailbox through w.
func (w world) subjectOf(name, report string) (subjectPlan, error) {
	if !boundedID(report) {
		return subjectPlan{}, refuseDecision(DecisionNoSubject, "", "%q is no report id", report)
	}
	open, _, err := w.openOccurrences(name)
	if err != nil {
		return subjectPlan{}, &DecisionError{Outcome: DecisionUnreadable, Path: stopsPath(w.dir, name), Err: err}
	}
	var about []openStop
	journals := map[string]bool{}
	for _, stop := range open {
		if stop.unreadable != nil || stop.reportOf() != report {
			continue
		}
		journal, _, _ := strings.Cut(stop.record.Op, "/")
		journals[journal] = true
		if stop.resolution == nil {
			about = append(about, stop)
		}
	}
	switch {
	case len(journals) == 0:
		return subjectPlan{}, refuseDecision(DecisionNoSubject, "", "no open stop of %s is about report %s", name, report)
	case len(journals) > 1:
		return subjectPlan{}, refuseDecision(DecisionNoSubject, "", "the stops of %s about report %s name %d journals, so which operation carries it is not known", name, report, len(journals))
	case len(about) == 0:
		return subjectPlan{}, refuseDecision(DecisionNoSubject, "", "every stop of %s about report %s has a resolution that does not read, so none can be disposed of", name, report)
	}
	return w.subjectIn(name, slices.Collect(maps.Keys(journals))[0], report, about)
}

// subjectIn reads the journal the occurrences name and the report's step in
// it.
func (w world) subjectIn(name, id, report string, about []openStop) (subjectPlan, error) {
	if !boundedID(id) || strings.HasSuffix(id, doneSuffix) {
		return subjectPlan{}, refuseDecision(DecisionNoSubject, "", "the stops about report %s name %q, which is no journal", report, id)
	}
	path := filepath.Join(JournalPath(w.dir, name), id)
	raw, err := w.readFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return subjectPlan{}, refuseDecision(DecisionNoSubject, path, "the journal that carries report %s is gone", report)
	}
	if err != nil {
		return subjectPlan{}, &DecisionError{Outcome: DecisionUnreadable, Path: path, Err: err}
	}
	journal, err := parseJournal(path, raw)
	if err != nil {
		return subjectPlan{}, refuseDecision(DecisionNoSubject, path, "the journal that carries report %s does not read, so nothing can decide it: %v", report, err)
	}
	var found []Message
	for _, message := range journal.Reports {
		if message.ID == report {
			found = append(found, message)
		}
	}
	settled := slices.Contains(journal.Published, report) || slices.Contains(journal.Moot, report) ||
		slices.ContainsFunc(journal.Decided, func(step DecidedStep) bool { return step.Report == report })
	switch {
	case len(found) != 1:
		return subjectPlan{}, refuseDecision(DecisionNoSubject, path, "the journal carries report %s %d times", report, len(found))
	case settled:
		return subjectPlan{}, refuseDecision(DecisionNoSubject, path, "report %s is settled already", report)
	case journal.Done:
		return subjectPlan{}, refuseDecision(DecisionNoSubject, path, "the journal that carries report %s is done", report)
	}
	message := found[0]
	subject := Subject{
		Sender: name, SenderRun: journal.Epoch, Journal: id, Op: journal.Op, Report: report, Kind: message.Kind,
		Recipient: message.To, RecipientRun: message.ToEpoch,
	}
	// The report is the sender run's own only if it says so: one of another
	// run, or of none, carried in this journal is no step a decision of this
	// run may dispose.
	if subject.SenderRun == "" || !boundedID(subject.SenderRun) || subject.Kind == "" || !state.ValidName(subject.Recipient) || !boundedID(subject.RecipientRun) ||
		message.From != name || message.FromEpoch != journal.Epoch {
		return subjectPlan{}, refuseDecision(DecisionNoSubject, path, "report %s does not name its sender and the journal's run, its kind and its recipient's run", report)
	}
	return subjectPlan{subject: subject, journal: journal, report: message, about: about}, nil
}

// parentOf is the decision whose resend a subject is: empty for a report no
// decision sent.
func (s Subject) parentOf() string {
	parent, _ := strings.CutPrefix(s.Journal, resendPrefix)
	if parent == s.Journal {
		return ""
	}
	return parent
}
