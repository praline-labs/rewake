package mail

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// An operator decision (docs/v2/stage3-decision.md) is a choice made on a
// report whose outcome is unknown: "arrived" or "did not arrive". It is never
// evidence. It is written once, at decisions/<report-id> of the sender's
// mailbox, before any effect, and holds everything its effects need — the
// occurrences it disposes, the notes to tell and, for "did not arrive", the
// resend journal whole — so that whoever installs them later reads the
// decision alone and generates nothing again (docs/v2/stage3-decision-build.md,
// Q4). One strict reader serves every reader of it (Q6): a decision that does
// not read or does not hold together gives no authority, disposes nothing and
// is an undecidable cause of its own.

// decisionsDir holds a mailbox's decisions and the records beside them.
const decisionsDir = "decisions"

// The records beside a decision, by the suffix of its name.
const (
	toldRecipientSuffix = ".told-recipient"
	toldMainSuffix      = ".told-main"
	closedSuffix        = ".closed"
)

// The choices.
const (
	ChoiceArrived    = "arrived"
	ChoiceNotArrived = "not-arrived"
)

// Who decided.
const (
	ByRecipient = "recipient"
	ByPerson    = "person"
)

// deliveryUnproven is what every decision says of delivery.
const deliveryUnproven = "unproven"

// resendPrefix names the journal a decision installs: journal/decided-<id>.
const resendPrefix = "decided-"

// The bounds of what a decision keeps (Q12): a read, all of them, a listing.
// The record's own bound leaves room for the encoding of the bytes.
const (
	maxObservedRead    = 16 << 20
	maxObservedTotal   = 64 << 20
	maxObservedEntries = 65536
	maxDecisionBytes   = 100 << 20
	maxRecordID        = 128
)

// Decision is the record of one decision.
type Decision struct {
	ID       string    `json:"id"`
	Choice   string    `json:"choice"`
	By       DecidedBy `json:"by"`
	At       time.Time `json:"at"`
	Delivery string    `json:"delivery"`
	Subject  Subject   `json:"subject"`
	// Parent is the decision whose resend this subject is; empty for a
	// report no decision sent.
	Parent string `json:"parent,omitempty"`
	// Disposes are the exact occurrences the decision settles; an
	// occurrence of the same key found later is a new one.
	Disposes []Disposal `json:"disposes"`
	// Notes are the notes owed to the runs that did not decide verified,
	// whole, as they will be published.
	Notes        []DecisionNote `json:"notes,omitempty"`
	Observations []Observation  `json:"observations"`
	// Resend is the resend journal's bytes as they will be linked, for "did
	// not arrive" only.
	Resend json.RawMessage `json:"resend,omitempty"`
}

// DecidedBy is who decided, and whether a wrapper vouched for it.
type DecidedBy struct {
	Kind     string `json:"kind"`
	Name     string `json:"name,omitempty"`
	Run      string `json:"run,omitempty"`
	Verified bool   `json:"verified"`
}

// Subject is the report decided and the operation that carries it.
type Subject struct {
	Sender    string `json:"sender"`
	SenderRun string `json:"senderRun"`
	// Journal is the journal's name under journal/; Op its operation.
	Journal      string `json:"journal"`
	Op           string `json:"op,omitempty"`
	Report       string `json:"report"`
	Kind         Kind   `json:"kind"`
	Recipient    string `json:"recipient"`
	RecipientRun string `json:"recipientRun"`
}

// Disposal names one occurrence of a stop.
type Disposal struct {
	Key        string `json:"key"`
	Occurrence string `json:"occurrence"`
}

// DecisionNote is a note a decision owes: to its recipient run or to main's.
type DecisionNote struct {
	Told string  `json:"told"`
	Note Message `json:"note"`
}

// The runs a note tells.
const (
	ToldRecipient = "recipient"
	ToldMain      = "main"
)

// DecisionPath is the record of the decision on a report.
func DecisionPath(dir, name, report string) string {
	return filepath.Join(state.InboxPath(dir, name), decisionsDir, report)
}

