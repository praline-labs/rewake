package host

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
)

// afterStartRun is a run whose turn boundary is proven only once its harness
// runs, by a hello from inside it: its service reports Wake alone, and prove
// stands for the hello.
type afterStartRun struct {
	mu       sync.Mutex
	live     func([]adapter.Capability)
	withheld []adapter.Withheld
}

func (*afterStartRun) ReportTurns(adapter.TurnHandler) {}

func (r *afterStartRun) ReportLive(live func([]adapter.Capability)) { r.live = live }

func (r *afterStartRun) Start(context.Context, func(string)) error {
	r.report(adapter.WakeCapability)
	return nil
}

func (*afterStartRun) Close() {}

func (r *afterStartRun) Withheld() []adapter.Withheld { return r.withheld }

func (r *afterStartRun) report(set ...adapter.Capability) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.live(set)
}

func (r *afterStartRun) prove() { r.report(adapter.WakeCapability, adapter.TurnBoundaryCapability) }

type afterStartHarness struct {
	fakeHarness
	run *afterStartRun
	// noBoundary leaves the plan without its turn boundary.
	noBoundary bool
}

func (*afterStartHarness) Offers() adapter.Offer {
	return adapter.Offer{Wake: true, TurnBoundary: adapter.TurnBoundaryOffer{Reports: true, AfterStart: true}}
}

func (h *afterStartHarness) Launch(request adapter.LaunchRequest) (adapter.LaunchPlan, error) {
	plan, err := h.fakeHarness.Launch(request)
	plan.Service, plan.Live = h.run, h.run
	if !h.noBoundary {
		plan.TurnBoundary = h.run
	}
	return plan, err
}

// waitUntil polls until ok or the bound, and says which.
func waitUntil(bound time.Duration, ok func() bool) bool {
	deadline := time.Now().Add(bound)
	for !ok() {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
	return true
}

// awaitFile is a harness that runs until the file exists, ten seconds at most.
func awaitFile(path string) string {
	return fmt.Sprintf(`i=0; while [ ! -e %q ] && [ $i -lt 200 ]; do sleep 0.05; i=$((i+1)); done`, path)
}

// launchAfterStart runs a write session of h until the test releases its
// harness, and answers what Run returned.
func launchAfterStart(t *testing.T, dir string, h *afterStartHarness) (release func() error) {
	t.Helper()
	gate := filepath.Join(t.TempDir(), "go")
	h.script = awaitFile(gate)
	done := make(chan error, 1)
	go func() {
		_, err := Run(context.Background(), Request{Harness: h, Dir: dir, Name: "api", Role: role.Write})
		done <- err
	}()
	return func() error {
		if err := os.WriteFile(gate, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-done:
			return err
		case <-time.After(15 * time.Second):
			t.Fatal("the run did not end")
			return nil
		}
	}
}

// record is the run's record: a launch named api of the fake is api-fake.
func record(dir string) registry.Session {
	session, _ := registry.Load(dir, "api-fake")
	return session
}

// A reporting role whose boundary is proven only after the start is not
// held before it: the harness starts with Wake alone live, so a task to it is
// refused, and the run is not available until the boundary is proven.
func TestABoundaryProvenAfterTheStartStartsTheHarnessAndAvailabilityWaitsForIt(t *testing.T) {
	dir := stateDir(t)
	h := &afterStartHarness{run: &afterStartRun{}}
	release := launchAfterStart(t, dir, h)
	if !waitUntil(5*time.Second, func() bool { return record(dir).HarnessPID != 0 }) {
		t.Fatal("the harness of a boundary proven after the start was not started")
	}
	time.Sleep(600 * time.Millisecond)
	before := record(dir)
	if before.HasLive(string(adapter.TurnBoundaryCapability)) || !before.HasLive(string(adapter.WakeCapability)) {
		t.Fatalf("before the proof the record has %v, want Wake alone", before.Live)
	}
	if before.MessagingReadyAt != nil {
		t.Fatal("the run is available before its boundary is proven")
	}
	h.run.prove()
	if !waitUntil(5*time.Second, func() bool { return record(dir).MessagingReadyAt != nil }) {
		t.Fatal("the run is not available once its boundary is proven")
	}
	if live := record(dir).Live; !slices.Contains(live, string(adapter.TurnBoundaryCapability)) {
		t.Fatalf("after the proof the record has %v", live)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
}

// Past its bound the run is available with its boundary not live.
func TestAvailabilityGoesAheadPastTheBoundWithTheBoundaryNotLive(t *testing.T) {
	previous := afterStartBound
	afterStartBound = 300 * time.Millisecond
	t.Cleanup(func() { afterStartBound = previous })
	dir := stateDir(t)
	h := &afterStartHarness{run: &afterStartRun{}}
	release := launchAfterStart(t, dir, h)
	if !waitUntil(5*time.Second, func() bool { return record(dir).MessagingReadyAt != nil }) {
		t.Fatal("the run never became available past the bound")
	}
	if record(dir).HasLive(string(adapter.TurnBoundaryCapability)) {
		t.Fatal("the boundary is live with no proof")
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
}

// A cause the run knows its boundary will not be proven by refuses a
// reporting role before the harness starts, naming it; a silent role starts.
// A plan without the boundary its offer reports is refused the same way.
func TestAKnownCauseRefusesAReportingRoleBeforeTheStart(t *testing.T) {
	cause := "the rewake mod will not load for this run: its socket path is too long"
	for _, c := range []struct {
		name       string
		part       role.Role
		withheld   []adapter.Withheld
		noBoundary bool
		refused    string
	}{
		{"a known cause", role.Write, []adapter.Withheld{{Capability: adapter.TurnBoundaryCapability, Cause: cause}}, false, cause},
		{"no boundary in the plan", role.Write, nil, true, "has no turn boundary"},
		{"a silent role", role.Main, []adapter.Withheld{{Capability: adapter.TurnBoundaryCapability, Cause: cause}}, true, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := stateDir(t)
			started := filepath.Join(t.TempDir(), "started")
			h := &afterStartHarness{run: &afterStartRun{withheld: c.withheld}, noBoundary: c.noBoundary}
			h.script = "touch " + started
			_, err := Run(context.Background(), Request{Harness: h, Dir: dir, Name: "api", Role: c.part})
			_, statErr := os.Stat(started)
			if c.refused == "" {
				if err != nil || statErr != nil {
					t.Fatalf("a silent role was refused (%v) or not started (%v)", err, statErr)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.refused) || !strings.Contains(err.Error(), "nothing was started") {
				t.Fatalf("the launch answered %v, want a refusal naming %q", err, c.refused)
			}
			if statErr == nil {
				t.Fatal("the harness was started")
			}
		})
	}
}
