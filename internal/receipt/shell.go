package receipt

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/praline-labs/rewake/internal/state"
)

// The shell observations of a run (docs/mail-bridge-channel.md#the-shell-observation):
// one small file per mail operation that wrote under a lock, or failed reaching
// the state, under the run's receipts. The CLI writes them; the run's wrapper
// folds and removes them. A file per observation, named by its boot-clock time
// and pid, so concurrent commands never overwrite one another.

// The shell failure classes, by what reaching the state met. They match the
// channel record's.
const (
	ShellReadOnly   = "state read-only"
	ShellNotAllowed = "state not permitted"
	ShellIOError    = "state I/O error"
)

// ShellNote is one shell observation.
type ShellNote struct {
	OK    bool      `json:"ok"`
	Class string    `json:"class,omitempty"`
	Boot  int64     `json:"boot"`
	Wall  time.Time `json:"wall"`
}

// maxShellNotes bounds what one fold takes: the newest decides, and a
// directory that grew past this while no wrapper read it is cut to it.
const maxShellNotes = 64

func shellDir(dir, name, epoch string) (string, error) {
	journal, err := Path(dir, name, epoch)
	if err != nil {
		return "", err
	}
	return filepath.Join(journal, "channel"), nil
}

// ShellClass is the class of an error that reaching the state met, or ""
// for one that is not about reaching it: a refusal, a timeout, a hold.
func ShellClass(err error) string {
	switch {
	case !state.Unreachable(err):
		return ""
	case errors.Is(err, syscall.EROFS):
		return ShellReadOnly
	case errors.Is(err, syscall.EACCES), errors.Is(err, syscall.EPERM):
		return ShellNotAllowed
	}
	return ShellIOError
}

// WriteShell records one shell observation of a run. It writes what it can
// and answers nothing: a state that cannot be reached loses the observation,
// and the display then says the shell is unconfirmed, which is true.
func WriteShell(dir, name, epoch string, note ShellNote) {
	path, err := shellDir(dir, name, epoch)
	if err != nil {
		return
	}
	for _, each := range []string{filepath.Dir(filepath.Dir(path)), filepath.Dir(path), path} {
		if state.EnsureSubdir(each) != nil {
			return
		}
	}
	encoded, err := json.Marshal(note)
	if err != nil {
		return
	}
	_ = state.WriteAtomic(filepath.Join(path, fmt.Sprintf("%d.%d", note.Boot, os.Getpid())), encoded)
}

// TakeShell reads a run's shell observations, oldest first, and removes
// those it read.
func TakeShell(dir, name, epoch string) []ShellNote {
	path, err := shellDir(dir, name, epoch)
	if err != nil {
		return nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil
	}
	sort.Slice(entries, func(i, j int) bool { return bootOf(entries[i].Name()) < bootOf(entries[j].Name()) })
	if len(entries) > maxShellNotes {
		for _, stale := range entries[:len(entries)-maxShellNotes] {
			_ = state.Remove(filepath.Join(path, stale.Name()))
		}
		entries = entries[len(entries)-maxShellNotes:]
	}
	var notes []ShellNote
	for _, entry := range entries {
		file := filepath.Join(path, entry.Name())
		if !entry.Type().IsRegular() || bootOf(entry.Name()) == 0 {
			continue
		}
		raw, err := state.ReadFile(file)
		var note ShellNote
		if err == nil && len(raw) < 1024 && json.Unmarshal(raw, &note) == nil && note.Boot > 0 {
			notes = append(notes, note)
		}
		_ = state.Remove(file)
	}
	return notes
}

// bootOf is the boot-clock time a file is named by, 0 for a name that is
// not one of these files.
func bootOf(name string) int64 {
	boot, _, found := strings.Cut(name, ".")
	if !found {
		return 0
	}
	value, err := strconv.ParseInt(boot, 10, 64)
	if err != nil || value <= 0 {
		return 0
	}
	return value
}
