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

type fixedTelemetry struct{}

func (fixedTelemetry) SessionState() session.Snapshot { return session.Snapshot{} }

// A run whose turns are reported is woken when it stalls under a pending
// mark, by the bound, and judged idle by the snapshot its telemetry
// publishes; main, whose turns are reported to nobody, and a run that
// publishes no state are never woken.
func TestOnlyAReportingRunWithTelemetryIsWokenWhenItStalls(t *testing.T) {
	dir := stateDir(t)
	server := func(roleID string, telemetry adapter.Telemetry) *delivery.Server {
		r := &run{
			request: Request{Dir: dir}, session: registry.Session{Name: "api", Role: roleID}, epoch: "1.1",
			plan: adapter.LaunchPlan{ServicePlan: adapter.ServicePlan{Telemetry: telemetry}}, live: newLiveSet(dir, "api", "1.1"),
		}
		return r.deliveryServer(make(chan struct{}), nil, nil)
	}
	if s := server(role.Main.ID, fixedTelemetry{}); s.Stall != 0 || s.Idle != nil {
		t.Errorf("main's server wakes it after %s", s.Stall)
	}
	if s := server(role.General.ID, nil); s.Stall != 0 {
		t.Errorf("a run that publishes no state is woken after %s", s.Stall)
	}
	s := server(role.General.ID, fixedTelemetry{})
	if s.Stall != delivery.StallBound || s.Idle == nil {
		t.Fatalf("a worker's server: bound %s, idle %v", s.Stall, s.Idle != nil)
	}
	if s.Idle() {
		t.Error("a run with no snapshot is taken for idle")
	}
	now := time.Now()
	for activity, want := range map[string]bool{session.ActivityIdle: true, session.ActivityWorking: false} {
		snapshot := session.Snapshot{Activity: &activity, ActivityAt: &now, ActivityFresh: true, Fresh: true, PublishedAt: &now}
		if err := session.Save(dir, "api", "1.1", snapshot); err != nil {
			t.Fatal(err)
		}
		if got := s.Idle(); got != want {
			t.Errorf("a run %s is idle: %v", activity, got)
		}
	}
}
