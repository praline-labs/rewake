package host

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/core/session"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// A compaction main was told of before a switch is not told again by the
// next image, though the notice has left the mailbox; a scan from nothing
// would tell it twice.
func TestMainsNoticesGoOverWithItsRun(t *testing.T) {
	dir := stateDir(t)
	main := availabilityPeer(t, dir, "main-fixture", role.Main.ID, 1, true)
	peer := availabilityPeer(t, dir, "worker-fixture", role.General.ID, 2, true)
	observer := newSessionNotices()
	if err := observer.scan(context.Background(), dir, main); err != nil {
		t.Fatal(err)
	}
	noticeState(t, dir, peer, "idle", 1, []session.CompactionEvent{{Sequence: 1, ObservedAt: time.Now()}})
	if err := observer.scan(context.Background(), dir, main); err != nil {
		t.Fatal(err)
	}
	available, completed, _ := noticeKinds(t, dir, main.Name)
	if len(available) != 1 || len(completed) != 1 {
		t.Fatalf("%d notices of the worker and %d of its compaction before the switch", len(available), len(completed))
	}
	for _, m := range append(available, completed...) {
		if err := os.Remove(filepath.Join(state.InboxPath(dir, main.Name), m.ID+".json")); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := json.Marshal(Handover{Notices: observer.handed()})
	if err != nil {
		t.Fatal(err)
	}
	var handed Handover
	if err := json.Unmarshal(raw, &handed); err != nil {
		t.Fatal(err)
	}
	if err := noticesFrom(handed.Notices).scan(context.Background(), dir, main); err != nil {
		t.Fatal(err)
	}
	if available, completed, _ := noticeKinds(t, dir, main.Name); len(available)+len(completed) != 0 {
		t.Fatalf("the next image told again: %d availability, %d compaction", len(available), len(completed))
	}
	if err := newSessionNotices().scan(context.Background(), dir, main); err != nil {
		t.Fatal(err)
	}
	if available, completed, _ := noticeKinds(t, dir, main.Name); len(available)+len(completed) == 0 {
		t.Fatal("a scan from nothing does not tell it again either: the test shows nothing")
	}
}

// A drop takes main's notice state while the scan stands still, before its
// first scan as well, and the scan goes on after an exec that did not
// happen.
func TestADropTakesMainsNoticesAndAFailedExecResumesThem(t *testing.T) {
	dir := stateDir(t)
	main := availabilityPeer(t, dir, "main-fixture", role.Main.ID, 1, true)
	handed := &Notices{Known: map[string]KnownWorker{"worker-fixture@2": {Compactions: 4}}}
	ready := make(chan struct{})
	stop, hand := startAvailability(context.Background(), dir, main, handed, ready, func() bool { return true }, nil)
	defer stop()
	r := &reexec{session: main, notices: hand}
	taken, why := r.takeNotices()
	if why != "" || taken == nil || taken.Known["worker-fixture@2"].Compactions != 4 {
		t.Fatalf("the drop before the first scan took %+v, %q", taken, why)
	}
	r.giveNotices()
	close(ready)
	for range 2 {
		if taken, why = r.takeNotices(); why != "" || taken == nil {
			t.Fatalf("the scan was not taken again: %q", why)
		}
		r.giveNotices()
	}
}
