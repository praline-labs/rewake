package inbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// legacy(rewake <2026-09-30): the earlier build kept a turn end's reports and waits in its receipt, with no journal; remove when no session started by an earlier build is registered
//
// The earlier build's receipts are met by the barrier of the name's next run
// (docs/turn-end-recovery.md#reconciliation): all of them are converted, at
// once, into one conversion journal holding a decision for every report,
// before any of them is removed. They are all of ended runs, since this build
// does not act for a run of that build, so no writer adds to them. Their
// origin is kept: a missing mark never proves a report unpublished, since
// that build kept none, and a done mark proves the reports it lists and
// nothing after them. The conversion journal holds the person's decisions
// too (Settle), so once done it is kept whole and for good.

// conversionFile names the conversion journal among the turn journals.
const conversionFile = "conversion"

type conversionJournal struct {
	Receipts []convertedReceipt
	// Published lists the reports whose letter was found: the sender's own
	// proof, which outlives the sweep of the letter.
	Published []string `json:",omitempty"`
	// Settled holds the person's decisions: true for delivered.
	Settled map[string]bool `json:",omitempty"`
	// Unknown lists the reports whose publication nothing proves, while the
	// mailbox is stopped on them; Withheld those that close nothing, which
	// are never weighed again.
	Unknown  []string `json:",omitempty"`
	Withheld []string `json:",omitempty"`
	// Steps carry the decisions, one per receipt, once there is no unknown
	// left: the reports to publish and the waits to clear.
	Steps []TurnJournal `json:",omitempty"`
	Done  bool          `json:",omitempty"`
}

// convertedReceipt is an earlier build's receipt as that build wrote it.
type convertedReceipt struct {
	File        string
	Done        bool
	Prepared    bool
	KeepWaiters bool
	Interim     bool
	Waiters     []Waiter  `json:",omitempty"`
	Reports     []Message `json:",omitempty"`
}

// run is the earlier run a receipt belongs to: the one its reports came
// from; a receipt without reports names none.
func (r convertedReceipt) run() string {
	if len(r.Reports) == 0 {
		return ""
	}
	return r.Reports[0].FromEpoch
}

func conversionPath(dir, name string) string {
	return filepath.Join(JournalPath(dir, name), conversionFile)
}

func readConversion(dir, name string) (*conversionJournal, error) {
	return live(dir).readConversion(name)
}

func (w world) readConversion(name string) (*conversionJournal, error) {
	path := conversionPath(w.dir, name)
	raw, err := w.readFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("the conversion journal %s cannot be read: %w", path, err)
	}
	var journal conversionJournal
	if err := json.Unmarshal(raw, &journal); err != nil {
		return nil, unknownRecord(path, fmt.Errorf("the conversion journal %s is not readable: %w", path, err))
	}
	return &journal, nil
}

func (c *conversionJournal) save(dir, name string) error { return c.saveIn(live(dir), name) }

