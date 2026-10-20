package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/space"
	"github.com/praline-labs/rewake/internal/host/worktree"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// The outcomes of a close forecast.
const (
	// closeRemoves: the space's close would remove the checkout and its branch.
	closeRemoves = "remove"
	// closeKeeps: it would keep them, for the reasons given.
	closeKeeps = "keep"
	// closeUnknown: rewake could not tell; the close keeps what it cannot judge.
	closeUnknown = "unknown"
	// closeNone: no space's close removes a checkout of this session.
	closeNone = "none"
)

// worktreeStatus is what rewake worktree status answers: the checkout the
// calling run claimed, and what its space's close would do with it.
type worktreeStatus struct {
	Session string `json:"session"`
	Room    string `json:"room"`
	Role    string `json:"role"`
	// Checkout is the one this run claimed; null when it claimed none.
	Checkout *statusCheckout `json:"checkout"`
	// Space is the space whose worker this run is, when it is one.
	Space string        `json:"space,omitempty"`
	Close closeForecast `json:"close"`
}

type statusCheckout struct {
	Ref    string `json:"ref"`
	Path   string `json:"path"`
	Branch string `json:"branch,omitempty"`
	// Base is the commit it was made at, Source the checkout it was made
	// from.
	Base   string `json:"base"`
	Source string `json:"source"`
}

// closeForecast is what the close of this run's space would do with its
// checkout if it closed now: a look, not a promise.
type closeForecast struct {
	Outcome string   `json:"outcome"`
	Reasons []string `json:"reasons,omitempty"`
	Note    string   `json:"note"`
}

// worktreeStatusView answers for the session it runs in, known by its room,
// name and run — never by its working directory, nor by whoever holds the
// name now. A view: it reads the records and Git, and writes nothing, not
// even a lock's holder file.
func worktreeStatusView(ctx *Context, call Call, root string) error {
	dir, err := state.Dir()
	if err != nil {
		return &UsageError{Command: call.Command, Message: err.Error()}
	}
	self, epoch, err := ownRunIn(accessView, dir)
	if errors.Is(err, errNotASession) {
		return &UsageError{Command: call.Command, Message: "status answers for the session it runs in; this shell runs in none. rewake worktree ls lists every worktree."}
	}
	if err != nil {
		return failf("%v", err)
	}
	status := worktreeStatus{Session: self.Name, Room: filepath.Base(dir), Role: self.Role}
	record, claimed, err := claimedBy(root, dir, self, epoch)
	if err != nil {
		return failf("could not read the worktrees under %s: %v", root, err)
	}
	if claimed {
		status.Checkout = &statusCheckout{Ref: record.Ref(), Path: record.Path, Branch: record.Branch, Base: record.Commit, Source: record.Source}
	}
	worker, err := spaceOf(dir, self, epoch)
	if err != nil {
		return failf("could not read the spaces of %s: %v", dir, err)
	}
	status.Space = worker
	status.Close = forecast(record, claimed, worker != "")
	return printValue(ctx, status, status.lines)
}

// claimedBy is the checkout whose record names this run as its owner.
func claimedBy(root, dir string, self registry.Session, epoch string) (worktree.Record, bool, error) {
	records, err := worktree.List(root)
	if err != nil {
		return worktree.Record{}, false, err
	}
	for _, record := range records {
		owner := record.Session
		if owner != nil && filepath.Clean(owner.Dir) == filepath.Clean(dir) && owner.Name == self.Name && owner.Epoch == epoch {
			return record, true, nil
		}
	}
	return worktree.Record{}, false, nil
}

// spaceOf names the space not yet over whose worker is this run, or "".
func spaceOf(dir string, self registry.Session, epoch string) (string, error) {
	records, _, err := space.List(dir)
	if err != nil {
		return "", err
	}
	for _, record := range records {
		worker := record.Worker
		if !record.State.Final() && worker.Run == epoch && worker.Name+"-"+worker.Harness == self.Name {
			return record.Name, nil
		}
	}
	return "", nil
}

// forecast is what the close would do. Only a space's close removes a
// checkout by itself, and only its worker's.
func forecast(record worktree.Record, claimed, worker bool) closeForecast {
	switch {
	case !claimed && worker:
		return closeForecast{Outcome: closeNone, Note: "this worker has no checkout of its own; its space's close removes nothing"}
	case !claimed:
		return closeForecast{Outcome: closeNone, Note: "this session has no checkout rewake made, and no space's close applies to it"}
	case !worker:
		return closeForecast{Outcome: closeNone, Note: "no automatic removal at a close applies: this session is not a space's worker, and its checkout stays after it until rewake worktree finish or rm removes it"}
	}
	reasons, err := worktree.CloseForecast(record)
	switch {
	case err != nil:
		return closeForecast{Outcome: closeUnknown, Note: "rewake could not tell what the close would do, and a close keeps a checkout it cannot judge: " + err.Error()}
	case len(reasons) > 0:
		return closeForecast{Outcome: closeKeeps, Reasons: reasons, Note: "if this space closed now, its close would keep the checkout and its branch; rewake space list then shows them for main to land or remove"}
	}
	return closeForecast{Outcome: closeRemoves, Note: "if this space closed now, its close would remove the checkout and its branch: nothing in it would be lost, and files git ignores go with it. A look now, not a promise: later edits and commits change it"}
}

func (s worktreeStatus) lines() []string {
	lines := []string{fmt.Sprintf("session %s in room %s", s.Session, s.Room)}
	if s.Space != "" {
		lines[0] += ", the worker of space " + s.Space
	}
	if c := s.Checkout; c != nil {
		lines = append(lines, fmt.Sprintf("checkout %s at %s, branch %s, made from %s at %s", c.Ref, c.Path, orDash(c.Branch), c.Source, shortCommit(c.Base)))
	} else {
		lines = append(lines, "checkout: none")
	}
	lines = append(lines, "at close: "+s.Close.Outcome+" — "+s.Close.Note)
	if len(s.Close.Reasons) > 0 {
		lines = append(lines, "  because "+strings.Join(s.Close.Reasons, "; "))
	}
	return lines
}
