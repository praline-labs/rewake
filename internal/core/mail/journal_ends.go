package mail

import (
	"errors"
	"os"
	"path/filepath"
)

// The ends of a run on record: a pending mark looks, under the mailbox lock,
// for an end of the run whose journal records it ended at or after its own
// time. A journal is kept while its run lives, and one that cannot be read may
// be this run's: it is an error, never an absence.

// EndedSince says whether a journal of run epoch records an end at or after
// at.
func EndedSince(dir, name, epoch string, at int64) (bool, error) {
	latest, err := LatestEnd(dir, name, epoch)
	return latest > 0 && latest >= at, err
}

// LatestEnd is the latest Ended a journal of run epoch records; zero when
// none does. A journal is retired to its done name under the mailbox lock; a
// caller without it may see one leave between the listing and the read, and
// then lists again.
func LatestEnd(dir, name, epoch string) (int64, error) {
	for range 3 {
		latest, moved, err := latestEnd(dir, name, epoch)
		if !moved {
			return latest, err
		}
	}
	return 0, errors.New("the turn journals kept moving while they were read")
}

func latestEnd(dir, name, epoch string) (int64, bool, error) {
	entries, err := os.ReadDir(JournalPath(dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	latest := int64(0)
	for _, entry := range entries {
		if entry.IsDir() || entry.Name()[0] == '.' {
			continue
		}
		journal, err := readJournalFile(filepath.Join(JournalPath(dir, name), entry.Name()))
		if errors.Is(err, os.ErrNotExist) {
			return 0, true, nil
		}
		if err != nil {
			return 0, false, err
		}
		if journal.Epoch == epoch {
			latest = max(latest, journal.Ended)
		}
	}
	return latest, false, nil
}
