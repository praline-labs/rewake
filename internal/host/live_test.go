package host

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/grant"
	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/infra/state"
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
	message := mail.Message{ID: mail.NewID(), Text: "hello"}
	for _, set := range [][]adapter.Capability{nil, {adapter.TurnBoundaryCapability}} {
		live.report(set)
		if result := deliver(context.Background(), message); result.State != mail.Pending || result.Detail != noWake {
			t.Fatalf("with %v live: %+v", set, result)
		}
	}
	if len(fake.seen()) != 0 {
		t.Fatal("a notice went through a Wake that is not live")
	}
	live.report([]adapter.Capability{adapter.WakeCapability})
	if result := deliver(context.Background(), message); result.State != mail.Delivered || len(fake.seen()) != 1 {
		t.Fatalf("with the Wake live again: %+v", result)
	}
	if result := wakeIfLive(nil, live)(context.Background(), message); result.State != mail.Pending {
		t.Fatalf("a plan with no Wake: %+v", result)
	}
}

// fakeReserver answers each reservation with what its hook says.
type fakeReserver struct {
	calls int
	hook  func() (mail.Reservation, error)
}

func (f *fakeReserver) Reserve(context.Context, mail.Message) (mail.Reservation, error) {
	f.calls++
	return f.hook()
}

type heldReservation struct{ mail.Reservation }

// Without a live Wake nothing is reserved and no conversation is asked, so
// the letter goes the plain way, readable and waiting; a Wake withdrawn while
// it was asked is the same. With the Wake live again both go through, and a
// refusal while it is live stays a refusal.
func TestWithoutALiveWakeNothingIsReservedOrPinned(t *testing.T) {
	dir, session := liveRun(t)
	live := newLiveSet(dir, session.Name, session.Epoch())
	held := heldReservation{}
	reserver := &fakeReserver{hook: func() (mail.Reservation, error) { return held, nil }}
	reserve := reserveIfLive(reserver, live)
	asked := 0
	thread := threadIfLive(func() (string, error) { asked++; return "A", nil }, live)

	live.report([]adapter.Capability{adapter.TurnBoundaryCapability})
	if r, err := reserve(context.Background(), mail.Message{}); r != nil || err != nil || reserver.calls != 0 {
		t.Fatalf("without a live Wake: %v %v, asked %d times", r, err, reserver.calls)
	}
	if conversation, err := thread(); conversation != "" || err != nil || asked != 0 {
		t.Fatalf("a conversation without a live Wake: %q %v", conversation, err)
	}

	live.report([]adapter.Capability{adapter.WakeCapability})
	if r, err := reserve(context.Background(), mail.Message{}); r != held || err != nil {
		t.Fatalf("with the Wake live again: %v %v", r, err)
	}
	if conversation, err := thread(); conversation != "A" || err != nil {
		t.Fatalf("the conversation with the Wake live again: %q %v", conversation, err)
	}

	reserver.hook = func() (mail.Reservation, error) {
		live.report(nil)
		return nil, mail.ErrThreadUnavailable
	}
	if r, err := reserve(context.Background(), mail.Message{}); r != nil || err != nil {
		t.Fatalf("a Wake withdrawn while reserving: %v %v", r, err)
	}

	live.report([]adapter.Capability{adapter.WakeCapability})
	reserver.hook = func() (mail.Reservation, error) { return nil, mail.ErrThreadUnavailable }
	if _, err := reserve(context.Background(), mail.Message{}); !errors.Is(err, mail.ErrThreadUnavailable) {
		t.Fatalf("a refusal with the Wake live: %v", err)
	}
	if threadIfLive(nil, live) != nil {
		t.Fatal("a run with no conversation got one")
	}
}

// A withdrawal whose write fails is not taken for published: the record is
// owed it and written again until it lands, so a reader stops admitting what
// the run has lost. A record that has become another run's is never written.
func TestAWithdrawalThatFailsToLandIsWrittenAgain(t *testing.T) {
	dir, session := liveRun(t)
	live := newLiveSet(dir, session.Name, session.Epoch())
	t.Cleanup(live.stop)
	both := []adapter.Capability{adapter.WakeCapability, adapter.TurnBoundaryCapability}
	live.report(both)
	previous := state.Fault
	t.Cleanup(func() { state.Fault = previous })
	var failing atomic.Int32
	state.Fault = func(op, path string) error {
		if op == state.OpWrite && path == state.SessionPath(dir, session.Name) && failing.Add(-1) >= 0 {
			return errors.New("a transient write failure")
		}
		return nil
	}
	// One failure is written again at once: no moment with the old set.
	failing.Store(1)
	live.report([]adapter.Capability{adapter.WakeCapability})
	if record := recordedLive(t, dir); !live.published() || !slices.Equal(record.Live, []string{"wake"}) {
		t.Fatalf("one failed write: published %t, record %v", live.published(), record.Live)
	}

	// More are owed and retried.
	failing.Store(3)
	live.report(nil)
	if live.has(adapter.TurnBoundaryCapability) {
		t.Fatal("the run's own view kept what it lost")
	}
	if live.published() || !recordedLive(t, dir).HasLive(string(adapter.WakeCapability)) {
		t.Fatalf("a withdrawal that did not land counted as published: %v", recordedLive(t, dir).Live)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !live.published() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if record := recordedLive(t, dir); !live.published() || len(record.Live) != 0 {
		t.Fatalf("the withdrawal never landed: published %t, record %v", live.published(), record.Live)
	}

	// Owed when a later run takes the name: the retry finds the record no
	// longer this run's, gives up, and leaves the later run's set as it is.
	failing.Store(1 << 20)
	live.report(both)
	live.writing.Lock()
	state.Fault = previous
	later := session
	later.ServiceStart++
	encoded, err := json.Marshal(later)
	if err == nil {
		err = state.WriteAtomic(state.SessionPath(dir, session.Name), encoded)
	}
	live.writing.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(5 * time.Second)
	for !live.published() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if record := recordedLive(t, dir); !live.published() || len(record.Live) != 0 || record.Epoch() != later.Epoch() {
		t.Fatalf("a retry after the name changed hands: given up %t, record %v of %s", live.published(), record.Live, record.Epoch())
	}
}
