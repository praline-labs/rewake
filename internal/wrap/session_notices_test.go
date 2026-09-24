package wrap

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/proc"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/role"
	"github.com/iiiokojiadbi/rewake/internal/sessionstate"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

func noticeState(t *testing.T, dir string, peer registry.Session, activity string, count uint64, events []sessionstate.CompactionEvent) {
	t.Helper()
	now := time.Now()
	model := "fixture"
	effort := "high"
	snapshot := sessionstate.Snapshot{Fresh: true, Selection: "ready", PublishedAt: &now, Activity: &activity, ActivityAt: &now, ActivityFresh: true, WaitingFor: []string{}, Compactions: &count, Coverage: "observed", CompactionEvents: events, Model: &model, Effort: &effort, SettingsFresh: true, SettingsAt: &now}
	if err := sessionstate.Save(dir, peer.Name, peer.Epoch(), snapshot); err != nil {
		t.Fatal(err)
	}
}

func noticeKinds(t *testing.T, dir, name string) (available, compacted, departed []inbox.Message) {
	t.Helper()
	for _, m := range availabilityFiles(t, dir, name) {
		if m.Availability != nil {
			available = append(available, m)
		}
		if m.Compaction != nil {
			compacted = append(compacted, m)
		}
		if m.Departure != nil {
			departed = append(departed, m)
		}
	}
	return
}

func TestCompactionNoticesDedupAndLateMain(t *testing.T) {
	dir := stateDir(t)
	main := availabilityPeer(t, dir, "main-fixture", role.Main.ID, 1, true)
	peer := availabilityPeer(t, dir, "worker-fixture", role.General.ID, 2, true)
	past := main.StartedAt.Add(-time.Second)
	noticeState(t, dir, peer, "idle", 1, []sessionstate.CompactionEvent{{Sequence: 1, ObservedAt: past}})
	observer := newSessionNotices()
	if err := observer.scan(context.Background(), dir, main); err != nil {
		t.Fatal(err)
	}
	available, completed, departed := noticeKinds(t, dir, main.Name)
	if len(available) != 1 || len(completed) != 0 || len(departed) != 0 {
		t.Fatal("late main received historical compaction/departure")
	}
	current := time.Now()
	events := []sessionstate.CompactionEvent{{Sequence: 1, ObservedAt: past}, {Sequence: 2, ObservedAt: current}}
	noticeState(t, dir, peer, "working", 2, events)
	for range 3 {
		if err := observer.scan(context.Background(), dir, main); err != nil {
			t.Fatal(err)
		}
	}
	_, completed, _ = noticeKinds(t, dir, main.Name)
	if len(completed) != 1 || completed[0].Compaction.Count != 2 || completed[0].FromEpoch != peer.Epoch() || completed[0].ToEpoch != main.Epoch() || inbox.Owed(completed[0]) {
		t.Fatal("compaction notice scope/dedup incorrect")
	}
	// Model state changes do not invent a new completion, including reconnect/staleness.
	snapshot := sessionstate.Load(dir, peer.Name, peer.Epoch())
	snapshot.Stale()
	if err := sessionstate.Save(dir, peer.Name, peer.Epoch(), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(state.InboxPath(dir, main.Name), completed[0].ID+".json")); err != nil {
		t.Fatal(err)
	}
	if err := observer.scan(context.Background(), dir, main); err != nil {
		t.Fatal(err)
	}
	_, completed, departed = noticeKinds(t, dir, main.Name)
	if len(completed) != 0 || len(departed) != 0 {
		t.Fatal("stale/reconnect state caused another wake or departure")
	}
	if len(availabilityFiles(t, dir, peer.Name)) != 0 {
		t.Fatal("worker was woken")
	}
	if err := observer.scan(context.Background(), dir, peer); err == nil {
		t.Fatal("worker allowed to run main observer")
	}
}

// A compaction this main asked for with rewake compact is reported by the
// command, with its count, and gets no notice; one another main asked for, and
// one the worker made itself, each get theirs.
func TestACompactionMainAskedForGetsNoNotice(t *testing.T) {
	dir := stateDir(t)
	main := availabilityPeer(t, dir, "main-fixture", role.Main.ID, 1, true)
	peer := availabilityPeer(t, dir, "worker-fixture", role.General.ID, 2, true)
	observer := newSessionNotices()
	if err := observer.scan(context.Background(), dir, main); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	noticeState(t, dir, peer, "idle", 3, []sessionstate.CompactionEvent{
		{Sequence: 1, ObservedAt: now, RequestedBy: main.Name, Request: "0123456789abcdef0123456789abcdef"},
		{Sequence: 2, ObservedAt: now, RequestedBy: "other-main", Request: "fedcba9876543210fedcba9876543210"},
		{Sequence: 3, ObservedAt: now},
	})
	for range 2 {
		if err := observer.scan(context.Background(), dir, main); err != nil {
			t.Fatal(err)
		}
	}
	_, completed, _ := noticeKinds(t, dir, main.Name)
	var counts []uint64
	for _, notice := range completed {
		counts = append(counts, notice.Compaction.Count)
	}
	if len(counts) != 2 || counts[0]+counts[1] != 5 {
		t.Fatalf("notices of compactions %v, want 2 and 3", counts)
	}
}

