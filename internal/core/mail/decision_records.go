package mail

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// The records a decision's effects leave beside it, each write-once and kept
// while the mailbox lives, each read by one strict reader: a record that does
// not read is never taken for what it would say — a note told, a closure, a
// resend's outcome.

// The dispositions a resend's summary names.
const (
	DispositionPublished = "published"
	DispositionMoot      = "moot"
	DispositionDecided   = "decided"
)

// ResendSummary is what the done form of a resend journal keeps of its
// report: its disposition, the decision that installed it, and the child
// decision when its own outcome was unknown and decided. Moot says an attempt
// found the run ended, never that the report did not land.
type ResendSummary struct {
	Report      string `json:"report"`
	Disposition string `json:"disposition"`
	Decision    string `json:"decision"`
	Child       string `json:"child,omitempty"`
}

// check holds a summary to the journal it is in, named decided-<decision>.
func (s ResendSummary) check(journal string) error {
	decision, ok := strings.CutPrefix(journal, resendPrefix)
	switch {
	case !ok || s.Decision != decision:
		return fmt.Errorf("the summary in %s names the decision %q", journal, s.Decision)
	case !freshID.MatchString(s.Report):
		return fmt.Errorf("the summary in %s names no resend %q", journal, s.Report)
	case s.Disposition == DispositionDecided && !freshID.MatchString(s.Child):
		return fmt.Errorf("the summary in %s says decided and names no child decision", journal)
	case s.Disposition != DispositionDecided && s.Child != "":
		return fmt.Errorf("the summary in %s names a child decision of a resend it says was %s", journal, s.Disposition)
	case s.Disposition != DispositionPublished && s.Disposition != DispositionMoot && s.Disposition != DispositionDecided:
		return fmt.Errorf("the summary in %s says %q, no disposition", journal, s.Disposition)
	}
	return nil
}

// Closure records that a journal's tail closed a decided report's obligation:
// the decision, the journal, and the waits its tail cleared that the report
// answers. It is closure, never delivery.
type Closure struct {
	Decision string    `json:"decision"`
	Journal  string    `json:"journal"`
	Op       string    `json:"op,omitempty"`
	Report   string    `json:"report"`
	Waits    []Waiter  `json:"waits"`
	At       time.Time `json:"at"`
}

// maxClosedWaits bounds a closure: a report answers the waits of one run.
const maxClosedWaits = 1024

func parseClosureFile(path string, raw []byte) error {
	_, err := parseClosure(path, raw)
	return err
}

func parseClosure(path string, raw []byte) (Closure, error) {
	var closure Closure
	if err := strictJSON(raw, &closure); err != nil {
		return Closure{}, fmt.Errorf("the closure %s is not readable: %w", path, err)
	}
	report := strings.TrimSuffix(filepath.Base(path), closedSuffix)
	switch {
	case closure.Report != report || !freshID.MatchString(closure.Decision):
		return Closure{}, fmt.Errorf("the closure %s names report %q and decision %q", path, closure.Report, closure.Decision)
	case !boundedID(closure.Journal) || strings.HasSuffix(closure.Journal, doneSuffix):
		return Closure{}, fmt.Errorf("the closure %s names no journal", path)
	case closure.At.IsZero() || len(closure.Waits) > maxClosedWaits:
		return Closure{}, fmt.Errorf("the closure %s says no time, or more waits than a run has", path)
	}
	return closure, nil
}

// The answers a note's publication gives, which alone record it told.
const (
	ToldWritten = "written"
	ToldBefore  = "written-before"
	ToldEnded   = "run-ended"
)

// ToldRecord says a decision's note was told: the run, and the answer. Only
// a note written, one written before, or a run proven ended is told.
type ToldRecord struct {
	Name   string    `json:"name"`
	Run    string    `json:"run"`
	Note   string    `json:"note"`
	Answer string    `json:"answer"`
	At     time.Time `json:"at"`
}

func parseToldFile(path string, raw []byte) error {
	_, err := parseTold(path, raw)
	return err
}

func parseTold(path string, raw []byte) (ToldRecord, error) {
	var told ToldRecord
	if err := strictJSON(raw, &told); err != nil {
		return ToldRecord{}, fmt.Errorf("the record %s of a note told is not readable: %w", path, err)
	}
	switch {
	case told.Answer != ToldWritten && told.Answer != ToldBefore && told.Answer != ToldEnded:
		return ToldRecord{}, fmt.Errorf("the record %s says %q, no answer that tells a note", path, told.Answer)
	case told.Name == "" || !boundedID(told.Run) || !freshID.MatchString(told.Note) || told.At.IsZero():
		return ToldRecord{}, fmt.Errorf("the record %s names no run, note or time", path)
	}
	return told, nil
}

// readTold reads the told record of a decision's note and holds it to that
// note: one that names another note, name or run says nothing of this one,
// and is an error — neither told nor owed. The installation, its evidence and
// the views read it so alike.
func readTold(files fileAccess, path string, entry DecisionNote) (ToldRecord, error) {
	raw, err := files.ReadFile(path)
	if err != nil {
		return ToldRecord{}, err
	}
	told, err := parseTold(path, raw)
	if err != nil {
		return ToldRecord{}, err
	}
	note := entry.Note
	if told.Name != note.To || told.Run != note.ToEpoch || told.Note != note.ID {
		return ToldRecord{}, fmt.Errorf("the record %s tells note %s to %s's run %s, not note %s to %s's run %s", path, told.Note, told.Name, told.Run, note.ID, note.To, note.ToEpoch)
	}
	return told, nil
}

// toldPath is where a decision's note to whom is recorded told.
func toldPath(dir, name, report, whom string) string {
	suffix := toldRecipientSuffix
	if whom == ToldMain {
		suffix = toldMainSuffix
	}
	return DecisionPath(dir, name, report) + suffix
}

// strictJSON decodes one JSON value into v, refusing fields v has no place
// for and anything after it but JSON's own whitespace. The decoder's More
// is no such check: it asks about the next element of an array or object,
// and answers false on a closing delimiter left after the value.
func strictJSON(raw []byte, v any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(v); err != nil {
		return err
	}
	if rest := bytes.TrimLeft(raw[decoder.InputOffset():], " \t\r\n"); len(rest) > 0 {
		return fmt.Errorf("%d bytes follow the record", len(rest))
	}
	return nil
}
