package cli

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

type turnReceipt struct {
	ID          string
	Done        bool
	Prepared    bool
	KeepWaiters bool
	// Interim marks a turn end that kept its waiters because it was marked
	// pending, not because it was stopped.
	Interim bool
	Waiters []inbox.Waiter
	Reports []inbox.Message
}

func loadTurnReceipt(dir string, self registry.Session, event turnResult) (turnReceipt, string, error) {
	receipt := turnReceipt{ID: inbox.NewID()}
	if event.ID == "" {
		return receipt, "", nil
	}
	key := self.Epoch() + "\x00" + event.ID
	if event.Stopped {
		key = "stopped\x00" + key
	}
	sum := sha256.Sum256([]byte(key))
	directory := filepath.Join(state.InboxPath(dir, self.Name), "turns")
	if err := state.EnsureSubdir(directory); err != nil {
		return receipt, "", err
	}
	path := filepath.Join(directory, fmt.Sprintf("%x", sum[:16]))
	raw, err := os.ReadFile(path)
	if err == nil {
		err = json.Unmarshal(raw, &receipt)
		if err == nil && !event.Stopped && receipt.KeepWaiters && !receipt.Interim {
			stopEvent := event
			stopEvent.Stopped = true
			_, _, loadErr := loadTurnReceipt(dir, self, stopEvent)
			if loadErr != nil {
				return receipt, path, loadErr
			}

			receipt = turnReceipt{ID: inbox.NewID()}
			err = saveTurnReceipt(path, receipt)
		}
	} else if os.IsNotExist(err) {
		// legacy(rewake <2026-09-19): earlier builds kept a turn's receipt under a hash of the run and the event id alone; remove when no session started by such a build is registered
		if event.Stopped {
			legacySum := sha256.Sum256([]byte(self.Epoch() + "\x00" + event.ID))
			legacyPath := filepath.Join(directory, fmt.Sprintf("%x", legacySum[:16]))
			if previous, readErr := os.ReadFile(legacyPath); readErr == nil {
				var old turnReceipt
				if err := json.Unmarshal(previous, &old); err != nil {
					return receipt, path, err
				}
				if old.KeepWaiters {
					receipt = old
				}
			} else if !os.IsNotExist(readErr) {
				return receipt, path, readErr
			}
		}
		err = saveTurnReceipt(path, receipt)
	}
	return receipt, path, err
}

func saveTurnReceipt(path string, receipt turnReceipt) error {
	if path == "" {
		return nil
	}
	raw, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	return state.WriteAtomic(path, raw)
}
