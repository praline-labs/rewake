package inbox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/state"
)

// TurnsPath holds the receipts the earlier build wrote for its turn ends.
// This build writes none, and never sweeps them by age: each is met by the
// conversion of the name's next run.
//
// legacy(rewake <2026-09-30): the earlier build kept a receipt per turn end here, which the barrier converts (conversion.go); remove when no session started by an earlier build is registered
func TurnsPath(dir, name string) string {
	return filepath.Join(state.InboxPath(dir, name), "turns")
}

// sweepTurnRecords keeps the done journals of the live run for as long as it
// lives, and removes those of ended runs (docs/turn-end-recovery.md): a retry
// of a turn end comes only from the run that heard it, and however late it
// comes it must find its end completed, and the next end's window must open
// after it. A live run's journal is looked at again a day later, not every
// sweep. An unfinished journal is an obligation and stays, whoever's; the
// conversion journal holds a person's decisions and stays for good.
func sweepTurnRecords(dir, name, live string, cutoff time.Time) {
	now := time.Now()
	directory := JournalPath(dir, name)
	entries, err := os.ReadDir(directory)
	if err != nil {
		return
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || entry.IsDir() || info.ModTime().After(cutoff) {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		switch {
		case strings.HasPrefix(entry.Name(), "."):
			// A write that never finished.
			_ = os.Remove(path)
		case !strings.HasSuffix(entry.Name(), doneSuffix):
		default:
			epoch, ok := recordEpoch(path)
			switch {
			case !ok:
				// Nothing is removed on a guess.
			case epoch == live:
				_ = os.Chtimes(path, now, now)
			default:
				_ = os.Remove(path)
			}
		}
	}
}

// recordEpoch reads the run a journal belongs to.
func recordEpoch(path string) (string, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	var record struct{ Epoch string }
	if json.Unmarshal(raw, &record) != nil || record.Epoch == "" {
		return "", false
	}
	return record.Epoch, true
}
