package spacehost

import (
	"github.com/praline-labs/rewake/internal/core/space"
	"github.com/praline-labs/rewake/internal/host/worktree"
)

// claimedCheckout finds the worktree the worker's launch made, once its
// session claimed it: the record names the worker's run as its owner, in
// main's room.
func claimedCheckout(dir, workerRun string) (space.Checkout, bool) {
	record, ok := claimed(dir, workerRun)
	if !ok {
		return space.Checkout{}, false
	}
	return space.Checkout{Path: record.Path, Branch: record.Branch}, true
}

func claimed(dir, workerRun string) (worktree.Record, bool) {
	root, err := worktree.Root()
	if err != nil {
		return worktree.Record{}, false
	}
	records, err := worktree.List(root)
	if err != nil {
		return worktree.Record{}, false
	}
	for _, record := range records {
		if record.Session != nil && record.Session.Epoch == workerRun && record.Session.Dir == dir {
			return record, true
		}
	}
	return worktree.Record{}, false
}

// seen is the checkout as the close finds it, once the worker is gone. A
// worker without a checkout of its own left none: gone, as never made.
func seen(dir string, r space.Record) space.Seen {
	if r.Checkout.Path == "" {
		return space.Seen{Presence: space.Gone}
	}
	presence := space.CheckoutPresence(r)
	if presence != space.There {
		return space.Seen{Presence: presence}
	}
	found := space.Seen{Presence: space.There}
	if record, ok := claimed(dir, r.Worker.Run); ok {
		if check, err := worktree.Inspect(record); err == nil {
			found.Changes, found.Commits, found.Locked = check.Changes, check.Head != record.Commit, check.Locked
		}
	}
	return found
}
