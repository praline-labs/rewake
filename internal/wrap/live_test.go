package wrap

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/grant"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/role"
)

// liveRun publishes a write session of this process with no live set yet.
func liveRun(t *testing.T) (string, registry.Session) {
	t.Helper()
	dir := stateDir(t)
	session := registry.Session{
		Name: "api", Harness: "fake", ServicePID: os.Getpid(), ServiceStart: selfStart(t), Boot: thisBoot(t),
		CWD: dir, StartedAt: time.Now(), Role: role.Write.ID,
	}
	if err := registry.Publish(dir, session); err != nil {
		t.Fatal(err)
	}
	return dir, session
}

func recordedLive(t *testing.T, dir string) registry.Session {
	t.Helper()
	session, err := registry.Load(dir, "api")
	if err != nil {
		t.Fatal(err)
	}
	return session
}

// The record carries each set as the run reports it, the withdrawal at once;
// a capability lost and made live again leaves the run as it was — its name,
// epoch and role, and the grants its tasks gave it, which are kept by the
// name and epoch.
func TestTheLiveSetIsRecordedAndADropLeavesTheRun(t *testing.T) {
	dir, session := liveRun(t)
	granted := []grant.Entry{{Path: dir, Message: "m1", At: time.Now(), Outcome: "granted"}}
	if err := grant.Save(dir, session.Name, session.Epoch(), granted); err != nil {
		t.Fatal(err)
	}
	live := newLiveSet(dir, session.Name, session.Epoch())
	var exchanged [][]adapter.Capability
	live.exchanged = func(set []adapter.Capability) { exchanged = append(exchanged, set) }
	both := []adapter.Capability{adapter.WakeCapability, adapter.TurnBoundaryCapability}
	for _, step := range []struct {
		set  []adapter.Capability
		want []string
	}{
		{both, []string{"wake", "turn-boundary"}},
		{[]adapter.Capability{adapter.WakeCapability}, []string{"wake"}},
		{nil, nil},
		{both, []string{"wake", "turn-boundary"}},
	} {
		live.report(step.set)
		record := recordedLive(t, dir)
		if !slices.Equal(record.Live, step.want) {
			t.Fatalf("after %v the record has %v", step.set, record.Live)
		}
		if record.Name != session.Name || record.Epoch() != session.Epoch() || record.Role != session.Role {
			t.Fatalf("liveness changed the run: %+v", record)
		}
		if live.has(adapter.TurnBoundaryCapability) != slices.Contains(step.want, "turn-boundary") {
			t.Fatalf("the run's own view disagrees with its record after %v", step.set)
		}
		if entries := grant.Load(dir, session.Name, session.Epoch()); len(entries) != 1 || entries[0].Message != "m1" {
			t.Fatalf("the grants after %v: %+v", step.set, entries)
		}
	}
	if len(exchanged) != 1 || !slices.Equal(exchanged[0], both) {
		t.Fatalf("the exchange was heard %v, want once with its own set", exchanged)
	}
}

// A record the run no longer owns is not written: a later run under the name
// keeps its own set.
func TestTheLiveSetDoesNotWriteAnotherRunsRecord(t *testing.T) {
	dir, session := liveRun(t)
	live := newLiveSet(dir, session.Name, "999.999")
	live.report([]adapter.Capability{adapter.WakeCapability})
	if record := recordedLive(t, dir); len(record.Live) != 0 {
		t.Fatalf("a stale run wrote %v", record.Live)
	}
}

// boundaryHarness is the fake as a harness whose contract reports turns.
type boundaryHarness struct{ *fakeHarness }

func (boundaryHarness) Offers() adapter.Offer {
	return adapter.Offer{Wake: true, TurnBoundary: adapter.TurnBoundaryOffer{Reports: true}}
}

