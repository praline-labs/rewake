package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/adapter/claude/modhost"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// helloFrom posts a hello to the mod socket as this process, which the test
// names the run's harness.
func helloFrom(t *testing.T, socket string) modhost.HelloAnswer {
	t.Helper()
	client := http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socket)
		},
	}}
	body, _ := json.Marshal(modhost.HelloRequest{LoadID: "load-1", SessionID: "conversation-1", HarnessVersion: "2.1.296"})
	answer, err := client.Post("http://rewake"+modhost.PathHello, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = answer.Body.Close() }()
	var hello modhost.HelloAnswer
	if answer.StatusCode != http.StatusOK || json.NewDecoder(answer.Body).Decode(&hello) != nil {
		t.Fatalf("the hello was answered %d", answer.StatusCode)
	}
	return hello
}

// A run's inbox lane is live from its start, alone until the mod's hello is
// admitted; the turn boundary joins it then, and spaces with it only on a run
// whose role opens them, which the hello's answer tells the mod.
func TestTheSetIsWakeUntilTheHelloThenTheBoundaryAndSpacesOnMain(t *testing.T) {
	for _, test := range []struct {
		role  role.Role
		hosts bool
	}{{role.Main, true}, {role.Write, false}} {
		t.Run(test.role.ID, func(t *testing.T) {
			base, err := os.MkdirTemp("", "ml")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(base) })
			room := buildtest.Room(t, base, state.DefaultRoom)
			request := adapter.LaunchRequest{
				Name: "api", Epoch: "e1", Dir: room, RoomDir: room, Socket: filepath.Join(room, "sock", "api.e1.sock"),
				RunDir: filepath.Join(room, "run", "api.e1"), Role: test.role,
			}
			if err := state.EnsureSubdir(request.RunDir); err != nil {
				t.Fatal(err)
			}
			plan, err := New().Launch(request)
			if err != nil {
				t.Fatal(err)
			}
			var mu sync.Mutex
			var reports [][]adapter.Capability
			plan.Live.ReportLive(func(set []adapter.Capability) {
				mu.Lock()
				reports = append(reports, slices.Clone(set))
				mu.Unlock()
			})
			if err := plan.Service.Start(context.Background(), func(string) {}); err != nil {
				t.Fatal(err)
			}
			defer plan.Service.Close()
			start, err := proc.StartTime(os.Getpid())
			if err != nil {
				t.Fatal(err)
			}
			plan.Service.(adapter.HarnessWatcher).HarnessStarted(adapter.RunProcess{PID: os.Getpid(), Start: start})
			if answer := helloFrom(t, modSocketPath(request)); answer.HostsSpaces != test.hosts {
				t.Fatalf("the hello was answered hostsSpaces %v, the role opens spaces: %v", answer.HostsSpaces, test.hosts)
			}
			proven := []adapter.Capability{adapter.WakeCapability, adapter.TurnBoundaryCapability}
			if test.hosts {
				proven = append(proven, adapter.SpacesCapability)
			}
			want := [][]adapter.Capability{{adapter.WakeCapability}, proven}
			mu.Lock()
			defer mu.Unlock()
			if !slices.EqualFunc(reports, want, slices.Equal) {
				t.Fatalf("the run reported %v, want %v", reports, want)
			}
		})
	}
}

// A lapse withdraws the turn boundary, and spaces on main, leaving the inbox
// lane live, and a new hello restores them; a report of an earlier
// transition heard after a later one changes nothing, and nothing is heard
// after the close.
func TestALapseLeavesWakeAndANewHelloRestoresTheSet(t *testing.T) {
	for _, test := range []struct {
		role  role.Role
		hosts bool
	}{{role.Main, true}, {role.Write, false}} {
		t.Run(test.role.ID, func(t *testing.T) {
			run := &modRun{lane: newLane("", "", false), server: modhost.New(modhost.Config{}), hostsSpaces: test.hosts}
			var reports [][]adapter.Capability
			run.ReportLive(func(set []adapter.Capability) { reports = append(reports, slices.Clone(set)) })
			proven := []adapter.Capability{adapter.WakeCapability, adapter.TurnBoundaryCapability}
			if test.hosts {
				proven = append(proven, adapter.SpacesCapability)
			}
			wake := []adapter.Capability{adapter.WakeCapability}
			run.transition(1, true)
			run.transition(2, false)
			run.transition(3, true)
			run.transition(2, false)
			run.transition(3, false)
			run.Close()
			run.transition(4, false)
			want := [][]adapter.Capability{proven, wake, proven}
			if !slices.EqualFunc(reports, want, slices.Equal) {
				t.Fatalf("the run reported %v, want %v", reports, want)
			}
		})
	}
}
