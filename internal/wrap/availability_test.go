package wrap

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/role"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

func availabilityPeer(t *testing.T, dir, name, roleName string, epoch uint64, ready bool) registry.Session {
	t.Helper()
	if err := state.EnsureSubdir(state.SessionsPath(dir)); err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(-time.Minute)
	peer := registry.Session{Name: name, Role: roleName, Harness: "fixture", Room: filepath.Base(dir), CWD: "/workspace", ServicePID: 111, ServiceStart: epoch, PIDNamespace: "fixture-foreign", StartedAt: at}
	if ready {
		peer.MessagingReadyAt = &at
	}
	if err := registry.Publish(dir, peer); err != nil {
		t.Fatal(err)
	}
	return peer
}

func availabilityFiles(t *testing.T, dir, name string) []inbox.Message {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(state.InboxPath(dir, name), "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	var messages []inbox.Message
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		var m inbox.Message
		if json.Unmarshal(raw, &m) != nil {
			t.Fatal("invalid announcement")
		}
		messages = append(messages, m)
	}
	return messages
}

func TestAvailabilityMainOnlyReadinessEpochAndRetryDedup(t *testing.T) {
	dir := stateDir(t)
	main := availabilityPeer(t, dir, "main-fixture", role.Main.ID, 1, true)
	peer := availabilityPeer(t, dir, "worker-fixture", role.Write.ID, 2, false)
	seen := map[string]registry.Session{}
	if err := announceAvailable(context.Background(), dir, main, seen); err != nil {
		t.Fatal(err)
	}
	if len(availabilityFiles(t, dir, main.Name)) != 0 {
		t.Fatal("announced before initialization")
	}
	if err := markMessagingReady(dir, peer); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if err := announceAvailable(context.Background(), dir, main, seen); err != nil {
			t.Fatal(err)
		}
	}
	messages := availabilityFiles(t, dir, main.Name)
	if len(messages) != 1 {
		t.Fatalf("duplicate availability: %d", len(messages))
	}
	m := messages[0]
	if m.From != peer.Name || m.FromEpoch != peer.Epoch() || m.ToEpoch != main.Epoch() || m.Kind != inbox.Note || inbox.Owed(m) || m.Availability == nil || m.Availability.Role != role.Write.ID || m.Availability.CWD != peer.CWD {
		t.Fatalf("wrong service message: %+v", m)
	}
	// A retry after losing the in-memory receipt still finds the durable message.
	if err := announceAvailable(context.Background(), dir, main, map[string]registry.Session{}); err != nil {
		t.Fatal(err)
	}
	if len(availabilityFiles(t, dir, main.Name)) != 1 {
		t.Fatal("durable retry duplicated notification")
	}
	// Even normal mailbox retention does not reannounce a live registered epoch.
	if err := os.Remove(filepath.Join(state.InboxPath(dir, main.Name), m.ID+".json")); err != nil {
		t.Fatal(err)
	}
	if err := announceAvailable(context.Background(), dir, main, seen); err != nil {
		t.Fatal(err)
	}
	if len(availabilityFiles(t, dir, main.Name)) != 0 {
		t.Fatal("live epoch reannounced after retention")
	}
	if err := announceAvailable(context.Background(), dir, peer, map[string]registry.Session{}); err == nil {
		t.Fatal("worker was allowed to discover/receive availability")
	}
	if len(availabilityFiles(t, dir, peer.Name)) != 0 {
		t.Fatal("worker got announcement")
	}
	if err := registry.Remove(dir, peer.Name); err != nil {
		t.Fatal(err)
	}
	replacement := availabilityPeer(t, dir, peer.Name, role.Write.ID, 3, true)
	if err := announceAvailable(context.Background(), dir, main, seen); err != nil {
		t.Fatal(err)
	}
	messages = availabilityFiles(t, dir, main.Name)
	if len(messages) != 1 || messages[0].FromEpoch != replacement.Epoch() || messages[0].ID == m.ID {
		t.Fatal("name reuse borrowed prior launch announcement")
	}
}

