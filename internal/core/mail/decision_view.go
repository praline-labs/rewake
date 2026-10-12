package mail

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// What the mailbox holds of decisions, read without a lock, a lease or any
// effect (Q10 of docs/v2/stage3-decision-build.md): such a read is no
// snapshot, so each record says only what was established when it was read,
// a record missing is a phase not reached yet rather than an effect that did
// not happen, and a record that does not read is named, never skipped.

// DecisionView is one decision and the records its effects left.
type DecisionView struct {
	Report string
	// Decision is the record; nil when it does not read, and Err says why.
	Decision *Decision
	Err      error
	// Resend is the journal the decision installs, for "did not arrive".
	Resend ResendView
	// Told are the notes the decision owes, by the run each tells.
	Told map[string]ToldView
	// Closure is its report's obligation closed; nil while J's tail has not
	// run, and ClosureErr says why when its record does not read.
	Closure    *Closure
	ClosureErr error
}

// ResendView is how far a resend has gone: not installed, installed and
// running, or done with its outcome — or kept with none.
type ResendView struct {
	Installed bool
	Done      bool
	Summary   *ResendSummary
	Err       error
}

// ToldView is a note owed, or the record that it was told.
type ToldView struct {
	Record *ToldRecord
	Err    error
}

// Decisions are the decisions of name's mailbox, by report.
func Decisions(dir, name string) ([]DecisionView, error) {
	root, entries, err := world{dir: dir, files: osAccess{}}.decisionsOf(name)
	if err != nil {
		return nil, fmt.Errorf("the decisions %s cannot be listed, so which reports were decided is unknown: %w", root, err)
	}
	var views []DecisionView
	for _, entry := range entries {
		report := entry.Name()
		if isDecisionRecord(report) {
			views = append(views, decisionView(dir, name, report))
		}
	}
	sort.Slice(views, func(i, j int) bool { return views[i].Report < views[j].Report })
	return views, nil
}

// decisionsOf lists the decisions of name's mailbox. None there is no
// decision; a link, or anything but a directory, in that place is an unknown
// and is never followed: what it leads to is no record of this mailbox.
func (w world) decisionsOf(name string) (string, []fs.DirEntry, error) {
	root := filepath.Dir(DecisionPath(w.dir, name, "x"))
	info, err := w.lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		return root, nil, nil
	}
	if err != nil {
		return root, nil, err
	}
	if !info.IsDir() {
		return root, nil, unknownRecord(root, fmt.Errorf("%s, where the decisions belong, is no directory: %s", root, info.Mode().Type()))
	}
	entries, err := w.readDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return root, nil, nil
	}
	return root, entries, err
}

// isDecisionRecord says a name under decisions/ is a decision rather than a
// record beside one or a write that never finished.
func isDecisionRecord(name string) bool {
	return !strings.HasPrefix(name, ".") && !strings.HasSuffix(name, toldRecipientSuffix) &&
		!strings.HasSuffix(name, toldMainSuffix) && !strings.HasSuffix(name, closedSuffix)
}

func decisionView(dir, name, report string) DecisionView {
	view := DecisionView{Report: report, Told: map[string]ToldView{}}
	path := DecisionPath(dir, name, report)
	d, err := readDecision(osAccess{}, path)
	if err != nil {
		view.Err = err
		return view
	}
	view.Decision = &d
	if d.Choice == ChoiceNotArrived {
		view.Resend = resendView(dir, name, d.ID)
	}
	for _, note := range d.Notes {
		raw, err := os.ReadFile(toldPath(dir, name, report, note.Told))
		switch {
		case errors.Is(err, os.ErrNotExist):
			view.Told[note.Told] = ToldView{}
		case err != nil:
			view.Told[note.Told] = ToldView{Err: err}
		default:
			told, err := parseTold(toldPath(dir, name, report, note.Told), raw)
			view.Told[note.Told] = ToldView{Record: &told, Err: err}
			if err == nil && told.Note != note.Note.ID {
				view.Told[note.Told] = ToldView{Err: fmt.Errorf("the record of the note told names note %s, not %s", told.Note, note.Note.ID)}
			}
		}
	}
	closure, err := readClosure(osAccess{}, path+closedSuffix, d)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		view.ClosureErr = err
	default:
		view.Closure = &closure
	}
	return view
}

// resendView looks for the resend open and then done: a journal is renamed
// from the one to the other and never back, and never swept, so a look in
// that order that finds neither saw it not installed yet.
func resendView(dir, name, decision string) ResendView {
	path := filepath.Join(JournalPath(dir, name), ResendJournal(decision))
	for _, at := range []string{path, path + doneSuffix} {
		journal, err := readJournalFile(at)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return ResendView{Installed: true, Err: err}
		}
		return ResendView{Installed: true, Done: journal.Done, Summary: journal.Summary}
	}
	return ResendView{}
}

// readDecision reads the decision at path through files.
func readDecision(files fileAccess, path string) (Decision, error) {
	raw, err := files.ReadFile(path)
	if err != nil {
		return Decision{}, err
	}
	return parseDecision(path, raw)
}

// readClosure reads the closure at path and holds it to the decision it is
// beside: one that names another decision or journal closed nothing of it.
func readClosure(files fileAccess, path string, d Decision) (Closure, error) {
	raw, err := files.ReadFile(path)
	if err != nil {
		return Closure{}, err
	}
	closure, err := parseClosure(path, raw)
	if err != nil {
		return Closure{}, err
	}
	if closure.Decision != d.ID || closure.Journal != d.Subject.Journal {
		return Closure{}, fmt.Errorf("the closure %s names decision %s of journal %s, not %s of %s", path, closure.Decision, closure.Journal, d.ID, d.Subject.Journal)
	}
	return closure, nil
}

// disposals are the occurrences the valid decisions of a mailbox dispose,
// each with the operation it must be about: a decision disposes only the
// occurrences about its own report, named exactly, never a key, a journal or
// every occurrence a gate holds. A decision that does not read disposes
// nothing; the reading stops the mailbox on it on its own.
func (w world) disposals(name string) (map[Disposal]string, error) {
	root, entries, err := w.decisionsOf(name)
	if err != nil {
		return nil, fmt.Errorf("the decisions %s cannot be listed, so what they dispose is unknown: %w", root, err)
	}
	disposed := map[Disposal]string{}
	for _, entry := range entries {
		if !isDecisionRecord(entry.Name()) {
			continue
		}
		d, err := readDecision(w.files, filepath.Join(root, entry.Name()))
		if err != nil {
			continue
		}
		for _, disposal := range d.Disposes {
			disposed[disposal] = d.Subject.Journal + "/" + d.Subject.Report
		}
	}
	return disposed, nil
}

// disposedBy says a decision disposes the occurrence: it names it, the
// occurrence's record reads and is about the decision's report, and nothing
// beside it leaves its state unknown.
func (o openStop) disposedBy(disposed map[Disposal]string) bool {
	op, ok := disposed[Disposal{Key: o.key, Occurrence: o.id}]
	return ok && o.unreadable == nil && o.resolution == nil && o.record.Op == op
}
