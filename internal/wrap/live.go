package wrap

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/role"
)

// noWake is what a letter to a run without a live Wake waits with: written,
// and read when the session runs rewake inbox.
const noWake = "no notice can reach this session; it reads its mail when it runs rewake inbox"

// liveSet is the run's live set (docs/v2/design-api.md#two-levels-offered-and-live):
// what the run has proven of its adapter's capabilities, recorded in the
// session record as it changes, so every decision another session makes about
// this one reads it there. A run that reports its own (adapter.LiveReporter)
// sets it by its readiness exchange and withdraws a capability the moment it
// is lost; any other run has live what its plan carries, from its start.
type liveSet struct {
	dir, name, epoch string
	// planned is the set of a run that reports none.
	planned []adapter.Capability
	// exchanged hears the first report, the readiness exchange's result.
	exchanged func([]adapter.Capability)

	// writing orders the reports with their records: the run reports one
	// at a time, and the record written last is the set reported last.
	writing sync.Mutex
	mu      sync.Mutex
	set     []adapter.Capability
	ready   bool
	changed chan struct{}
}

func newLiveSet(dir, name, epoch string) *liveSet {
	return &liveSet{dir: dir, name: name, epoch: epoch, changed: make(chan struct{})}
}

// report records the whole live set. A record that cannot be written keeps
// its last set, which a reader takes as it stands; the run goes on.
func (l *liveSet) report(set []adapter.Capability) {
	l.writing.Lock()
	defer l.writing.Unlock()
	names := make([]string, 0, len(set))
	for _, capability := range set {
		names = append(names, string(capability))
	}
	_ = registry.Modify(l.dir, l.name, l.epoch, func(session *registry.Session) { session.Live = names })
	l.mu.Lock()
	first := !l.ready
	l.set, l.ready = slices.Clone(set), true
	changed := l.changed
	l.changed = make(chan struct{})
	l.mu.Unlock()
	close(changed)
	if first && l.exchanged != nil {
		l.exchanged(set)
	}
}

// has says whether a capability is live now.
func (l *liveSet) has(capability adapter.Capability) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Contains(l.set, capability)
}

// await waits for the run's readiness exchange to report, up to bound, and
// says whether the capability is live then: what the exchange did not prove
// is absent, and so is everything when it never reports.
func (l *liveSet) await(ctx context.Context, capability adapter.Capability, bound time.Duration) bool {
	timer := time.NewTimer(bound)
	defer timer.Stop()
	for {
		l.mu.Lock()
		live, ready, changed := slices.Contains(l.set, capability), l.ready, l.changed
		l.mu.Unlock()
		if live || ready {
			return live
		}
		select {
		case <-changed:
		case <-timer.C:
			return l.has(capability)
		case <-ctx.Done():
			return false
		}
	}
}

// wakeIfLive delivers through the run's Wake while it is live. Otherwise the
// letter stays readable and waits for a Wake that becomes live; the sender was
// told already (docs/v2/design-api.md#the-capabilities).
func wakeIfLive(wake adapter.Wake, live *liveSet) func(context.Context, inbox.Message) inbox.Result {
	return func(ctx context.Context, message inbox.Message) inbox.Result {
		if wake == nil || !live.has(adapter.WakeCapability) {
			return inbox.Result{State: inbox.Pending, Detail: noWake}
		}
		return wake.Deliver(ctx, message)
	}
}

// reserveIfLive reserves through the run's Wake while it is live. Otherwise it
// reserves nothing, and the letter takes the plain path: made readable, then
// handed to wakeIfLive, which leaves it waiting. A Wake withdrawn while the
// reservation was asked is the same case, so the letter a sender was told it
// could read is never failed for want of a notice.
func reserveIfLive(reserver adapter.Reserver, live *liveSet) inbox.Reserver {
	return func(ctx context.Context, message inbox.Message) (inbox.Reservation, error) {
		if !live.has(adapter.WakeCapability) {
			return nil, nil
		}
		reservation, err := reserver.Reserve(ctx, message)
		if err != nil && !live.has(adapter.WakeCapability) {
			return nil, nil
		}
		return reservation, err
	}
}

// threadIfLive asks the run's conversation while its Wake is live. Without
// it, as when the conversation rests on the Wake and went with it, a letter
// owed a report is made readable in no conversation rather than failed: the
// state of a letter whose sender's wrapper did not answer its pin, which
// holds for this run and is not taken into a resume (wrap/grants.go). A
// delivery tried again once the Wake is live pins it then.
func threadIfLive(thread func() (string, error), live *liveSet) func() (string, error) {
	if thread == nil {
		return nil
	}
	return func() (string, error) {
		if !live.has(adapter.WakeCapability) {
			return "", nil
		}
		conversation, err := thread()
		if err != nil && !live.has(adapter.WakeCapability) {
			return "", nil
		}
		return conversation, err
	}
}

// plannedLive is the live set of a run that reports none: each capability
// its plan carries, and Control where it is offered and the run has its
// control directory. The order is the API's, as a reporting run's.
func plannedLive(offer adapter.Offer, plan adapter.LaunchPlan, controlDir string) []adapter.Capability {
	var live []adapter.Capability
	if plan.Wake != nil {
		live = append(live, adapter.WakeCapability)
	}
	if plan.TurnBoundary != nil && offer.TurnBoundary.Reports {
		live = append(live, adapter.TurnBoundaryCapability)
	}
	if plan.Telemetry != nil {
		live = append(live, adapter.TelemetryCapability)
	}
	if offer.Control.Any() && controlDir != "" {
		live = append(live, adapter.ControlCapability)
	}
	if _, ok := plan.Service.(adapter.ToolTransport[bridge.ToolDescriptor]); ok && offer.ToolTransport != "" {
		live = append(live, adapter.ToolTransportCapability)
	}
	return live
}

// awaitBoundary holds a launch of a role that owes reports until the run's
// turn boundary is live, up to the readiness bound, and refuses it after:
// without the boundary no turn's end reaches the sender, and the worker's
// contract — end your turn, your final reply goes to the sender — would not
// hold (docs/v2/design-api.md#turnboundary). Before the harness starts, so a
// refusal starts nothing. A harness that offers no boundary has none to wait
// for: its launch of such a role was refused at once, before the claim
// (S9's rule, cli/turn_reporting.go).
func awaitBoundary(ctx context.Context, session registry.Session, h adapter.Harness, live *liveSet) error {
	part := role.Of(session.Role)
	if part.Silent || !h.Offers().TurnBoundary.Reports || live.await(ctx, adapter.TurnBoundaryCapability, adapter.ReadinessBound) {
		return nil
	}
	return fmt.Errorf("a %s session owes a report at each turn's end, and this run of %s did not make its turn boundary live within %s, so the end of its turns would reach nobody; nothing was started. Start it again, or start it with --main",
		part.ID, h.Title(), adapter.ReadinessBound)
}
