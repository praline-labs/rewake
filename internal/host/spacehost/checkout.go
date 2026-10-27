package spacehost

import (
	"strings"

	"github.com/praline-labs/rewake/internal/core/space"
	"github.com/praline-labs/rewake/internal/host/reviewcheckout"
	"github.com/praline-labs/rewake/internal/host/worktree"
)

// claimedCheckout finds the worker's worktree — its launch's, or the one it
// entered — once its session claimed it: the record names the worker's run as its owner, in
// main's room. Not found is an observation only when the records could be
// read; otherwise it is an error, never an absence.
func claimedCheckout(dir, workerRun string) (space.Checkout, bool, error) {
	record, ok, err := claimed(dir, workerRun)
	if !ok {
		return space.Checkout{}, false, err
	}
	return space.Checkout{Path: record.Path, Branch: record.Branch}, true, nil
}

func claimed(dir, workerRun string) (worktree.Record, bool, error) {
	root, err := worktree.Root()
	if err != nil {
		return worktree.Record{}, false, err
	}
	records, err := worktree.List(root)
	if err != nil {
		return worktree.Record{}, false, err
	}
	for _, record := range records {
		if record.Session != nil && record.Session.Epoch == workerRun && record.Session.Dir == dir {
			return record, true, nil
		}
	}
	return worktree.Record{}, false, nil
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
	if record, ok, _ := claimed(dir, r.Worker.Run); ok {
		if check, err := worktree.Inspect(record); err == nil {
			found.Changes, found.Commits, found.Locked = check.Changes, check.Head != record.Commit, check.Locked
		}
	}
	return found
}

// release removes the worker's checkout and its branch when nothing would be
// lost, and says why it stays otherwise. It runs only once the worker's
// process group is seen gone, so no harness of the worker writes there any
// more. A checkout whose record is gone — removed by hand — is left to seen.
func (h *host) release() (stays, note string) {
	if h.record.Checkout.Path == "" {
		return "", ""
	}
	record, ok, err := claimed(h.spec.Dir, h.record.Worker.Run)
	switch {
	case err != nil:
		return "rewake could not read its record: " + err.Error(), ""
	case !ok:
		return "", ""
	}
	return worktree.Release(record)
}

// releaseReviews removes the checkouts the worker was given to review in,
// its run having ended, and says which stay and why, and what kept it from
// looking.
func (h *host) releaseReviews() string {
	released := reviewcheckout.ReleaseRun(h.spec.Dir, h.record.Worker.Run)
	var notes []string
	for _, path := range released.Paths() {
		notes = append(notes, "its review checkout "+path+" stays: "+released.Kept[path])
	}
	if released.Err != nil {
		notes = append(notes, "its review checkouts could not all be released: "+released.Err.Error())
	}
	return strings.Join(notes, "; ")
}
