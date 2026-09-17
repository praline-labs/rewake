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

// GreetingPending checks this run's marker without consuming it. The caller
// can persist a completion receipt before removing the bootstrap state.
func GreetingPending(dir, name, epoch string) (bool, error) {
	raw, err := os.ReadFile(filepath.Join(state.InboxPath(dir, name), "greeting"))
	if os.IsNotExist(err) {
		return false, nil
	}
	return string(raw) == epoch, err
}

// TakeGreeting consumes only this run's marker under the caller's mailbox lock.
func TakeGreeting(dir, name, epoch string) (bool, error) {
	pending, err := GreetingPending(dir, name, epoch)
	if err != nil || !pending {
		return false, err
	}
	if err := os.Remove(filepath.Join(state.InboxPath(dir, name), "greeting")); err != nil {
		return false, err
	}
	return true, nil
}
