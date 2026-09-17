package cli

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/role"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

type turnReceipt struct {
	ID      string
	Done    bool
	Waiters []inbox.Waiter
}

func loadTurnReceipt(dir string, self registry.Session, event turnResult) (turnReceipt, string, error) {
	receipt := turnReceipt{ID: inbox.NewID()}
	if event.ID == "" {
		return receipt, "", nil
	}
	sum := sha256.Sum256([]byte(self.Epoch() + "\x00" + event.ID))
	directory := filepath.Join(state.InboxPath(dir, self.Name), "turns")
	if err := state.EnsureSubdir(directory); err != nil {
		return receipt, "", err
	}
	path := filepath.Join(directory, fmt.Sprintf("%x", sum[:16]))
	raw, err := os.ReadFile(path)
	if err == nil {
		err = json.Unmarshal(raw, &receipt)
	} else if os.IsNotExist(err) {
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

// The caller holds its own mailbox lock. Keep the waits until every output is
// published; retries use the same report ids. Identified turns also suppress a
// duplicate fallback to main after their original waits have been cleared.
func publishTurn(dir string, self registry.Session, event turnResult, currentThread string, waiters []inbox.Waiter) error {
	if !event.Failed && strings.TrimSpace(event.Text) == "" {
		if len(waiters) == 0 {
			return nil
		}
		event.Failed = true
		event.Text = ""
	}
	if !event.Failed && (role.Of(self.Role).Silent || len(waiters) == 0) {
		return nil
	}
	receipt, path, err := loadTurnReceipt(dir, self, event)
	if err != nil {
		return err
	}
	if receipt.Done {
		for _, w := range receipt.Waiters {
			inbox.ClearAwaiting(dir, self.Name, self.Epoch(), w)
		}
		return nil
	}
	kind := inbox.Finished
	if event.Failed {
		kind = errorKind.kind
	}
	recipients := 0
	for _, waiter := range waiters {
		peer, err := registry.Lookup(dir, waiter.Name)
		if err != nil && !errors.Is(err, registry.ErrNotFound) {
			return err
		}
		if err != nil || peer.Epoch() != waiter.Epoch {
			continue
		}
		report := inbox.Message{ID: inbox.ReportID(self.Name, self.Epoch(), waiter), From: self.Name, FromEpoch: self.Epoch(), To: peer.Name, ToEpoch: peer.Epoch(), Kind: kind, Text: event.Text, InReplyTo: waiter.Messages, CreatedAt: time.Now(), ThreadChanged: inbox.ReportThreadChanged(dir, self.Name, waiter.Messages, currentThread)}
		if err := inbox.PutOnce(dir, report); err != nil {
			return err
		}
		recipients++
	}
	if event.Failed && recipients == 0 {
		target := self
		if role.Of(self.Role).ID != role.Main.ID {
			sessions, err := registry.List(dir)
			if err != nil {
				return err
			}
			for _, candidate := range sessions {
				if role.Of(candidate.Role).ID == role.Main.ID {
					target = candidate
					break
				}
			}
		}
		report := inbox.Message{ID: receipt.ID, From: self.Name, FromEpoch: self.Epoch(), To: target.Name, ToEpoch: target.Epoch(), Kind: kind, Text: event.Text, CreatedAt: time.Now()}
		if target.Name == self.Name {
			// A failed main must not wake itself into another failing turn.
			err = inbox.PutLocal(dir, report)
		} else {
			err = inbox.PutOnce(dir, report)
		}
		if err != nil {
			return err
		}
	}
	receipt.Done = true
	receipt.Waiters = waiters
	if err := saveTurnReceipt(path, receipt); err != nil {
		return err
	}
	for _, waiter := range waiters {
		inbox.ClearAwaiting(dir, self.Name, self.Epoch(), waiter)
	}
	return nil
}
