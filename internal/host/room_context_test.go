package host

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/brief"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

type contextHarness struct {
	fakeHarness
	launch adapter.LaunchRequest
}

func (h *contextHarness) Launch(request adapter.LaunchRequest) (adapter.LaunchPlan, error) {
	h.launch = request
	return h.fakeHarness.Launch(request)
}

func TestTheHarnessReceivesItsRoomAndDefaultGeneral(t *testing.T) {
	t.Setenv(state.RoomEnv, "red")
	dir := stateDir(t)
	out := filepath.Join(t.TempDir(), "environment")
	fake := &contextHarness{fakeHarness: fakeHarness{script: `printf '%s\n%s\n' "$REWAKE_DIR" "$REWAKE_ROOM" > ` + out}}
	t.Setenv(state.RoomEnv, "parent-room")
	if _, err := Run(context.Background(), Request{Dir: dir, Name: "api", Harness: fake, Intro: true}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != os.Getenv(state.DirEnv)+"\nred\n" {
		t.Errorf("child environment=%q", raw)
	}
	if fake.launch.Room != "red" || fake.launch.Role.ID != "general" || fake.launch.Role.Silent {
		t.Errorf("launch context=%+v", fake.launch)
	}
	intro := brief.Intro(fake.launch.BriefContext())
	for _, text := range []string{`room "red"`, "role general", "default general", "no role flag"} {
		if !strings.Contains(intro, text) {
			t.Errorf("intro lacks %q: %s", text, intro)
		}
	}
}

func TestADeadMainDoesNotSelectAnImplicitMain(t *testing.T) {
	dir := stateDir(t)
	dead := registry.Session{Name: "old", Role: "main", ServicePID: 999999999, ServiceStart: 1}
	if err := registry.Publish(dir, dead); err != nil {
		t.Fatal(err)
	}
	session, err := claimRole(t, dir, "new", role.Role{})
	if err != nil || session.Role != "general" {
		t.Fatalf("default role=%+v %v", session, err)
	}
}

func TestEachRoomHasItsOwnMain(t *testing.T) {
	dir := stateDir(t)
	first, err := claimRole(t, dir, "api", role.Main)
	if err != nil {
		t.Fatal(err)
	}
	other := buildtest.Room(t, os.Getenv(state.DirEnv), "other")
	second, err := claimRole(t, other, "api", role.Main)
	if err != nil || second.Role != "main" || first.Room == second.Room {
		t.Errorf("independent mains=%+v %+v err=%v", first, second, err)
	}
}

// A setting the launch skipped as obsolete reaches the session's briefing, so
// main — which starts on what is valid — can repair what it runs under
// (docs/work-modes.md#the-old-worker-types).
func TestASkippedSettingReachesTheLaunchBriefing(t *testing.T) {
	dir := stateDir(t)
	fake := &contextHarness{fakeHarness: fakeHarness{script: "exit 0"}}
	notice := `the project alias file at /work/.rewake/aliases.toml:3: [alias.w] still has the obsolete type field, so that template is left out until it is fixed`
	if _, err := Run(context.Background(), Request{Dir: dir, Name: "api", Harness: fake, Intro: true, Migrations: []string{notice}}); err != nil {
		t.Fatal(err)
	}
	if len(fake.launch.Migrations) != 1 || fake.launch.Migrations[0] != notice {
		t.Fatalf("the launch request: %v", fake.launch.Migrations)
	}
	intro := brief.Intro(fake.launch.BriefContext())
	for _, want := range []string{"skipped as obsolete", notice} {
		if !strings.Contains(intro, want) {
			t.Errorf("the briefing lacks %q: %s", want, intro)
		}
	}
}
