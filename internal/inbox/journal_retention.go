package inbox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/state"
)

// sweepTurnRecords keeps the done journals of the live run for as long as it
// lives, and removes those of ended runs (docs/turn-end-recovery.md): a retry
// of a turn end comes only from the run that heard it, and however late it
// comes it must find its end completed, and the next end's window must open
// after it. A live run's journal is looked at again a day later, not every
// sweep. An unfinished journal is an obligation and stays, whoever's.
func sweepTurnRecords(dir, name, live string, cutoff time.Time) {
	now := time.Now()
	directory := JournalPath(dir, name)
	entries, err := os.ReadDir(directory)
	if err != nil {
		return
	}
	kept := keptByStopOf(dir, name)
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || entry.IsDir() || info.ModTime().After(cutoff) || kept.keeps(filepath.Join(directory, entry.Name())) {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		switch {
		case strings.HasPrefix(entry.Name(), "."):
			// A write that never finished.
			_ = state.Remove(path)
		case !strings.HasSuffix(entry.Name(), doneSuffix):
		default:
			epoch, ok := recordEpoch(path)
			switch {
			case !ok:
				// Nothing is removed on a guess.
			case epoch == live:
				_ = os.Chtimes(path, now, now)
			default:
				_ = state.Remove(path)
			}
		}
	}
}

// recordEpoch reads the run a journal belongs to.
func recordEpoch(path string) (string, bool) {
	raw, err := state.ReadFile(path)
	if err != nil {
		return "", false
	}
	var record struct{ Epoch string }
	if json.Unmarshal(raw, &record) != nil || record.Epoch == "" {
		return "", false
	}
	return record.Epoch, true
}
