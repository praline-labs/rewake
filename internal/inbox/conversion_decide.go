package inbox

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/praline-labs/rewake/internal/registry"
)

// evidence is what the mailboxes prove of one report's publication.
type evidence int

const (
	evidenceUnknown evidence = iota
	evidenceDone
	evidenceNotDone
)

// legacy(rewake <2026-09-30): the earlier build published a turn end's reports with no mark of its own; remove when no session started by an earlier build is registered
//
// evidenceOf establishes one earlier-build report's state
// (docs/turn-end-recovery.md#the-evidence-for-each-effect): done by its entry
// here, a person's decision, the receipt's done mark, a published mark, or its
// letter, which is recorded here first since the sweep takes it; not done only
// by a person's decision, since that build left nothing that proves absence.
// A mailbox that cannot be searched, or a mark that says neither, is an error:
// a later look may answer.
func (c *conversionJournal) evidenceOf(w world, name string, ref reportRef) (evidence, error) {
	dir := w.dir
	id := ref.report.ID
	if slices.Contains(c.Published, id) {
		return evidenceDone, nil
	}
	if delivered, ok := c.Settled[id]; ok {
		if delivered {
			return evidenceDone, nil
		}
		return evidenceNotDone, nil
	}
	if ref.receipt.Done {
		return evidenceDone, nil
	}
	if path, ok := oncePath(dir, ref.report.To, ref.report.ToEpoch, id); ok {
		mark, err := w.readOnceMark(path)
		if err != nil {
			return evidenceUnknown, err
		}
		if mark == oncePublished {
			return evidenceDone, nil
		}
	}
	found, err := w.present(ref.report.To, id)
	if err != nil {
		return evidenceUnknown, fmt.Errorf("whether the earlier build's report %s reached %s cannot be told: %w", id, ref.report.To, err)
	}
	if found {
		c.Published = append(c.Published, id)
		return evidenceDone, c.saveIn(w, name)
	}
	return evidenceUnknown, nil
}

// decide establishes every report's state, then which obligations proven
// publications closed, and only then decides the rest
// (docs/turn-end-recovery.md#reconciliation). It answers the steps that carry
// the decisions, or the reports nothing can decide, which stop the mailbox.
func (c *conversionJournal) decide(w world, name string) ([]TurnJournal, []string, error) {
	refs := c.reports()
	states := make([]evidence, len(refs))
	closed := map[string]bool{}
	for i, ref := range refs {
		state, err := c.evidenceOf(w, name, ref)
		if err != nil {
			return nil, nil, err
		}
		states[i] = state
		if state == evidenceDone && ref.closes() {
			for _, key := range ref.obligations() {
				closed[key] = true
			}
		}
	}
	var unknown []string
	publish := map[string]bool{}
	// moot holds what closing reports that can reach nobody answered: no
	// report will answer it, and step 6 clears it as it clears the closed.
	moot := map[string]bool{}
	for i, ref := range refs {
		keys := ref.obligations()
		some := slices.ContainsFunc(keys, func(key string) bool { return closed[key] })
		all := len(keys) > 0 && !slices.ContainsFunc(keys, func(key string) bool { return !closed[key] })
		ended, err := w.mootRecipient(ref.report)
		if err != nil {
			return nil, nil, err
		}
		switch {
		case states[i] == evidenceDone, all:
			// Out, or superseded by the reports that closed what it answers.
		case ended:
			// For a run of this build that has ended: it can reach nobody.
			if ref.closes() {
				for _, key := range keys {
					moot[key] = true
				}
			}
		case states[i] == evidenceNotDone && !some:
			publish[ref.report.ID] = true
		case states[i] == evidenceNotDone:
			// Partly answered by another: not published, and the rest stays
			// owed for the next end.
		case !ref.closes():
			// Losing a note repeats nothing: withheld for good.
			if !slices.Contains(c.Withheld, ref.report.ID) {
				c.Withheld = append(c.Withheld, ref.report.ID)
			}
		default:
			// Unknown, and whether it answered what nothing else closed is
			// unknown too.
			unknown = append(unknown, ref.report.ID)
		}
	}
	if len(unknown) > 0 {
		return nil, unknown, nil
	}
	for _, ref := range refs {
		if publish[ref.report.ID] && ref.closes() {
			for _, key := range ref.obligations() {
				closed[key] = true
			}
		}
	}
	var steps []TurnJournal
	for _, receipt := range c.Receipts {
		run := receipt.run()
		if !receipt.Prepared || run == "" {
			continue
		}
		step := TurnJournal{Epoch: run, Op: "conversion/" + receipt.File}
		for _, report := range receipt.Reports {
			if publish[report.ID] {
				step.Reports = append(step.Reports, report)
			}
		}
		for _, waiter := range receipt.Waiters {
			cleared := waiter
			cleared.Messages, cleared.ReadSequences, cleared.ReadAt = nil, nil, nil
			for _, id := range waiter.Messages {
				if key := waiter.Name + "\x00" + waiter.Epoch + "\x00" + id; closed[key] || moot[key] {
					cleared.Messages = append(cleared.Messages, id)
				}
			}
			if len(cleared.Messages) > 0 {
				step.Clear = append(step.Clear, cleared)
			}
		}
		steps = append(steps, step)
	}
	if steps == nil {
		steps = []TurnJournal{}
	}
	return steps, nil, nil
}

// mootRecipient says a report can reach nobody: its recipient is a run of
// this build that has ended or was replaced. One for a run of the earlier
// build is not moot merely because that run ended: the name's successor
// decides it (journal_held.go).
func (w world) mootRecipient(report Message) (bool, error) {
	if registry.EarlierBuildEpoch(report.ToEpoch) {
		return false, nil
	}
	peer, err := w.lookup(report.To)
	if errors.Is(err, registry.ErrNotFound) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return peer.Epoch() != report.ToEpoch, nil
}

// StoppedError says a mailbox stopped changing on reports nothing can decide
// (docs/turn-end-recovery.md#the-stop-and-rewake-settle): every call that would
// change it answers this, until each report is settled.
type StoppedError struct {
	Name    string
	Journal string
	Reports []Message
}

func (e *StoppedError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Rewake: %s stopped changing its mailbox. Its record %s holds %d report(s) rewake cannot tell were delivered: an earlier build may have written them and the sweep removed them since, or never written them.\n", e.Name, e.Journal, len(e.Reports))
	for _, report := range e.Reports {
		fmt.Fprintf(&b, "  report %s to %s (run %s): %s\n", report.ID, report.To, report.ToEpoch, strings.Join(report.InReplyTo, ", "))
	}
	fmt.Fprintf(&b, "Until each is settled, %s reads, reports and clears nothing. Ask each recipient whether it has that report, then run one line per report:\n", e.Name)
	if len(e.Reports) > 0 {
		id := e.Reports[0].ID
		fmt.Fprintf(&b, "  rewake settle %s %s --delivered     it arrived; what it answered counts as answered\n", e.Name, id)
		fmt.Fprintf(&b, "  rewake settle %s %s --undelivered   it did not; it goes now", e.Name, id)
	}
	return b.String()
}

func (c *conversionJournal) stopped(dir, name string) *StoppedError {
	stop := &StoppedError{Name: name, Journal: conversionPath(dir, name)}
	for _, ref := range c.reports() {
		if slices.Contains(c.Unknown, ref.report.ID) {
			stop.Reports = append(stop.Reports, ref.report)
		}
	}
	return stop
}
