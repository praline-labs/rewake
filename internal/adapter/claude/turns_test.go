package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/adapter/claude/modhost"
	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// Claude Code reports every turn's end and holds none open, and its boundary
// is proven only once the harness runs: its mod says hello from inside it.
// A run that loads the mod carries the boundary in its plan, served by the
// same service that proves it.
func TestTheOfferReportsTurnsAfterTheStartAndHoldsNone(t *testing.T) {
	if got := New().Offers().TurnBoundary; got != (adapter.TurnBoundaryOffer{Reports: true, HoldsEnd: false, AfterStart: true}) {
		t.Fatalf("the offer's turn boundary: %+v", got)
	}
	room := shortRoom(t)
	privateHome(t)
	plan, err := New().Launch(adapter.LaunchRequest{
		Name: "api", Epoch: "e1", Dir: room, RoomDir: room, Socket: filepath.Join(room, "sock", "api.e1.sock"),
		RunDir: filepath.Join(room, "runs", "api.e1"), Role: role.Write,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.TurnBoundary == nil || plan.TurnBoundary != plan.Service.(adapter.TurnBoundary) || plan.Live == nil {
		t.Fatalf("the plan's boundary %T, service %T, live %T", plan.TurnBoundary, plan.Service, plan.Live)
	}
}

// shortRoom is a room whose sockets' paths fit a unix socket's limit.
func shortRoom(t *testing.T) string {
	t.Helper()
	base, err := os.MkdirTemp("", "mt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	return buildtest.Room(t, base, state.DefaultRoom)
}

// withheld is what a run's service says it will not prove.
func withheld(t *testing.T, service adapter.Service) string {
	t.Helper()
	withholder, ok := service.(adapter.Withholder)
	if !ok {
		t.Fatalf("the service %T says nothing of what it withholds", service)
	}
	var causes []string
	for _, w := range withholder.Withheld() {
		if w.Capability != adapter.TurnBoundaryCapability {
			t.Fatalf("withheld %s, want the turn boundary", w.Capability)
		}
		causes = append(causes, w.Cause)
	}
	return strings.Join(causes, "; ")
}

// Each cause rewake knows the mod will not be heard by is said before the
// harness starts, naming where it lies, so the host refuses a role that owes
// reports: a mod socket's path past a unix socket's limit, a launch with no
// run directory, mod files that cannot be written. A run that loads the mod
// withholds nothing.
func TestAKnownCauseWithholdsTheBoundary(t *testing.T) {
	room := shortRoom(t)
	base := adapter.LaunchRequest{
		Name: "api", Epoch: "e1", Dir: room, RoomDir: room, Socket: filepath.Join(room, "sock", "api.e1.sock"),
		RunDir: filepath.Join(room, "runs", "api.e1"), Role: role.Write,
	}
	long := base
	dir := "/" + strings.Repeat("d", maxSocketPath-21)
	// The inbox socket fits; the mod's, in its own form or hashed, does not.
	long.Socket = filepath.Join(dir, strings.Repeat("a", 12)+".sock")
	noRun := base
	noRun.RunDir = ""
	for name, c := range map[string]struct {
		request adapter.LaunchRequest
		cause   string
	}{
		"a long mod socket": {long, "longer than a unix socket allows"},
		"no run directory":  {noRun, "no run directory"},
	} {
		plan, err := New().Launch(c.request)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := withheld(t, plan.Service); !strings.Contains(got, c.cause) || !strings.Contains(got, "rewake mod") {
			t.Fatalf("%s: withheld %q", name, got)
		}
		if plan.TurnBoundary != nil || indexOf(plan.Args, pluginDirFlag) >= 0 {
			t.Fatalf("%s: a run without its mod plans a boundary or loads a mod", name)
		}
	}

	blocked := base
	blocked.RunDir = filepath.Join(room, "runs", "blocked")
	if err := state.EnsureSubdir(filepath.Dir(blocked.RunDir)); err != nil {
		t.Fatal(err)
	}
	// A file where the run's directory would be: no mod is written under it.
	if err := state.WriteFile(blocked.RunDir, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := New().Launch(blocked)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Service.Start(context.Background(), func(string) {}); err != nil {
		t.Fatalf("a run whose mod cannot be written failed to start: %v", err)
	}
	defer plan.Service.Close()
	if got := withheld(t, plan.Service); !strings.Contains(got, "could not be written") {
		t.Fatalf("mod files not written: withheld %q", got)
	}

	plan, err = New().Launch(base)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.EnsureSubdir(base.RunDir); err != nil {
		t.Fatal(err)
	}
	if err := plan.Service.Start(context.Background(), func(string) {}); err != nil {
		t.Fatal(err)
	}
	defer plan.Service.Close()
	if got := withheld(t, plan.Service); got != "" {
		t.Fatalf("a run that loads its mod withheld %q", got)
	}
}

// A run's end, sent by its mod after its hello, reaches the core through the
// handler the host gave the run's boundary, under its end's event.
func TestARunsEndReachesTheCoreThroughItsMod(t *testing.T) {
	room := shortRoom(t)
	request := adapter.LaunchRequest{
		Name: "api", Epoch: "e1", Dir: room, RoomDir: room, Socket: filepath.Join(room, "sock", "api.e1.sock"),
		RunDir: filepath.Join(room, "run", "api.e1"), Role: role.Write,
	}
	if err := state.EnsureSubdir(request.RunDir); err != nil {
		t.Fatal(err)
	}
	plan, err := New().Launch(request)
	if err != nil {
		t.Fatal(err)
	}
	published := make(chan adapter.TurnEnd, 1)
	plan.TurnBoundary.ReportTurns(adapter.TurnHandler{
		Capture: func() *mail.ReadBoundary { return &mail.ReadBoundary{Mailbox: "api", Epoch: "e1"} },
		Publish: func(_ context.Context, end adapter.TurnEnd) error { published <- end; return nil },
	})
	plan.Live.ReportLive(func([]adapter.Capability) {})
	if err := plan.Service.Start(context.Background(), func(string) {}); err != nil {
		t.Fatal(err)
	}
	defer plan.Service.Close()
	start, _ := proc.StartTime(os.Getpid())
	plan.Service.(adapter.HarnessWatcher).HarnessStarted(adapter.RunProcess{PID: os.Getpid(), Start: start})
	helloFrom(t, modSocketPath(request))
	answer := "the suite is green"
	body, _ := json.Marshal(modhost.TurnEndRequest{
		LoadID: "load-1", Conversation: "conversation-1", TurnID: "turn-1", EndID: "end-1",
		Reason: modhost.EndReasonAnswered, Answer: &answer,
	})
	client := http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", modSocketPath(request))
		},
	}}
	reply, err := client.Post("http://rewake"+modhost.PathTurnEnd, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	_ = reply.Body.Close()
	if reply.StatusCode != http.StatusOK {
		t.Fatalf("the end was answered %d", reply.StatusCode)
	}
	if end := <-published; end.Event != "claude/end-1" || end.Answer != answer || end.Reason != adapter.Answered {
		t.Fatalf("the core was handed %+v", end)
	}
}

// A run is proven ended only on proof: its wrapper's process replaced; one
// alive, or an epoch that cannot be read, is not.
func TestARunIsProvenEndedOnlyOnProof(t *testing.T) {
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if runEnded(registry.RunEpoch(os.Getpid(), start, "")) {
		t.Fatal("a run alive was proven ended")
	}
	if !runEnded(registry.RunEpoch(os.Getpid(), start+1, "")) {
		t.Fatal("a run whose wrapper was replaced was not proven ended")
	}
	if runEnded("not-an-epoch") {
		t.Fatal("an epoch that cannot be read was proven ended")
	}
}