func TestKnownDepartureRetainsOldStateAndDeduplicates(t *testing.T) {
	for _, mode := range []string{"removed", "dead", "replaced"} {
		t.Run(mode, func(t *testing.T) {
			dir := stateDir(t)
			main := availabilityPeer(t, dir, "main-fixture", role.Main.ID, 1, true)
			peer := availabilityPeer(t, dir, "worker-fixture", role.Write.ID, 2, true)
			noticeState(t, dir, peer, "working", 2, nil)
			observer := newSessionNotices()
			if err := observer.scan(context.Background(), dir, main); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "dead":
				peer.PIDNamespace = proc.Namespace()
				peer.HarnessPID = 2147483647
				peer.HarnessStart = 1
				if err := registry.Update(dir, peer); err != nil {
					t.Fatal(err)
				}
			default:
				if err := registry.Remove(dir, peer.Name); err != nil {
					t.Fatal(err)
				}
				if mode == "replaced" {
					replacement := availabilityPeer(t, dir, peer.Name, role.General.ID, 3, true)
					replacement.CWD = "/replacement"
					if err := registry.Update(dir, replacement); err != nil {
						t.Fatal(err)
					}
					noticeState(t, dir, replacement, "idle", 9, nil)
				}
			}
			// Even with the old snapshot removed, its cached exact-epoch state survives.
			files, _ := filepath.Glob(filepath.Join(dir, "observations", "*.json"))
			for _, file := range files {
				raw, err := os.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}
				var saved sessionstate.Snapshot
				if json.Unmarshal(raw, &saved) != nil {
					t.Fatal("bad state fixture")
				}
				if saved.Epoch == peer.Epoch() {
					if err := os.Remove(file); err != nil {
						t.Fatal(err)
					}
				}
			}
			for range 3 {
				if err := observer.scan(context.Background(), dir, main); err != nil {
					t.Fatal(err)
				}
			}
			available, _, departed := noticeKinds(t, dir, main.Name)
			if len(departed) != 1 {
				t.Fatalf("departures=%d", len(departed))
			}
			note := departed[0]
			snapshot := note.SenderState
			if note.FromEpoch != peer.Epoch() || note.Departure.Identity.CWD != peer.CWD || note.Departure.Identity.Role != role.Write.ID || snapshot == nil || snapshot.Epoch != peer.Epoch() || snapshot.Activity == nil || *snapshot.Activity != "working" || snapshot.Fresh || snapshot.ActivityFresh || *snapshot.Compactions != 2 || inbox.Owed(note) {
				t.Fatal("departed epoch identity/state was lost, replaced or fresh")
			}
			if strings.Contains(note.Text, "crash") {
				t.Fatal("unknown exit called a crash")
			}
			if mode == "replaced" && (len(available) != 2 || note.Departure.Reason != "replaced by a new run") {
				t.Fatal("replacement not separately available")
			}
			if len(availabilityFiles(t, dir, peer.Name)) != 0 {
				t.Fatal("departed worker received a notice")
			}
		})
	}
}

func TestDepartureIgnoresUnknownLivenessStaleStateAndOldAbsence(t *testing.T) {
	dir := stateDir(t)
	main := availabilityPeer(t, dir, "main-fixture", role.Main.ID, 1, true)
	peer := availabilityPeer(t, dir, "worker-fixture", role.General.ID, 2, true)
	noticeState(t, dir, peer, "working", 0, nil)
	observer := newSessionNotices()
	if err := observer.scan(context.Background(), dir, main); err != nil {
		t.Fatal(err)
	}
	// Namespace is intentionally not judgeable: nonexistent-looking PIDs are not proof.
	peer.HarnessPID = 2147483647
	peer.HarnessStart = 1
	if err := registry.Update(dir, peer); err != nil {
		t.Fatal(err)
	}
	snapshot := sessionstate.Load(dir, peer.Name, peer.Epoch())
	old := time.Now().Add(-time.Hour)
	snapshot.PublishedAt = &old
	snapshot.Stale()
	if err := sessionstate.Save(dir, peer.Name, peer.Epoch(), snapshot); err != nil {
		t.Fatal(err)
	}
	if err := observer.scan(context.Background(), dir, main); err != nil {
		t.Fatal(err)
	}
	_, _, departed := noticeKinds(t, dir, main.Name)
	if len(departed) != 0 {
		t.Fatal("unknown process namespace or stale telemetry implied departure")
	}
	if err := registry.Remove(dir, peer.Name); err != nil {
		t.Fatal(err)
	}
	// A new observer/main with no known worker must not announce historical absence.
	fresh := newSessionNotices()
	if err := fresh.scan(context.Background(), dir, main); err != nil {
		t.Fatal(err)
	}
	_, _, departed = noticeKinds(t, dir, main.Name)
	if len(departed) != 0 {
		t.Fatal("unknown historical worker was announced gone")
	}
}

