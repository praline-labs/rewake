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
type keptRecord struct {
	Epoch   string `json:"epoch"`
	Text    string `json:"text"`
	Version string `json:"version,omitempty"`
	Seq     uint64 `json:"seq,omitempty"`
}

func keptPath(dir, name string) string { return filepath.Join(pendingDir(dir, name), "kept.json") }

// pendingDir holds the turn-end records apart from the messages: everything
// directly in a mailbox is mail.
func pendingDir(dir, name string) string { return filepath.Join(state.InboxPath(dir, name), "pending") }

// readKept answers the kept answer when it is this run's. One of another run
// — an ended one, or an earlier build's — is none: it is never taken
// (docs/turn-end-recovery.md#the-cutover). One that cannot be read is an
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
// position of the run's read clock. The caller holds the mailbox lock. A kept
// answer that cannot be read is never written over: it may be one no end has
// published yet.
func KeepAnswer(dir, name, epoch, text string) error {
	if _, _, err := readKept(dir, name, epoch); err != nil {
		return err
	}
	if err := state.EnsureSubdir(pendingDir(dir, name)); err != nil {
		return err
	}
	return onReadClock(dir, name, epoch, func(position uint64) error {
		raw, err := json.Marshal(keptRecord{Epoch: epoch, Text: text, Version: NewID(), Seq: position})
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

// KeptAnswerThrough answers this run's kept answer that a turn end takes: for
// an end with a read boundary, only one kept at or below through; for an end
// heard once (through nil), whatever it finds, since nothing can be kept
// between its reading and its journal, which share the lock. The version is
// what the end's journal names to take it.
func KeptAnswerThrough(dir, name, epoch string, through *uint64) (string, string, bool, error) {
	record, ok, err := readKept(dir, name, epoch)
	if err != nil || !ok {
		return "", "", false, err
	}
	if through != nil && (record.Seq == 0 || record.Seq > *through) {
		// Kept after the boundary was captured: a later end's.
		return "", "", false, nil
	}
	return record.Text, record.Version, true, nil
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