// ResendJournal is the name of the journal a decision installs.
func ResendJournal(decision string) string { return resendPrefix + decision }

// freshID is the shape NewID gives: fixed in length, so a chain of resends
// never grows a name (I7).
var freshID = regexp.MustCompile(`^[0-9]{19}-[0-9a-f]{12}$`)

var stopKey = regexp.MustCompile(`^[0-9a-f]{32}$`)

// boundedID is an id a record may name: one path element, bounded.
func boundedID(id string) bool { return safeID(id) && len(id) <= maxRecordID }

// parseDecisionFile is the reading's check of a decision record.
func parseDecisionFile(path string, raw []byte) error {
	_, err := parseDecision(path, raw)
	return err
}

// parseDecision reads a decision at path strictly: every field it gives
// authority by is checked against its place and against the others before
// anything rests on it.
func parseDecision(path string, raw []byte) (Decision, error) {
	if len(raw) > maxDecisionBytes {
		return Decision{}, fmt.Errorf("the decision %s is %d bytes, past its bound of %d", path, len(raw), maxDecisionBytes)
	}
	var d Decision
	if err := strictJSON(raw, &d); err != nil {
		return Decision{}, fmt.Errorf("the decision %s is not readable: %w", path, err)
	}
	name := filepath.Base(filepath.Dir(filepath.Dir(path)))
	if err := d.check(name, filepath.Base(path)); err != nil {
		return Decision{}, fmt.Errorf("the decision %s does not hold together: %w", path, err)
	}
	return d, nil
}

// check holds a decision to its place — mailbox name and report — and to
// itself.
func (d Decision) check(name, report string) error {
	s := d.Subject
	switch {
	case !freshID.MatchString(d.ID):
		return fmt.Errorf("its id %q is not a fresh id", d.ID)
	case d.Choice != ChoiceArrived && d.Choice != ChoiceNotArrived:
		return fmt.Errorf("its choice %q is neither %s nor %s", d.Choice, ChoiceArrived, ChoiceNotArrived)
	case d.At.IsZero():
		return errors.New("it says no time")
	case d.Delivery != deliveryUnproven:
		return fmt.Errorf("it says delivery %q, where a decision proves nothing", d.Delivery)
	case s.Sender != name || s.Report != report:
		return fmt.Errorf("its subject, report %q of %q, is not the one its place names", s.Report, s.Sender)
	case !boundedID(s.Report) || s.SenderRun == "" || !boundedID(s.SenderRun):
		return errors.New("its subject names no report or no sender's run")
	case !boundedID(s.Journal) || strings.HasSuffix(s.Journal, doneSuffix):
		return fmt.Errorf("its subject names no journal: %q", s.Journal)
	case s.Kind == "" || !state.ValidName(s.Recipient) || !boundedID(s.RecipientRun):
		return errors.New("its subject names no kind, recipient or recipient's run")
	}
	if err := d.checkBy(); err != nil {
		return err
	}
	parent, resent := strings.CutPrefix(s.Journal, resendPrefix)
	if resent != (d.Parent != "") || resent && (parent != d.Parent || !freshID.MatchString(parent)) {
		return fmt.Errorf("its parent %q is not the decision its journal %s was installed by", d.Parent, s.Journal)
	}
	if err := checkDisposals(d.Disposes); err != nil {
		return err
	}
	if err := d.checkNotes(); err != nil {
		return err
	}
	if err := checkObservations(d.Observations); err != nil {
		return err
	}
	if d.Choice == ChoiceArrived {
		if len(d.Resend) != 0 {
			return errors.New("it says arrived and holds a resend")
		}
		return nil
	}
	_, err := d.resendJournal()
	return err
}

