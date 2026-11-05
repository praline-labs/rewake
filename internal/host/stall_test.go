package host

import (
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/core/session"
	"github.com/praline-labs/rewake/internal/host/delivery"
)

// reportedTelemetry is an adapter whose session reports the activity in
// activity, fresh, or nothing when it is empty.
type reportedTelemetry struct{ activity *string }

func (r reportedTelemetry) SessionState() session.Snapshot {
	if *r.activity == "" {
		return session.Snapshot{}
	}
	activity, now := *r.activity, time.Now()
	return session.Snapshot{Activity: &activity, ActivityAt: &now, ActivityFresh: true, Fresh: true, PublishedAt: &now}
}

func stallServer(t *testing.T, dir, roleID string, telemetry adapter.Telemetry) *delivery.Server {
	t.Helper()
	r := &run{
		request: Request{Dir: dir}, session: registry.Session{Name: "api", Role: roleID}, epoch: "1.1",
		plan: adapter.LaunchPlan{ServicePlan: adapter.ServicePlan{Telemetry: telemetry}}, live: newLiveSet(dir, "api", "1.1"),
	}
	return r.deliveryServer(make(chan struct{}), nil, nil)
}

// A run whose turns are reported is woken when it stalls under a pending
// mark, by the bound, and judged idle by its telemetry; main, whose turns are
// reported to nobody, and a run that publishes no state are never woken.
func TestOnlyAReportingRunWithTelemetryIsWokenWhenItStalls(t *testing.T) {
	dir := stateDir(t)
	none := ""
	if s := stallServer(t, dir, role.Main.ID, reportedTelemetry{&none}); s.Stall != 0 || s.StallSnapshot != nil {
		t.Errorf("main's server wakes it after %s", s.Stall)
	}
	if s := stallServer(t, dir, role.General.ID, nil); s.Stall != 0 {
		t.Errorf("a run that publishes no state is woken after %s", s.Stall)
	}
	activity := ""
	s := stallServer(t, dir, role.General.ID, reportedTelemetry{&activity})
	if s.Stall != delivery.StallBound || s.StallSnapshot == nil {
		t.Fatalf("a worker's server: bound %s, idle %v", s.Stall, s.StallSnapshot != nil)
	}
	if s.StallSnapshot().Judge().Activity == session.ActivityIdle {
		t.Error("a run that reported nothing is taken for idle")
	}
	for state, want := range map[string]bool{session.ActivityIdle: true, session.ActivityWorking: false} {
		activity = state
		if got := s.StallSnapshot().Judge().Activity == session.ActivityIdle; got != want {
			t.Errorf("a run %s is idle: %v", state, got)
		}
	}
}

// A turn the adapter already reports running is not idle, though the last
// publication on disk, a quarter second old at most, still says idle: neither
// real harness times the start of the turn, so nothing else tells the wake.
func TestAStallIsJudgedByTheAdaptersStateNotTheLastPublication(t *testing.T) {
	dir := stateDir(t)
	idle, now := session.ActivityIdle, time.Now()
	if err := session.Save(dir, "api", "1.1", session.Snapshot{Activity: &idle, ActivityAt: &now, ActivityFresh: true, Fresh: true, PublishedAt: &now}); err != nil {
		t.Fatal(err)
	}
	working := session.ActivityWorking
	if s := stallServer(t, dir, role.General.ID, reportedTelemetry{&working}); s.StallSnapshot().Judge().Activity == session.ActivityIdle {
		t.Fatal("a run its adapter reports working is woken by the idle its last publication said")
	}
	if err := session.Save(dir, "api", "1.1", session.Snapshot{Activity: &working, ActivityAt: &now, ActivityFresh: true, Fresh: true, PublishedAt: &now}); err != nil {
		t.Fatal(err)
	}
	if s := stallServer(t, dir, role.General.ID, reportedTelemetry{&idle}); s.StallSnapshot().Judge().Activity != session.ActivityIdle {
		t.Fatal("a live idle sample was replaced by the working last publication")
	}
}
