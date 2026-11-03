package mail

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// reportedSuffix names the receipt of a turn end that reported to a waiter
// (docs/delivery-turn-end.md#the-order-of-the-effects).
const reportedSuffix = ".reported"

// unclaimedAnswer is the notify that carries a worker's finished answer to
// main when no waiter took it (docs/turn-outcomes.md#unclaimed-answers): the
// final message of a worker whose task an earlier end settled would otherwise
// reach nobody. A retry of an end whose ordinary report was written and whose
// wait was cleared sees no waiter, and is told by the receipt that its answer
// went out already. There is none without a live main, nor for main's own
// ends, which report to nobody.
func unclaimedAnswer(dir string, self registry.Session, event TurnEnd, pending bool, op string) (*Message, error) {
	if event.Failed || event.Stopped || pending || !OwesReports(self.Role) || role.Of(self.Role).ID == role.Main.ID {
		return nil, nil
	}
	text := strings.TrimSpace(event.Text)
	if text == "" {
		return nil, nil
	}
	if _, err := os.Stat(filepath.Join(JournalPath(dir, self.Name), op+reportedSuffix)); !errors.Is(err, os.ErrNotExist) {
		// One that cannot be looked at may say it was reported: a second
		// copy of the answer is worse than a retry.
		return nil, err
	}
	sessions, err := registry.List(dir)
	if err != nil {
		return nil, err
	}
	for _, main := range sessions {
		if role.Of(main.Role).ID != role.Main.ID || main.Name == self.Name {
			continue
		}
		id, err := reportOfOp(dir, main.Name, op)
		if err != nil {
			return nil, err
		}
		return &Message{
			ID: id, From: self.Name, FromEpoch: self.Epoch(), To: main.Name, ToEpoch: main.Epoch(), Kind: Note,
			Text:      "Unclaimed answer: " + self.Name + "'s turn ended with no task or question waiting on it.\n\n" + text,
			CreatedAt: time.Now(),
		}, nil
	}
	return nil, nil
}

// writeReportedReceipt records that the end op reported to a waiter, before
// the waits it answered are cleared: a retry past the clearing finds no
// waiter, and must not take its answer for an unclaimed one.
func writeReportedReceipt(dir string, self registry.Session, op string) error {
	if err := state.EnsureSubdir(JournalPath(dir, self.Name)); err != nil {
		return err
	}
	return writeJournalFile(filepath.Join(JournalPath(dir, self.Name), op+reportedSuffix), TurnJournal{Epoch: self.Epoch(), Op: op})
}
