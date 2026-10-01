package inbox

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"
)

// The conversation main is told a grant went into is the one its letter is
// pinned to, not the one the session had when the grant was first checked: a
// task that waited for an idle reader may go into another after a /clear, and
// main keeps the first conversation it hears of.
func TestAGrantIsNamedTheConversationItsLetterIsPinnedTo(t *testing.T) {
	dir := stateDir(t)
	members := grantPending(t, dir, false, true)
	s := batchServer(dir)
	s.CheckGrant = func(Message) error { return nil }
	type pin struct{ id, thread string }
	var pins []pin
	s.PinGrant = func(message Message, thread string) { pins = append(pins, pin{message.ID, thread}) }
	idle, thread := false, "before-clear"
	s.Reserve = func(_ context.Context, m Message) (Reservation, error) {
		if CarriesGrant(m) && !idle {
			return nil, fmt.Errorf("%w: waiting for idle", ErrNotYet)
		}
		r := &reservationFixture{}
		r.prepare = func(fn func(string) error) error { return fn(thread) }
		r.deliver = func(context.Context, Message) Result { return Result{State: Delivered} }
		return r, nil
	}
	s.drain(context.Background())
	if len(pins) != 0 {
		t.Fatalf("named before the letter was pinned: %v", pins)
	}
	idle, thread, s.attempts = true, "after-clear", map[string]time.Time{}
	s.drain(context.Background())
	if want := []pin{{members[1].ID, "after-clear"}}; !slices.Equal(pins, want) {
		t.Fatalf("named %v, want %v", pins, want)
	}
	if got := must(deliveryThread(dir, "api", members[1].ID)); got != "after-clear" {
		t.Fatalf("pinned to %q", got)
	}
}

// A delivery refused after its letter was pinned — the conversation
// compacting — is pinned again when it is tried again, and main is told the
// conversation the letter went into then, the same one its record keeps.
func TestAGrantIsNamedAgainWhenItsDeliveryIsPinnedAgain(t *testing.T) {
	dir := stateDir(t)
	members := grantPending(t, dir, true)
	s := batchServer(dir)
	s.CheckGrant = func(Message) error { return nil }
	var pins []string
	s.PinGrant = func(_ Message, thread string) { pins = append(pins, thread) }
	thread, outcome := "before-compaction", Result{State: Pending, Detail: "compacting"}
	s.Reserve = func(context.Context, Message) (Reservation, error) {
		r := &reservationFixture{}
		r.prepare = func(fn func(string) error) error { return fn(thread) }
		r.deliver = func(context.Context, Message) Result { return outcome }
		return r, nil
	}
	s.drain(context.Background())
	thread, outcome, s.attempts = "after-compaction", Result{State: Delivered}, map[string]time.Time{}
	s.drain(context.Background())
	if want := []string{"before-compaction", "after-compaction"}; !slices.Equal(pins, want) {
		t.Fatalf("named %v, want %v", pins, want)
	}
	if got := must(deliveryThread(dir, "api", members[0].ID)); got != "after-compaction" {
		t.Fatalf("pinned to %q", got)
	}
}
