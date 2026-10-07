package inbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/praline-labs/rewake/internal/state"
)

// After an interim turn end, rewake's Stop hook on Claude Code holds the next
// turn end that carries no mark, once, and asks the session whether the work
// is done (docs/turn-outcomes.md#the-confirmation-on-claude-code). Two records
// serve it, beside the pending marks and like them changed only under the
// mailbox lock: interim.json (interim.go), which makes an unmarked end worth
// asking about, and kept.json, which keeps the answer of the turn end that was
// held until the next turn end heard takes it into its own outcome: the
// continuation's Stop, a StopFailure, or the plugin's stop after an Esc.
// Without it the report would be the continuation alone, since the harness's
// second Stop carries only what the model said after the hold.

// keptRecord is a held answer. A hold takes its place on the run's read clock
// as a read does (docs/turn-end-recovery.md#the-read-clock): Seq is that
// position, so an end with a read boundary takes the answer only when it was
// kept at or below the boundary, and Version tells one kept answer from the
// next, so the end that published one removes that one and never a later one.
// Held names the operation of the end that was held and Reason what it was
// answered, so the same end confirmed again — its answer lost — is answered
// the same and never taken for its own continuation
// (docs/turn-end-recovery.md#a-held-end-confirmed-again).
type keptRecord struct {
	Epoch   string `json:"epoch"`
	Text    string `json:"text"`
	Version string `json:"version,omitempty"`
	Seq     uint64 `json:"seq,omitempty"`
	Held    string `json:"held,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

func keptPath(dir, name string) string { return filepath.Join(pendingDir(dir, name), "kept.json") }

// pendingDir holds the turn-end records apart from the messages: everything
// directly in a mailbox is mail.
func pendingDir(dir, name string) string { return filepath.Join(state.InboxPath(dir, name), "pending") }

// readKept answers the kept answer when it is this run's. One of another run,
// which has ended, is none: it is never taken. One that cannot be read is an
// error, since it may be this run's, and nothing writes over it.
func readKept(dir, name, epoch string) (keptRecord, bool, error) {
	return live(dir).readKept(name, epoch)
}

func (w world) readKept(name, epoch string) (keptRecord, bool, error) {
	dir := w.dir
	raw, err := w.readFile(keptPath(dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return keptRecord{}, false, nil
	}
	if err != nil {
		return keptRecord{}, false, err
	}
	var record keptRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return keptRecord{}, false, unknownRecord(keptPath(dir, name), fmt.Errorf("the kept answer %s is not readable: %w", keptPath(dir, name), err))
	}
	if record.Epoch != epoch {
		return keptRecord{}, false, nil
	}
	return record, true, nil
}

// KeepAnswer keeps the answer of a turn end that was held, at the next
// position of the run's read clock, with the held end's operation and the
// reason it was answered, in one write. The caller holds the mailbox lock. A
// kept answer that cannot be read is never written over: it may be one no end
// has published yet.
func KeepAnswer(dir, name, epoch, text, held, reason string) error {
	if _, _, err := readKept(dir, name, epoch); err != nil {
		return err
	}
	if err := state.EnsureSubdir(pendingDir(dir, name)); err != nil {
		return err
	}
	return onReadClock(dir, name, epoch, func(position uint64) error {
		raw, err := json.Marshal(keptRecord{Epoch: epoch, Text: text, Version: NewID(), Seq: position, Held: held, Reason: reason})
		if err != nil {
			return err
		}
		return state.WriteAtomic(keptPath(dir, name), raw)
	})
}

// KeptAnswer answers this run's kept answer, and whether there is one.
func KeptAnswer(dir, name, epoch string) (string, bool, error) {
	record, ok, err := readKept(dir, name, epoch)
	return record.Text, ok, err
}

// KeptTaken is the kept answer a turn end takes: its text, the version the
// end's journal names to take it, and the operation of the end that was held.
type KeptTaken struct {
	Text, Version, Held string
}

// KeptAnswerThrough answers this run's kept answer that a turn end takes: for
// an end with a read boundary, only one kept at or below through; for an end
// heard once (through nil), whatever it finds, since nothing can be kept
// between its reading and its journal, which share the lock.
func KeptAnswerThrough(dir, name, epoch string, through *uint64) (KeptTaken, bool, error) {
	record, ok, err := readKept(dir, name, epoch)
	if err != nil || !ok {
		return KeptTaken{}, false, err
	}
	if through != nil && (record.Seq == 0 || record.Seq > *through) {
		// Kept after the boundary was captured: a later end's.
		return KeptTaken{}, false, nil
	}
	return KeptTaken{Text: record.Text, Version: record.Version, Held: record.Held}, true, nil
}

// HeldEnd answers, when op is the end this run's kept answer holds, the reason
// it was answered — the answer to the same end confirmed again. The caller
// holds the mailbox lock. A hold whose clock commit was lost to a crash
// leaves its position above the clock's word; it is raised here, before the
// answer, so the continuation's boundary takes the kept answer
// (docs/turn-end-recovery.md#the-read-clock).
func HeldEnd(dir, name, epoch, op string) (string, bool, error) {
	record, ok, err := readKept(dir, name, epoch)
	if err != nil || !ok || op == "" || record.Held != op {
		return "", false, err
	}
	if err := raiseReadClock(dir, name, epoch, record.Seq); err != nil {
		return "", false, err
	}
	return record.Reason, true, nil
}

// dropKeptVersion forgets the kept answer of run epoch once a turn end has
// published it, when it is still the version published. A later answer kept
// since is another turn end's to publish; one that cannot be read may be
// either, and is an error.
func (w world) dropKeptVersion(name, epoch, version string) error {
	record, ok, err := w.readKept(name, epoch)
	if err != nil || !ok || record.Version != version {
		return err
	}
	return w.remove(keptPath(w.dir, name))
}
