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
	// Summary is its outcome once done; nil while it runs, and nil with
	// SummaryErr when a done resend's summary does not read: completed,
	// outcome not kept.
	Summary    *ResendSummary
	SummaryErr error
	Err        error
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
		!strings.HasSuffix(name, toldMainSuffix) && !strings.HasSuffix(name, closedSuffix) && !strings.HasSuffix(name, durableSuffix)
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
		view.Resend = resendView(dir, name, d)
	}
	for _, note := range d.Notes {
		told, err := readTold(osAccess{}, toldPath(dir, name, report, note.Told), note)
		switch {
		case errors.Is(err, os.ErrNotExist):
			view.Told[note.Told] = ToldView{}
		case err != nil:
			view.Told[note.Told] = ToldView{Err: err}
		default:
			view.Told[note.Told] = ToldView{Record: &told}
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

// resendView is how far d's resend has gone, read as the installation reads
// it (readResend).
func resendView(dir, name string, d Decision) ResendView {
	journal, summaryErr, found, err := world{dir: dir, files: osAccess{}}.readResend(name, d)
	if err != nil {
		return ResendView{Installed: true, Err: err}
	}
	return ResendView{Installed: found, Done: journal.Done, Summary: journal.Summary, SummaryErr: summaryErr}
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
// beside: one that names another decision, journal or operation, or a wait
// of another run than the report's recipient, closed nothing of it.
func readClosure(files fileAccess, path string, d Decision) (Closure, error) {
	raw, err := files.ReadFile(path)
	if err != nil {
		return Closure{}, err
	}
	closure, err := parseClosure(path, raw)
	if err != nil {
		return Closure{}, err
	}
	s := d.Subject
	if closure.Decision != d.ID || closure.Journal != s.Journal || closure.Op != s.Op {
		return Closure{}, fmt.Errorf("the closure %s names decision %s of journal %s, operation %q, not %s of %s, operation %q", path, closure.Decision, closure.Journal, closure.Op, d.ID, s.Journal, s.Op)
	}
	// J may be gone, and with it the tail to compare with; what the report
	// answers is the waits of its recipient's run, and no other.
	for _, wait := range closure.Waits {
		if wait.Name != s.Recipient || wait.Epoch != s.RecipientRun || len(wait.Messages) == 0 {
			return Closure{}, fmt.Errorf("the closure %s names a wait of %s's run %s, where the report answers %s's run %s", path, wait.Name, wait.Epoch, s.Recipient, s.RecipientRun)
		}
	}
	return closure, nil
}

// disposals are the occurrences the valid decisions of a mailbox dispose,
// each with the operation it must be about: a decision disposes only the
// occurrences about its own report, named exactly, never a key, a journal or
// every occurrence a gate holds. A decision that does not read disposes
// nothing; the reading stops the mailbox on it on its own. Nor does one not
// made durable yet (makeDurable): its authority may still be lost.
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
		path := filepath.Join(root, entry.Name())
		d, err := readDecision(w.files, path)
		if err != nil || !w.durable(path, d) {
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