// A reporting role waits for its boundary up to the readiness bound and is
// refused when the exchange proves none; main and a harness with no boundary
// to wait for pass at once.
func TestAReportingRoleWaitsForItsBoundary(t *testing.T) {
	dir, session := liveRun(t)
	h := boundaryHarness{&fakeHarness{}}

	live := newLiveSet(dir, session.Name, session.Epoch())
	go func() {
		time.Sleep(50 * time.Millisecond)
		live.report([]adapter.Capability{adapter.WakeCapability, adapter.TurnBoundaryCapability})
	}()
	if err := awaitBoundary(context.Background(), session, h, live); err != nil {
		t.Fatalf("a boundary made live was refused: %v", err)
	}

	without := newLiveSet(dir, session.Name, session.Epoch())
	without.report([]adapter.Capability{adapter.WakeCapability})
	began := time.Now()
	err := awaitBoundary(context.Background(), session, h, without)
	if err == nil || !strings.Contains(err.Error(), "did not make its turn boundary live") || !strings.Contains(err.Error(), "nothing was started") {
		t.Fatalf("an exchange without the boundary: %v", err)
	}
	if time.Since(began) > time.Second {
		t.Fatal("a finished exchange without the boundary waited for the bound")
	}

	main := session
	main.Role = role.Main.ID
	if err := awaitBoundary(context.Background(), main, h, newLiveSet(dir, session.Name, session.Epoch())); err != nil {
		t.Fatalf("main was held: %v", err)
	}
	if err := awaitBoundary(context.Background(), session, &fakeHarness{}, newLiveSet(dir, session.Name, session.Epoch())); err != nil {
		t.Fatalf("a harness offering no boundary was held: %v", err)
	}
}

// A run whose exchange never reports is refused at the bound.
func TestABoundaryNeverReportedIsRefusedAtTheBound(t *testing.T) {
	dir, session := liveRun(t)
	live := newLiveSet(dir, session.Name, session.Epoch())
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if live.await(ctx, adapter.TurnBoundaryCapability, time.Hour) {
		t.Fatal("a set never reported has the boundary live")
	}
	if live.await(context.Background(), adapter.TurnBoundaryCapability, 30*time.Millisecond) {
		t.Fatal("the bound passed with the boundary live")
	}
}

// A run that reports no set of its own has live what its plan carries.
func TestARunThatReportsNoneHasItsPlanLive(t *testing.T) {
	plan := adapter.LaunchPlan{Wake: &fakeHarness{}}
	offer := adapter.Offer{Wake: true, Control: adapter.ControlOffer{Compact: true}}
	got := plannedLive(offer, plan, "/control")
	if !slices.Equal(got, []adapter.Capability{adapter.WakeCapability, adapter.ControlCapability}) {
		t.Fatalf("planned %v", got)
	}
	if got := plannedLive(offer, plan, ""); slices.Contains(got, adapter.ControlCapability) {
		t.Fatalf("control live without a control directory: %v", got)
	}
}

// A letter to a run without a live Wake waits in its inbox rather than
// failing: the session reads it with rewake inbox. A Wake made live again
// carries the next attempt.
func TestADeliveryWithoutALiveWakeWaits(t *testing.T) {
	dir, session := liveRun(t)
	fake := &fakeHarness{}
	live := newLiveSet(dir, session.Name, session.Epoch())
	deliver := wakeIfLive(fake, live)
	message := inbox.Message{ID: inbox.NewID(), Text: "hello"}
	for _, set := range [][]adapter.Capability{nil, {adapter.TurnBoundaryCapability}} {
		live.report(set)
		if result := deliver(context.Background(), message); result.State != inbox.Pending || result.Detail != noWake {
			t.Fatalf("with %v live: %+v", set, result)
		}
	}
	if len(fake.seen()) != 0 {
		t.Fatal("a notice went through a Wake that is not live")
	}
	live.report([]adapter.Capability{adapter.WakeCapability})
	if result := deliver(context.Background(), message); result.State != inbox.Delivered || len(fake.seen()) != 1 {
		t.Fatalf("with the Wake live again: %+v", result)
	}
	if result := wakeIfLive(nil, live)(context.Background(), message); result.State != inbox.Pending {
		t.Fatalf("a plan with no Wake: %+v", result)
	}
}
