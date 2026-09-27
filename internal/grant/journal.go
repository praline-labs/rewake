package grant

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/state"
)

// Outcomes of an entry. A grant is written once its roots reached the
// harness; it ends revoked when rewake took it back, or dropped when rewake
// found it gone already — a person's own turn in the terminal replaces the
// roots with the launch's.
const (
	Granted = "granted"
	Revoked = "revoked"
	Dropped = "dropped"
)

// Entry is one directory rewake added to a session's writable roots.
type Entry struct {
	Path string `json:"path"`
	// For names the granted directory whose Git metadata this is, when the
	// task also carried --grant-git.
	For     string     `json:"for,omitempty"`
	Message string     `json:"message"`
	At      time.Time  `json:"at"`
	Outcome string     `json:"outcome"`
	EndedAt *time.Time `json:"endedAt,omitempty"`
}

// Live says whether rewake still counts the entry as granted.
func (e Entry) Live() bool { return e.Outcome == Granted }

// maxEntries bounds a journal; ended entries go first.
const maxEntries = 64

func journalPath(dir, name, epoch string) string {
	sum := sha256.Sum256([]byte(name + "\x00" + epoch))
	return filepath.Join(dir, "grants", fmt.Sprintf("%x.json", sum[:]))
}

// Load reads the journal of one run of a session, empty when there is none.
func Load(dir, name, epoch string) []Entry {
	if !state.ValidName(name) || epoch == "" {
		return nil
	}
	raw, err := os.ReadFile(journalPath(dir, name, epoch))
	if err != nil {
		return nil
	}
	var entries []Entry
	if json.Unmarshal(raw, &entries) != nil {
		return nil
	}
	return entries
}

// Save replaces the journal. Only the wrapper serving the session writes it,
// from its delivery path, one delivery at a time.
func Save(dir, name, epoch string, entries []Entry) error {
	if !state.ValidName(name) || epoch == "" {
		return fmt.Errorf("invalid session identity")
	}
	for len(entries) > maxEntries {
		dropped := false
		for i, entry := range entries {
			if !entry.Live() {
				entries = append(entries[:i:i], entries[i+1:]...)
				dropped = true
				break
			}
		}
		if !dropped {
			entries = entries[1:]
		}
	}
	raw, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	path := journalPath(dir, name, epoch)
	if err := state.EnsureSubdir(filepath.Dir(path)); err != nil {
		return err
	}
	return state.WriteAtomic(path, append(raw, '\n'))
}