func (c *conversionJournal) saveIn(w world, name string) error {
	if err := w.ensureDir(JournalPath(w.dir, name)); err != nil {
		return err
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return w.writeFile(conversionPath(w.dir, name), raw)
}

// clone is a copy the plan may decide in, leaving this one to the effects.
func (c *conversionJournal) clone() *conversionJournal {
	if c == nil {
		return nil
	}
	var copied conversionJournal
	raw, err := json.Marshal(c)
	if err == nil {
		err = json.Unmarshal(raw, &copied)
	}
	if err != nil {
		panic(err)
	}
	return &copied
}

// earlierReceipts reads every receipt the earlier build left in the mailbox.
// This build writes none, so every one is that build's. One that cannot be
// read stops the barrier, naming it.
func earlierReceipts(dir, name string) ([]convertedReceipt, error) {
	return live(dir).earlierReceipts(name)
}

func (w world) earlierReceipts(name string) ([]convertedReceipt, error) {
	dir := w.dir
	entries, err := w.readDir(TurnsPath(dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("the earlier build's turn receipts %s cannot be listed: %w", TurnsPath(dir, name), err)
	}
	var receipts []convertedReceipt
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		path := filepath.Join(TurnsPath(dir, name), entry.Name())
		raw, err := w.readFile(path)
		receipt := convertedReceipt{File: entry.Name()}
		if err == nil {
			err = unknownRead(path, json.Unmarshal(raw, &receipt))
		}
		if err != nil {
			return nil, fmt.Errorf("the earlier build's turn receipt %s is not readable: %w", path, err)
		}
		receipt.File = entry.Name()
		receipts = append(receipts, receipt)
	}
	return receipts, nil
}

// convertReceipts writes the conversion journal from every receipt at once,
// and only then removes them. A conversion that died between the two left
// receipts the journal holds already: they are its own, removed on the next
// look, and weighed as the journal records them.
func (w world) convertReceipts(name string, records mailboxRecords) (*conversionJournal, error) {
	dir := w.dir
	journal, leftovers := records.conversion, records.leftovers
	if journal == nil && len(records.receipts) > 0 {
		journal = &conversionJournal{Receipts: records.receipts}
		if err := journal.saveIn(w, name); err != nil {
			return nil, err
		}
		for _, receipt := range records.receipts {
			leftovers = append(leftovers, receipt.File)
		}
	}
	if len(leftovers) == 0 {
		return journal, nil
	}
	for _, file := range leftovers {
		if err := w.remove(filepath.Join(TurnsPath(dir, name), file)); err != nil {
			return journal, err
		}
	}
	return journal, w.syncDir(TurnsPath(dir, name))
}

// holds says the journal converted this receipt as it reads now: its file,
// with what the earlier build wrote in it unchanged.
func (c *conversionJournal) holds(receipt convertedReceipt) bool {
	found, err := json.Marshal(receipt)
	if err != nil {
		return false
	}
	for _, converted := range c.Receipts {
		if converted.File != receipt.File {
			continue
		}
		kept, err := json.Marshal(converted)
		return err == nil && bytes.Equal(found, kept)
	}
	return false
}

// finishConversion decides and completes the conversion journal, or stops
// the mailbox on the reports nothing can decide.
func (w world) finishConversion(ctx context.Context, name string, journal *conversionJournal) error {
	if journal.Done {
		return nil
	}
	dir := w.dir
	save := func() error { return journal.saveIn(w, name) }
	if journal.Steps == nil {
		steps, unknown, err := journal.decide(w, name)
		if err != nil {
			return err
		}
		if len(unknown) > 0 {
			journal.Unknown = unknown
			if err := save(); err != nil {
				return err
			}
			// Telling main is an effect like any other: a note whose mark is
			// unknown stops the mailbox before the decision is recorded.
			stopped := journal.stopped(dir, name)
			if err := w.tellMain(name, "stopped\x00"+conversionPath(dir, name), stopped.Error()); err != nil {
				return err
			}
			return stopped
		}
		journal.Unknown, journal.Steps = nil, steps
		if err := save(); err != nil {
			return err
		}
	}
	complete := true
	for i := range journal.Steps {
		done, err := w.completeJournal(ctx, name, &journal.Steps[i], save)
		if err != nil {
			return err
		}
		complete = complete && done
	}
	if !complete {
		return nil
	}
	journal.Done = true
	return save()
}

// reportRef is one report of the conversion journal and the receipt it is in.
type reportRef struct {
	receipt convertedReceipt
	report  Message
}

// reports lists every report the earlier build prepared. A receipt not
// prepared sent nothing and cleared nothing.
func (c *conversionJournal) reports() []reportRef {
	var refs []reportRef
	for _, receipt := range c.Receipts {
		if !receipt.Prepared {
			continue
		}
		for _, report := range receipt.Reports {
			refs = append(refs, reportRef{receipt: receipt, report: report})
		}
	}
	return refs
}

// closes says a report answers its obligations: a finished or failed report
// of an end that did not keep its waits, answering tasks or questions.
func (r reportRef) closes() bool {
	return !r.receipt.KeepWaiters && (r.report.Kind == Finished || r.report.Kind == Error) && len(r.report.InReplyTo) > 0
}

// obligations names what a report answers: each message of one sender's run.
func (r reportRef) obligations() []string {
	var keys []string
	for _, id := range r.report.InReplyTo {
		keys = append(keys, r.report.To+"\x00"+r.report.ToEpoch+"\x00"+id)
	}
	return keys
}
