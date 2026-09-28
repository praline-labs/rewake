package grant

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/praline-labs/rewake/internal/proc"
	"github.com/praline-labs/rewake/internal/registry"
)

// A journal follows the conversation its grants went into: each entry names
// the thread, and a run that resumes that thread cold finds there which
// grants it may have had (docs/grants.md#after-a-cold-resume). The copy is one
// a worker can write, so it is a hint and never proof: every grant it names is
// confirmed again by the main that sent it before anything is restored.

// Hint is what the copies say one message granted a conversation.
type Hint struct {
	Message   string
	From      string
	FromEpoch string
	// Paths are the directories the copies journaled for it, its Git
	// metadata included.
	Paths []string
}

// Hints lists, by message, the grants the copies in the room say were
// delivered into a conversation and are not over.
func Hints(dir, thread string) []Hint {
	if thread == "" {
		return nil
	}
	files, err := os.ReadDir(filepath.Join(dir, "grants"))
	if err != nil {
		return nil
	}
	var hints []Hint
	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".json") {
			continue
		}
		for _, entry := range loadFile(filepath.Join(dir, "grants", file.Name())) {
			if entry.Thread != thread || entry.Message == "" || !entry.Live() && !entry.Revoking() {
				continue
			}
			at := slices.IndexFunc(hints, func(hint Hint) bool { return hint.Message == entry.Message })
			if at < 0 {
				hints = append(hints, Hint{Message: entry.Message, From: entry.From, FromEpoch: entry.FromEpoch})
				at = len(hints) - 1
			}
			if !slices.Contains(hints[at].Paths, entry.Path) {
				hints[at].Paths = append(hints[at].Paths, entry.Path)
			}
		}
	}
	return hints
}

// restorable says whether a copy names a grant that could still be confirmed
// again: one not over, in a known conversation, from a main run still alive.
func restorable(entries []Entry) bool {
	for _, entry := range entries {
		if entry.Thread == "" || !entry.Live() && !entry.Revoking() {
			continue
		}
		if pid, start, ok := registry.ParseEpoch(entry.FromEpoch); ok && proc.Alive(pid, start) {
			return true
		}
	}
	return false
}
