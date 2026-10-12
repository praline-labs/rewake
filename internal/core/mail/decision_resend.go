package mail

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// A journal a decision installed is read strictly, by its name: the envelope
// says what the resend is — its operation, its one report and how far it went,
// or that it is done — and bytes of any other kind under that name are an
// unknown, never completed nor written over (Q7, Q8 of
// docs/v2/stage3-decision-build.md). Its summary is apart: only the done form
// keeps one, and one that does not read loses the outcome, never the
// completion the envelope establishes.

// resendWire is a resend journal as it is read, its summary left in bytes so
// that a summary that does not read is no envelope that does not read.
type resendWire struct {
	TurnJournal
	Summary json.RawMessage `json:",omitempty"`
}

// parseResend reads the resend journal named name from raw. A summary that
// does not read leaves the journal's Summary nil and is answered apart, as
// summaryErr; anything wrong with the envelope is an unknown.
func parseResend(path, name string, raw []byte) (journal TurnJournal, summaryErr error, err error) {
	var wire resendWire
	if err := strictJSON(raw, &wire); err != nil {
		return TurnJournal{}, nil, unknownRecord(path, fmt.Errorf("the resend journal %s is not readable: %w", path, err))
	}
	journal = wire.TurnJournal
	summary := len(wire.Summary) != 0 && !bytes.Equal(wire.Summary, []byte("null"))
	if err := journal.checkResend(name, strings.HasSuffix(path, doneSuffix), summary); err != nil {
		return TurnJournal{}, nil, unknownRecord(path, fmt.Errorf("the resend journal %s does not hold together: %w", path, err))
	}
	if !summary {
		return journal, nil, nil
	}
	var kept ResendSummary
	if err := strictJSON(wire.Summary, &kept); err != nil {
		return journal, fmt.Errorf("the summary in %s does not read: %w", path, err), nil
	}
	if err := kept.check(name); err != nil {
		return journal, err, nil
	}
	journal.Summary = &kept
	return journal, nil, nil
}

// checkResend holds a resend journal's envelope to its name: the operation the
// name fixes on a run, no turn end's tail, and either open — the decision's one
// report and progress about that report alone — or done, its steps emptied. A
// journal renamed done says so, and only a done one keeps a summary.
func (j TurnJournal) checkResend(name string, renamed, summary bool) error {
	decision := strings.TrimPrefix(name, resendPrefix)
	switch {
	case !freshID.MatchString(decision):
		return fmt.Errorf("%s names no decision", name)
	case j.Epoch == "" || j.Op != name:
		return fmt.Errorf("it is not the operation %s on a run", name)
	case j.Ended != 0 || len(j.Clear) != 0 || j.Kept != nil || j.Held != "" || j.Mark != nil || j.Interim != nil || j.Settles:
		return errors.New("it carries a turn end's tail")
	case renamed && !j.Done:
		return errors.New("it is renamed done and does not say it is done")
	case summary && !j.Done:
		return errors.New("it keeps a summary and is not done")
	case j.Done && (len(j.Reports) != 0 || len(j.Published) != 0 || len(j.Moot) != 0 || len(j.Decided) != 0):
		return errors.New("it is done and keeps its steps")
	case j.Done:
		return nil
	case len(j.Reports) != 1 || j.Reports[0].Decision != decision:
		return fmt.Errorf("it carries no one report of the decision %s", decision)
	}
	if err := j.checkDecided(name); err != nil {
		return err
	}
	report := j.Reports[0].ID
	for _, ids := range [][]string{j.Published, j.Moot} {
		for _, id := range ids {
			if id != report {
				return fmt.Errorf("it records progress of %q, not its report", id)
			}
		}
	}
	for _, step := range j.Decided {
		if step.Report != report {
			return fmt.Errorf("it records %q decided, not its report", step.Report)
		}
	}
	return nil
}

// readResend reads the resend d installed, open and then done — a journal is
// renamed from the one to the other and never back — and holds it to d: the
// sender's run, the report d holds, and a readable summary of that report.
// found false with no error is a resend not installed yet. What is there and
// does not read, or is another resend, is an unknown.
func (w world) readResend(name string, d Decision) (journal TurnJournal, summaryErr error, found bool, err error) {
	id := ResendJournal(d.ID)
	path := filepath.Join(JournalPath(w.dir, name), id)
	for _, at := range []string{path, path + doneSuffix} {
		raw, err := w.readFile(at)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return TurnJournal{}, nil, true, err
		}
		journal, summaryErr, err := parseResend(at, id, raw)
		if err != nil {
			return TurnJournal{}, nil, true, err
		}
		if err := d.installedAs(journal); err != nil {
			return TurnJournal{}, nil, true, unknownRecord(at, fmt.Errorf("the resend journal %s is not the one decision %s holds: %w", at, d.ID, err))
		}
		return journal, summaryErr, true, nil
	}
	return TurnJournal{}, nil, false, nil
}

// installedAs holds a resend journal found under d's name to the resend d
// holds: the same run, and the same report while open or in its summary once
// done.
func (d Decision) installedAs(journal TurnJournal) error {
	held, err := d.resendJournal()
	if err != nil {
		return err
	}
	report := held.Reports[0]
	switch {
	case journal.Epoch != held.Epoch:
		return fmt.Errorf("it is of the run %q", journal.Epoch)
	case journal.Summary != nil && journal.Summary.Report != report.ID:
		return fmt.Errorf("its summary is of the report %q", journal.Summary.Report)
	case journal.Done:
		return nil
	}
	want, err := json.Marshal(report)
	if err != nil {
		return err
	}
	got, err := json.Marshal(journal.Reports[0])
	if err != nil {
		return err
	}
	if !bytes.Equal(want, got) {
		return fmt.Errorf("its report %s is not the one the decision holds", journal.Reports[0].ID)
	}
	return nil
}