func TestLateMainDiscoversExistingWorkersInItsRoomOnly(t *testing.T) {
	dir := stateDir(t)
	peer := availabilityPeer(t, dir, "early", role.General.ID, 1, true)
	main := availabilityPeer(t, dir, "main-fixture", role.Main.ID, 2, true)
	main.StartedAt = time.Now()
	if err := registry.Update(dir, main); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(filepath.Dir(dir), "other")
	if err := state.EnsureSubdir(other); err != nil {
		t.Fatal(err)
	}
	_ = availabilityPeer(t, other, "foreign", role.Write.ID, 3, true)
	if err := announceAvailable(context.Background(), dir, main, map[string]registry.Session{}); err != nil {
		t.Fatal(err)
	}
	messages := availabilityFiles(t, dir, main.Name)
	if len(messages) != 1 || messages[0].From != peer.Name || !messages[0].Availability.Existing {
		t.Fatal("late main missed existing worker or crossed room")
	}
	// The next main epoch has its own discovery, even for the same worker epoch.
	if err := registry.Remove(dir, main.Name); err != nil {
		t.Fatal(err)
	}
	next := availabilityPeer(t, dir, main.Name, role.Main.ID, 4, true)
	if err := announceAvailable(context.Background(), dir, next, map[string]registry.Session{}); err != nil {
		t.Fatal(err)
	}
	if got := availabilityFiles(t, dir, main.Name); len(got) != 2 || got[0].ID == got[1].ID {
		t.Fatal("new main inherited previous main's receipt")
	}
}

func TestAvailabilityWaitsForServiceAndTransportAndToleratesStorageFailure(t *testing.T) {
	dir := stateDir(t)
	peer := availabilityPeer(t, dir, "worker-fixture", role.General.ID, 1, false)
	ready := make(chan struct{})
	var usable atomic.Bool
	stop := startAvailability(context.Background(), dir, peer, ready, usable.Load)
	defer stop()
	usable.Store(true)
	time.Sleep(30 * time.Millisecond)
	current, _ := registry.Load(dir, peer.Name)
	if current.MessagingReadyAt != nil {
		t.Fatal("service signal ignored")
	}
	usable.Store(false)
	close(ready)
	time.Sleep(30 * time.Millisecond)
	current, _ = registry.Load(dir, peer.Name)
	if current.MessagingReadyAt != nil {
		t.Fatal("failed transport announced ready")
	}
	usable.Store(true)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		current, _ = registry.Load(dir, peer.Name)
		if current.MessagingReadyAt != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if current.MessagingReadyAt == nil {
		t.Fatal("ready service never registered")
	}
	main := availabilityPeer(t, dir, "main-fixture", role.Main.ID, 2, true)
	// Failure before publication keeps identity stable for the later retry.
	mailbox := state.InboxPath(dir, main.Name)
	if err := state.EnsureSubdir(filepath.Dir(mailbox)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mailbox, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	seen := map[string]registry.Session{}
	_ = announceAvailable(context.Background(), dir, main, seen)
	if len(seen) != 0 {
		t.Fatal("failed notification marked announced")
	}
	if err := os.Remove(mailbox); err != nil {
		t.Fatal(err)
	}
	if err := announceAvailable(context.Background(), dir, main, seen); err != nil {
		t.Fatal(err)
	}
	if len(availabilityFiles(t, dir, main.Name)) != 1 {
		t.Fatal("notification failure did not recover")
	}
}

func TestAvailabilityUsesNormalWrapperNotifyDelivery(t *testing.T) {
	dir := stateDir(t)
	peer := availabilityPeer(t, dir, "worker-fixture", role.General.ID, 1, true)
	fake := &fakeHarness{script: "sleep 0.7"}
	code, err := Run(context.Background(), Request{Harness: fake, Dir: dir, Role: role.Main})
	if code != 0 || err != nil {
		t.Fatalf("wrapper failed: %d %v", code, err)
	}
	messages := fake.seen()
	if len(messages) != 1 || messages[0].FromEpoch != peer.Epoch() || inbox.KindOf(messages[0]) != inbox.Note || inbox.Owed(messages[0]) {
		t.Fatalf("normal notification path: %+v", messages)
	}
}
