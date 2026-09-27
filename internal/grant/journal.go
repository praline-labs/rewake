package grant

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// Outcomes of an entry. A grant is written once its roots reached the
// harness; it ends revoked when rewake took it back, or dropped when rewake
// found it gone already — a person's own turn in the terminal replaces the
// roots with the launch's.
//
// One more belongs to Claude Code, whose grant only its permission hook can
// take back: revoking is a grant whose task is settled, waiting for the hook
// to remove the directory at the session's next tool call.
const (
	Granted  = "granted"
	Revoked  = "revoked"
	Dropped  = "dropped"
	Revoking = "revoking"
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
	// Thread is the conversation the grant was delivered into, and From and
	// FromEpoch the main run that sent it: what a run resuming that
	// conversation asks to confirm the grant again (resume.go). Empty in a
	// copy from before they were kept, which restores nothing.
	Thread    string `json:"thread,omitempty"`
	From      string `json:"from,omitempty"`
	FromEpoch string `json:"fromEpoch,omitempty"`
}

// Live says whether rewake still counts the entry as granted.
func (e Entry) Live() bool { return e.Outcome == Granted }

// Revoking says whether the entry waits for the harness to take it back.
func (e Entry) Revoking() bool { return e.Outcome == Revoking }

// maxEnded bounds the ended entries a journal keeps, the oldest going first.
// Live ones are never dropped: an entry is how rewake finds a grant to take
// back, and one pushed out would stay granted for good.
const maxEnded = 64

// MaxLive bounds the directories granted to one run at once. A grant past it
// is refused, never an earlier one forgotten.
const MaxLive = 64

func journalPath(dir, name, epoch string) string {
	sum := sha256.Sum256([]byte(name + "\x00" + epoch))
	return filepath.Join(dir, "grants", fmt.Sprintf("%x.json", sum[:]))
}

// Load reads the journal of one run of a session, empty when there is none.
func Load(dir, name, epoch string) []Entry {
	if !state.ValidName(name) || epoch == "" {
		return nil
	}
	return loadFile(journalPath(dir, name, epoch))
}

func loadFile(path string) []Entry {
	raw, err := os.ReadFile(path)
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
	ended := 0
	for _, entry := range entries {
		if !entry.Live() {
			ended++
		}
	}
	kept := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		if !entry.Live() && ended > maxEnded {
			ended--
			continue
		}
		kept = append(kept, entry)
	}
	entries = kept
	raw, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	path := journalPath(dir, name, epoch)
	if err := state.EnsureSubdir(filepath.Dir(path)); err != nil {
		return err
	}
	if err := state.WriteAtomic(path, append(raw, '\n')); err != nil {
		return err
	}
	sweep(dir)
	return nil
}

// sweepGrace keeps a journal a while after its run is gone from the
// registry: a run is registered before anything is granted to it, so only a
// record being rewritten could hide a live one, and not for this long.
const sweepGrace = time.Minute

// sweep removes the journals of runs that have ended. A journal is what
// rewake list shows, and what a run resuming the conversation is pointed to;
// what a run revokes by is held by its wrapper
// (docs/grants.md#taking-a-grant-back). So an ended run's copy stays only while
// it names a grant the main that sent it is still there to confirm again.
func sweep(dir string) {
	files, err := os.ReadDir(filepath.Join(dir, "grants"))
	if err != nil {
		return
	}
	sessions, err := registry.ListReadOnly(dir)
	if err != nil {
		return
	}
	live := map[string]bool{}
	for _, session := range sessions {
		live[filepath.Base(journalPath(dir, session.Name, session.Epoch()))] = true
	}
	for _, file := range files {
		if live[file.Name()] || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}
		if restorable(loadFile(filepath.Join(dir, "grants", file.Name()))) {
			continue
		}
		if info, err := file.Info(); err == nil && time.Since(info.ModTime()) > sweepGrace {
			_ = os.Remove(filepath.Join(dir, "grants", file.Name()))
		}
	}
}
