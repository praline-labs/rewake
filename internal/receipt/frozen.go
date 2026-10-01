package receipt

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/state"
)

// ReadBatch is an explicit read, frozen by its first call: which letters, in
// which version, cut into which parts. Letters that arrive later never join
// it, and a letter counts as read only once every part of its version was
// shown whole.
type ReadBatch struct {
	JSON    bool     `json:"json"`
	Letters []Letter `json:"letters"`
}

// Letter is one member of a frozen read.
type Letter struct {
	ID string `json:"id"`
	// Version is the digest of the stored letter it was frozen from; a
	// withdrawal before its first part was shown gives it a new one.
	Version string `json:"version"`
	// Message is the stored letter as frozen, which reading it marks.
	Message json.RawMessage `json:"message"`
	// Head is what the letter shows once, before its first part.
	Head string `json:"head,omitempty"`
	// Body is what is cut into parts.
	Body  string `json:"body"`
	Parts []Part `json:"parts"`
	// Read is set once the letter was marked read.
	Read bool `json:"read,omitempty"`
}

// Started says whether any part of the letter reached a call.
func (l Letter) Started() bool {
	for _, part := range l.Parts {
		if len(part.Shown) > 0 {
			return true
		}
	}
	return false
}

// Complete says whether every part was acknowledged.
func (l Letter) Complete() bool {
	for _, part := range l.Parts {
		if !part.Acked {
			return false
		}
	}
	return len(l.Parts) > 0
}

// Part is a byte range of a body.
type Part struct {
	Start int     `json:"start"`
	End   int     `json:"end"`
	Shown []Shown `json:"shown,omitempty"`
	Acked bool    `json:"acked,omitempty"`
}

// Shown is one call that carried a part.
type Shown struct {
	CallID     string `json:"callId,omitempty"`
	Transport  string `json:"transport"`
	CalledBoot int64  `json:"calledBoot,omitempty"`
	// Answer is the digest of the whole text the call printed
	// (bridge.AnswerDigest), recorded before it was printed: the evidence
	// of the part is a result with exactly that text.
	Answer string `json:"answer,omitempty"`
}

// FrozenText is an output too long for one result, kept so its parts can be
// fetched one call at a time.
type FrozenText struct {
	JSON  bool   `json:"json"`
	Text  string `json:"text"`
	Parts []Part `json:"parts"`
}

// Sweep removes what no call will come back for. In the live run that is a
// finished record older than cutoff whose effect is known and whose read, if
// it is one, left nothing in progress; unread says whether a letter is still
// unread, which a letter shown in part and read elsewhere no longer is. An open
// record stays however old: it is what a recovery reads. Another run's journal
// goes once its last change is older than cutoff: a receipt reaches only its
// own run's records, so nothing there can still be continued. That is the
// clean-up of a journal nobody can reach, not proof that its effects are
// absent; and a journal whose age cannot be read is not taken for an old one.
//
// Each record goes under its own lock, and one another call holds is passed
// over: removing a lock file somebody holds lets a second call take the same
// operation.
func Sweep(dir, name, live string, cutoff time.Time, unread func(id string) bool) {
	root := filepath.Join(state.InboxPath(dir, name), "receipts")
	runs, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, run := range runs {
		journal := filepath.Join(root, run.Name())
		if run.Name() == live {
			sweepRun(dir, name, run.Name(), journal, func(record Record) bool {
				return record.Phase == Done && !record.Uncertain && !record.Updated.After(cutoff) &&
					(record.Read == nil || readSettled(*record.Read, unread))
			})
			continue
		}
		if latest, known := newest(journal); known && latest.Before(cutoff) {
			sweepRun(dir, name, run.Name(), journal, func(Record) bool { return true })
			// Empty now unless a record was held; then it goes next time.
			sweepBindings(journal, nil)
			_ = state.Remove(filepath.Join(journal, callsDir))
			_ = state.Remove(journal)
		}
	}
}

func sweepRun(dir, name, epoch, journal string, removable func(Record) bool) {
	entries, err := os.ReadDir(journal)
	if err != nil {
		return
	}
	now, cancel := context.WithCancel(context.Background())
	cancel()
	kept := map[string]bool{}
	for _, entry := range entries {
		token, ok := strings.CutSuffix(entry.Name(), ".json")
		if !ok || !ValidToken(token) {
			continue
		}
		kept[token] = true
		record, err := Load(dir, name, epoch, token)
		if err != nil || !removable(record) {
			continue
		}
		release, err := Lock(now, dir, name, epoch, token)
		if err != nil {
			continue
		}
		// Decided again under the lock: a call may have changed it since.
		if record, err := Load(dir, name, epoch, token); err == nil && removable(record) {
			_ = state.Remove(recordPath(journal, token))
			_ = state.Remove(lockPath(journal, token))
			delete(kept, token)
		}
		release()
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "key-") {
			raw, err := state.ReadFile(filepath.Join(journal, entry.Name()))
			if err == nil && !kept[strings.TrimSpace(string(raw))] {
				_ = state.Remove(filepath.Join(journal, entry.Name()))
			}
		}
		// A lock file without its record is left by a call that lost the
		// race to journal; it goes under its own lock like any other.
		token, ok := strings.CutSuffix(strings.TrimPrefix(entry.Name(), "."), ".lock")
		if ok && ValidToken(token) && !kept[token] {
			if release, err := Lock(now, dir, name, epoch, token); err == nil {
				if _, err := os.Stat(recordPath(journal, token)); errors.Is(err, os.ErrNotExist) {
					_ = state.Remove(lockPath(journal, token))
				}
				release()
			}
		}
	}
	sweepBindings(journal, kept)
}

// readSettled says a frozen read left nothing in progress: every letter whose
// first part was shown was read to the end, through this read or another.
func readSettled(batch ReadBatch, unread func(id string) bool) bool {
	for _, letter := range batch.Letters {
		if letter.Started() && !letter.Read && unread(letter.ID) {
			return false
		}
	}
	return true
}

// newest is the latest change among a journal's files, and whether every one
// of them could be looked at: a file that could not may be the newest.
func newest(journal string) (time.Time, bool) {
	var latest time.Time
	entries, err := os.ReadDir(journal)
	if err != nil {
		return latest, false
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return latest, false
		}
		if info.ModTime().After(latest) {
			latest = info.ModTime()
		}
	}
	return latest, true
}
