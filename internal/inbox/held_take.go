package inbox

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/praline-labs/rewake/internal/state"
)

// TakeHeld is the fifth step of a launch (docs/protocol-cutover.md#the-launch):
// the run that took name is ready, so every mailbox whose journals hold a
// report for name's earlier-build runs passes its barrier, under its own lock,
// and the barrier publishes the report to this run. A sender that cannot be
// locked or reconciled now keeps the report held; its next barrier takes it.
func TakeHeld(ctx context.Context, dir, name string) {
	senders, err := os.ReadDir(filepath.Dir(state.InboxPath(dir, name)))
	if err != nil {
		return
	}
	for _, sender := range senders {
		if sender.Name() == name || !sender.IsDir() || !holdsFor(dir, sender.Name(), name) {
			continue
		}
		_ = state.WithMailboxLock(ctx, dir, sender.Name(), func() error {
			return Reconcile(ctx, dir, sender.Name())
		})
	}
}

// holdsFor says an unfinished journal of sender holds a report for name.
func holdsFor(dir, sender, name string) bool {
	entries, err := os.ReadDir(JournalPath(dir, sender))
	if err != nil {
		return false
	}
	for _, entry := range entries {
		file := entry.Name()
		if entry.IsDir() || strings.HasPrefix(file, ".") || strings.HasSuffix(file, doneSuffix) {
			continue
		}
		var journals []TurnJournal
		if file == conversionFile {
			conversion, err := readConversion(dir, sender)
			if err != nil || conversion == nil || conversion.Done {
				continue
			}
			journals = conversion.Steps
		} else if journal, err := readJournalFile(filepath.Join(JournalPath(dir, sender), file)); err == nil {
			journals = []TurnJournal{journal}
		}
		for _, journal := range journals {
			if slices.ContainsFunc(journal.Reports, func(report Message) bool {
				return report.To == name && slices.Contains(journal.Held, report.ID)
			}) {
				return true
			}
		}
	}
	return false
}
