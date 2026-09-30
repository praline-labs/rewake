package wrap

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/role"
	"github.com/praline-labs/rewake/internal/sessionstate"
)

func holdState(t *testing.T, dir string, peer registry.Session, hold *sessionstate.DeliveryHold) {
	t.Helper()
	now := time.Now()
	snapshot := sessionstate.Snapshot{Epoch: peer.Epoch(), Fresh: true, PublishedAt: &now, WaitingFor: []string{}, DeliveryHold: hold}
	if err := sessionstate.Save(dir, peer.Name, peer.Epoch(), snapshot); err != nil {
		t.Fatal(err)
	}
}

func holdNotices(t *testing.T, dir, name string) []inbox.Message {
	t.Helper()
	var held []inbox.Message
	for _, m := range availabilityFiles(t, dir, name) {
		if strings.HasPrefix(m.Text, "Rewake: deliveries to ") {
			held = append(held, m)
		}
	}
	return held
}

// main is told once when a worker's deliveries start to wait, once more when
// the reason or the conversation changes, and again when a hold that ended
// comes back.
func TestMainIsToldOncePerHold(t *testing.T) {
	dir := stateDir(t)
	main := availabilityPeer(t, dir, "main-fixture", role.Main.ID, 1, true)
	peer := availabilityPeer(t, dir, "worker-fixture", role.General.ID, 2, true)
	observer := newSessionNotices()
	scan := func(times int) {
		t.Helper()
		for range times {
			if err := observer.scan(context.Background(), dir, main); err != nil {
				t.Fatal(err)
			}
		}
	}
	scan(1)
	unintended := &sessionstate.DeliveryHold{Reason: sessionstate.HoldUnintended, Expected: "X", Selected: "Y", Detail: "the launch asked to resume X, and the terminal selected Y"}
	holdState(t, dir, peer, unintended)
	scan(3)
	held := holdNotices(t, dir, main.Name)
	if len(held) != 1 || held[0].From != peer.Name || held[0].ToEpoch != main.Epoch() || inbox.Owed(held[0]) || !strings.Contains(held[0].Text, "selected Y") {
		t.Fatalf("%+v", held)
	}
	moved := *unintended
	moved.Selected, moved.Detail = "Z", "the launch asked to resume X, and the terminal selected Z"
	holdState(t, dir, peer, &moved)
	scan(2)
	if held = holdNotices(t, dir, main.Name); len(held) != 2 {
		t.Fatalf("a change was not told: %d", len(held))
	}
	holdState(t, dir, peer, nil)
	scan(2)
	holdState(t, dir, peer, &moved)
	scan(2)
	if held = holdNotices(t, dir, main.Name); len(held) != 3 {
		t.Fatalf("a hold that came back was not told: %d", len(held))
	}
	if len(availabilityFiles(t, dir, peer.Name)) != 0 {
		t.Fatal("worker was woken")
	}
}

// A hold whose cause changes under the same reason and conversations — the
// admission record failing with one error, then another — is told again
// (acceptance of September 29, 2026, finding 7).
func TestAChangedCauseIsToldAgain(t *testing.T) {
	dir := stateDir(t)
	main := availabilityPeer(t, dir, "main-fixture", role.Main.ID, 1, true)
	peer := availabilityPeer(t, dir, "worker-fixture", role.General.ID, 2, true)
	observer := newSessionNotices()
	hold := &sessionstate.DeliveryHold{Reason: sessionstate.HoldMailClosed, Expected: "X", Selected: "X", Detail: "the session's mail could not be opened: disk full"}
	holdState(t, dir, peer, hold)
	if err := observer.scan(context.Background(), dir, main); err != nil {
		t.Fatal(err)
	}
	hold.Detail = "the session's mail could not be opened: permission denied"
	holdState(t, dir, peer, hold)
	if err := observer.scan(context.Background(), dir, main); err != nil {
		t.Fatal(err)
	}
	if got := len(holdNotices(t, dir, main.Name)); got != 2 {
		t.Fatalf("the changed cause was not told: %d notices", got)
	}
}