func (d Decision) checkBy() error {
	switch by := d.By; by.Kind {
	case ByRecipient:
		if !by.Verified || by.Name != d.Subject.Recipient || by.Run != d.Subject.RecipientRun {
			return errors.New("it was decided by the recipient, but not by the recipient run, verified")
		}
	case ByPerson:
		if by.Verified || by.Name != "" || by.Run != "" {
			return errors.New("it was decided by the person, who is never verified and names no run")
		}
	default:
		return fmt.Errorf("it was decided by %q, neither the recipient nor the person", by.Kind)
	}
	return nil
}

func checkDisposals(disposes []Disposal) error {
	if len(disposes) == 0 {
		return errors.New("it disposes no occurrence")
	}
	seen := map[Disposal]bool{}
	for _, disposal := range disposes {
		if !stopKey.MatchString(disposal.Key) || !boundedID(disposal.Occurrence) || seen[disposal] {
			return fmt.Errorf("it disposes %+v, which names no occurrence once", disposal)
		}
		seen[disposal] = true
	}
	return nil
}

func (d Decision) checkNotes() error {
	told := map[string]bool{}
	runs := map[string]bool{}
	for _, entry := range d.Notes {
		note := entry.Note
		run := note.To + "\x00" + note.ToEpoch
		switch {
		case entry.Told != ToldRecipient && entry.Told != ToldMain || told[entry.Told]:
			return fmt.Errorf("it owes a note to %q, which is no run it tells, once", entry.Told)
		case note.Kind != Note || !freshID.MatchString(note.ID) || note.From != d.Subject.Sender:
			return fmt.Errorf("its note %q is not a note from %s with a fresh id", note.ID, d.Subject.Sender)
		case !state.ValidName(note.To) || !boundedID(note.ToEpoch) || runs[run]:
			return fmt.Errorf("its note %q names no run, or one already told", note.ID)
		case entry.Told == ToldRecipient && (note.To != d.Subject.Recipient || note.ToEpoch != d.Subject.RecipientRun):
			return fmt.Errorf("its note to the recipient goes to %s's run %s", note.To, note.ToEpoch)
		}
		told[entry.Told], runs[run] = true, true
	}
	return nil
}

// resendJournal is the resend the decision holds, checked as the one report
// of the subject sent again: its text, kind and runs, a fresh id, naming the
// report and the decision, and nothing of the original journal's tail.
func (d Decision) resendJournal() (TurnJournal, error) {
	if len(d.Resend) == 0 {
		return TurnJournal{}, errors.New("it says did not arrive and holds no resend")
	}
	var journal TurnJournal
	if err := strictJSON(d.Resend, &journal); err != nil {
		return TurnJournal{}, fmt.Errorf("its resend is not a journal: %w", err)
	}
	s := d.Subject
	if len(journal.Reports) != 1 {
		return TurnJournal{}, fmt.Errorf("its resend carries %d reports, not one", len(journal.Reports))
	}
	report := journal.Reports[0]
	switch {
	case journal.Epoch != s.SenderRun || journal.Op != ResendJournal(d.ID):
		return TurnJournal{}, errors.New("its resend is not an operation of its own on the sender's run")
	case journal.Ended != 0 || len(journal.Clear) != 0 || journal.Kept != nil || journal.Held != "" || journal.Mark != nil ||
		journal.Interim != nil || journal.Settles:
		return TurnJournal{}, errors.New("its resend carries a turn end's tail, which is the original's")
	case len(journal.Published) != 0 || len(journal.Moot) != 0 || len(journal.Decided) != 0 || journal.Done || journal.Summary != nil:
		return TurnJournal{}, errors.New("its resend has progressed before it was installed")
	case !freshID.MatchString(report.ID) || report.ID == s.Report || report.Resends != s.Report || report.Decision != d.ID:
		return TurnJournal{}, errors.New("its resend's report is not the subject sent again under a fresh id by this decision")
	case report.From != s.Sender || report.FromEpoch != s.SenderRun || report.To != s.Recipient || report.ToEpoch != s.RecipientRun ||
		report.Kind != s.Kind:
		return TurnJournal{}, errors.New("its resend's report is not addressed as the subject was")
	}
	return journal, nil
}
