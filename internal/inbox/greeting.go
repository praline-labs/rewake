package inbox

import (
	"os"
	"path/filepath"

	"github.com/iiiokojiadbi/rewake/internal/state"
)

// MarkGreeting records the automatic first turn before the child can finish it.
func MarkGreeting(dir, name, epoch string) error {
	if err := state.EnsureSubdir(state.InboxPath(dir, name)); err != nil {
		return err
	}
	return state.WriteAtomic(filepath.Join(state.InboxPath(dir, name), "greeting"), []byte(epoch))
}

// TakeGreeting consumes only this run's marker under the caller's mailbox lock.
func TakeGreeting(dir, name, epoch string) (bool, error) {
	path := filepath.Join(state.InboxPath(dir, name), "greeting")
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if string(raw) != epoch {
		return false, nil
	}
	if err := os.Remove(path); err != nil {
		return false, err
	}
	return true, nil
}