func TestDeparturePublicationRetriesWithoutDuplicate(t *testing.T) {
	dir := stateDir(t)
	main := availabilityPeer(t, dir, "main-fixture", role.Main.ID, 1, true)
	peer := availabilityPeer(t, dir, "worker-fixture", role.General.ID, 2, true)
	noticeState(t, dir, peer, "idle", 1, nil)
	observer := newSessionNotices()
	if err := observer.scan(context.Background(), dir, main); err != nil {
		t.Fatal(err)
	}
	if err := registry.Remove(dir, peer.Name); err != nil {
		t.Fatal(err)
	}
	held, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		_ = state.WithMailboxLock(context.Background(), dir, main.Name, func() error { close(held); <-release; return nil })
	}()
	<-held
	if err := observer.scan(context.Background(), dir, main); err != nil {
		t.Fatal(err)
	}
	_, _, departed := noticeKinds(t, dir, main.Name)
	if len(departed) != 0 {
		t.Fatal("locked mailbox accepted notice")
	}
	var pendingID string
	for _, worker := range observer.known {
		if worker.departure != nil {
			pendingID = worker.departure.ID
		}
	}
	close(release)
	<-done
	if pendingID == "" {
		t.Fatal("failed publication lost pending notice identity")
	}
	for range 2 {
		if err := observer.scan(context.Background(), dir, main); err != nil {
			t.Fatal(err)
		}
	}
	_, _, departed = noticeKinds(t, dir, main.Name)
	if len(departed) != 1 || departed[0].ID != pendingID {
		t.Fatal("retry duplicated or renamed the departure notice")
	}
}

func TestObservedCompactionSurvivesDepartureBeforeNextScan(t *testing.T) {
	dir := stateDir(t)
	main := availabilityPeer(t, dir, "main-fixture", role.Main.ID, 1, true)
	peer := availabilityPeer(t, dir, "worker-fixture", role.General.ID, 2, true)
	noticeState(t, dir, peer, "working", 0, nil)
	observer := newSessionNotices()
	if err := observer.scan(context.Background(), dir, main); err != nil {
		t.Fatal(err)
	}
	noticeState(t, dir, peer, "idle", 1, []sessionstate.CompactionEvent{{Sequence: 1, ObservedAt: time.Now()}})
	if err := registry.Remove(dir, peer.Name); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := observer.scan(context.Background(), dir, main); err != nil {
			t.Fatal(err)
		}
	}
	_, completed, departed := noticeKinds(t, dir, main.Name)
	if len(completed) != 1 || len(departed) != 1 || completed[0].FromEpoch != peer.Epoch() {
		t.Fatal("departure lost or duplicated the already observed completion")
	}
}

func TestStateNoticeObserverStaysInRoomAndDoesNotNotifyItself(t *testing.T) {
	dir := stateDir(t)
	main := availabilityPeer(t, dir, "main-fixture", role.Main.ID, 1, true)
	other := filepath.Join(filepath.Dir(dir), "other")
	if err := state.EnsureSubdir(other); err != nil {
		t.Fatal(err)
	}
	foreign := availabilityPeer(t, other, "foreign-worker", role.General.ID, 2, true)
	noticeState(t, other, foreign, "idle", 1, []sessionstate.CompactionEvent{{Sequence: 1, ObservedAt: time.Now()}})
	noticeState(t, dir, main, "idle", 1, []sessionstate.CompactionEvent{{Sequence: 1, ObservedAt: time.Now()}})
	observer := newSessionNotices()
	if err := observer.scan(context.Background(), dir, main); err != nil {
		t.Fatal(err)
	}
	if err := registry.Remove(other, foreign.Name); err != nil {
		t.Fatal(err)
	}
	if err := observer.scan(context.Background(), dir, main); err != nil {
		t.Fatal(err)
	}
	if len(availabilityFiles(t, dir, main.Name)) != 0 {
		t.Fatal("cross-room or self notice appeared")
	}
	if err := registry.Remove(dir, main.Name); err != nil {
		t.Fatal(err)
	}
	if err := observer.scan(context.Background(), dir, main); err == nil {
		t.Fatal("ended main observer remained authorized")
	}
}
