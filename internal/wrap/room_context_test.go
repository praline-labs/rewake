package wrap

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/brief"
	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/role"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

type contextHarness struct {
	fakeHarness
	launch harness.LaunchRequest
}

func (h *contextHarness) Launch(request harness.LaunchRequest) (harness.LaunchPlan, error) {
	h.launch = request
	return h.fakeHarness.Launch(request)
}

func TestTheHarnessReceivesItsRoomAndElectedRole(t *testing.T) {
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
	if string(raw) != state.RootForRoom(dir)+"\nred\n" {
		t.Errorf("child environment=%q", raw)
	}
	if fake.launch.Room != "red" || fake.launch.Role.ID != "main" || !fake.launch.Role.Silent || !fake.launch.Role.GitWrite {
		t.Errorf("launch context=%+v", fake.launch)
	}
	intro := brief.Intro(fake.launch.BriefContext())
	for _, text := range []string{`room "red"`, "role is main", "automatically", "no live main"} {
		if !strings.Contains(intro, text) {
			t.Errorf("intro lacks %q: %s", text, intro)
		}
	}
}

func TestADeadMainDoesNotBlockElection(t *testing.T) {
	dir := stateDir(t)
	dead := registry.Session{Name: "old", Role: "main", ServicePID: 999999999, ServiceStart: 1}
	if err := registry.Publish(dir, dead); err != nil {
		t.Fatal(err)
	}
	session, err := claimRole(t, dir, "new", role.Role{})
	if err != nil || session.Role != "main" {
		t.Fatalf("election=%+v %v", session, err)
	}
}

func TestEachRoomHasItsOwnMain(t *testing.T) {
	dir := stateDir(t)
	first, err := claimRole(t, dir, "api", role.Main)
	if err != nil {
		t.Fatal(err)
	}
	other, err := state.RoomDir(state.RootForRoom(dir), "other")
	if err != nil {
		t.Fatal(err)
	}
	second, err := claimRole(t, other, "api", role.Main)
	if err != nil || second.Role != "main" || first.Room == second.Room {
		t.Errorf("independent mains=%+v %+v err=%v", first, second, err)
	}
}
