package wrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/proc"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/role"
	"github.com/iiiokojiadbi/rewake/internal/sessionstate"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

func TestReviewUnreadableProcessIdentityDoesNotAnnounceDeparture(t *testing.T) {
	if runProcessFixture(t) {
		return
	}
	dir := stateDir(t)
	main := availabilityPeer(t, dir, "main-fixture", role.Main.ID, 1, true)
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		t.Fatal(err)
	}
	peer := availabilityPeer(t, dir, "worker-fixture", role.General.ID, start, true)
	peer.PIDNamespace = "review-namespace"
	if err := registry.Update(dir, peer); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for _, path := range []string{"self/ns", fmt.Sprint(peer.ServicePID)} {
		if err := os.MkdirAll(filepath.Join(root, path), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(peer.PIDNamespace, filepath.Join(root, "self/ns/pid")); err != nil {
		t.Fatal(err)
	}
	statPath := filepath.Join(root, fmt.Sprint(peer.ServicePID), "stat")
	if err := os.WriteFile(statPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	original := proc.Default
	proc.Default = proc.Reader{Root: root}
	t.Cleanup(func() { proc.Default = original })
	noticeState(t, dir, peer, "working", 0, nil)
	observer := newSessionNotices()
	if err := observer.scan(context.Background(), dir, main); err != nil {
		t.Fatal(err)
	}
	if len(observer.known) != 1 || !peer.Alive() {
		t.Fatal("control did not establish a known live worker")
	}
	if err := os.Chmod(statPath, 0); err != nil {
		t.Fatal(err)
	}
	_, identityErr := proc.StartTime(peer.ServicePID)
	if !os.IsPermission(identityErr) {
		t.Fatalf("fixture must produce uncertain EACCES, got %v", identityErr)
	}
	if err := observer.scan(context.Background(), dir, main); err != nil {
		t.Fatal(err)
	}
	_, _, departed := noticeKinds(t, dir, main.Name)
	if err := os.Chmod(statPath, 0o600); err != nil {
		t.Fatal(err)
	}
	if !peer.Alive() {
		t.Fatal("restoring readability did not restore the same process identity")
	}
	if len(departed) != 0 {
		t.Fatalf("uncertain process identity produced %d departure notice(s): %s", len(departed), departed[0].Text)
	}
}

func TestReviewCompactionRetryTailGapAndDeparture(t *testing.T) {
	dir := stateDir(t)
	main := availabilityPeer(t, dir, "main-fixture", role.Main.ID, 1, true)
	peer := availabilityPeer(t, dir, "worker-fixture", role.General.ID, 2, true)
	noticeState(t, dir, peer, "working", 0, nil)
	observer := newSessionNotices()
	if err := observer.scan(context.Background(), dir, main); err != nil {
		t.Fatal(err)
	}
	observed := time.Now()
	events := []sessionstate.CompactionEvent{{Sequence: 7, ObservedAt: observed}, {Sequence: 8, ObservedAt: observed.Add(time.Nanosecond)}}
	noticeState(t, dir, peer, "working", 8, events)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := state.WithMailboxLock(context.Background(), dir, main.Name, func() error {
		return observer.scan(ctx, dir, main)
	}); err != nil {
		t.Fatal(err)
	}
	_, completed, _ := noticeKinds(t, dir, main.Name)
	if len(completed) != 0 {
		t.Fatal("failed publication incorrectly queued a notice")
	}
	if err := registry.Remove(dir, peer.Name); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if err := observer.scan(context.Background(), dir, main); err != nil {
			t.Fatal(err)
		}
	}
	_, completed, departed := noticeKinds(t, dir, main.Name)
	if len(completed) != 2 || len(departed) != 1 || completed[0].Compaction.Count != 7 || completed[1].Compaction.Count != 8 {
		t.Fatalf("gap/retry/departure: completions=%d departures=%d", len(completed), len(departed))
	}
	if completed[0].ID != compactionMessage(main, peer, events[0]).ID || completed[1].ID != compactionMessage(main, peer, events[1]).ID {
		t.Fatal("retry changed compaction identity")
	}
	if *departed[0].SenderState.Compactions != 8 || departed[0].SenderState.Fresh {
		t.Fatal("departure lost the truthful latest count or stale marker")
	}
	for _, message := range append(completed, departed...) {
		if err := os.Remove(filepath.Join(state.InboxPath(dir, main.Name), message.ID+".json")); err != nil {
			t.Fatal(err)
		}
	}
	if err := observer.scan(context.Background(), dir, main); err != nil {
		t.Fatal(err)
	}
	_, completed, departed = noticeKinds(t, dir, main.Name)
	if len(completed) != 0 || len(departed) != 0 {
		t.Fatal("retention recreated a completed or departure notice")
	}
}
