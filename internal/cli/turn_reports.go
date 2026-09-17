package cli

import (
	"errors"
	"strings"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/role"
)

// The mailbox lock serializes preparation and clearing. Persist the entire
// batch before its first publication so retries cannot absorb later work.
func publishTurn(dir string, self registry.Session, event turnResult, currentThread string, waiters []inbox.Waiter) error {
	receipt, path, err := loadTurnReceipt(dir, self, event)
	if err != nil {
		return err
	}
	if !receipt.Done {
		if !receipt.Prepared {
			receipt.Reports, receipt.Waiters, err = prepareTurnReports(dir, self, event, currentThread, waiters, receipt.ID)
			if err != nil {
				return err
			}
			receipt.Prepared = true
			receipt.KeepWaiters = event.Stopped
			if err := saveTurnReceipt(path, receipt); err != nil {
				return err
			}
		}
		for _, report := range receipt.Reports {
			peer, err := registry.Lookup(dir, report.To)
			if err != nil && !errors.Is(err, registry.ErrNotFound) {
				return err
			}
			if err != nil || peer.Epoch() != report.ToEpoch {
				continue
			}
			if report.To == self.Name && len(report.InReplyTo) == 0 {
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
		if err := saveTurnReceipt(path, receipt); err != nil {
			return err
		}
	}
	if receipt.KeepWaiters {
		return nil
	}
	for _, waiter := range receipt.Waiters {
		inbox.ClearAwaiting(dir, self.Name, self.Epoch(), waiter)
	}
	return nil
}

func prepareTurnReports(dir string, self registry.Session, event turnResult, currentThread string, waiters []inbox.Waiter, fallbackID string) ([]inbox.Message, []inbox.Waiter, error) {
	if !event.Failed && !event.Stopped && strings.TrimSpace(event.Text) == "" {
		if len(waiters) == 0 {
			return nil, nil, nil
		}
		event.Failed = true
		event.Text = ""
	}
	if !event.Failed && !event.Stopped && (role.Of(self.Role).Silent || len(waiters) == 0) {
		return nil, nil, nil
	}
	kind := inbox.Finished
	if event.Stopped {
		kind = inbox.Stopped
	} else if event.Failed {
		kind = errorKind.kind
	}
	var reports []inbox.Message
	for _, waiter := range waiters {
		peer, err := registry.Lookup(dir, waiter.Name)
		if err != nil && !errors.Is(err, registry.ErrNotFound) {
			return nil, nil, err
		}
		if err != nil || peer.Epoch() != waiter.Epoch {
			continue
		}
		id := inbox.ReportID(self.Name, self.Epoch(), waiter)
		if event.Stopped {
			id += "-stopped-" + fallbackID
		}
		reports = append(reports, inbox.Message{ID: id, From: self.Name, FromEpoch: self.Epoch(), To: peer.Name, ToEpoch: peer.Epoch(), Kind: kind, Text: event.Text, InReplyTo: waiter.Messages, CreatedAt: time.Now(), ThreadChanged: inbox.ReportThreadChanged(dir, self.Name, waiter.Messages, currentThread)})
	}
	if (event.Failed || event.Stopped) && len(reports) == 0 {
		target := self
		if role.Of(self.Role).ID != role.Main.ID {
			sessions, err := registry.List(dir)
			if err != nil {
				return nil, nil, err
			}
			for _, candidate := range sessions {
				if role.Of(candidate.Role).ID == role.Main.ID {
					target = candidate
					break
				}
			}
		}
		reports = append(reports, inbox.Message{ID: fallbackID, From: self.Name, FromEpoch: self.Epoch(), To: target.Name, ToEpoch: target.Epoch(), Kind: kind, Text: event.Text, CreatedAt: time.Now()})
	}
	return reports, waiters, nil
}
