package host

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/host/endpoint"
	"github.com/praline-labs/rewake/internal/host/launch"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// TestChildBaseRoundTripAndGrantAuthority holds the round trip of the base
// (docs/v2/stage3-state.md#the-roots): the wrapper exports the base, not its
// root, and a shell of the session and a tool call's child, each resolving
// the room from what they were given, find the wrapper's room byte for byte,
// with one v2 in it — so the grant authority, whose address hashes the room,
// is the one the wrapper listens at.
func TestChildBaseRoundTripAndGrantAuthority(t *testing.T) {
	t.Setenv(state.RoomEnv, "red")
	dir := stateDir(t)
	base := os.Getenv(state.DirEnv)
	out := filepath.Join(t.TempDir(), "environment")
	fake := &contextHarness{fakeHarness: fakeHarness{script: `printf '%s\n%s\n' "$REWAKE_DIR" "$REWAKE_ROOM" > ` + out}}
	if _, err := Run(context.Background(), Request{Dir: dir, Name: "api", Harness: fake}); err != nil {
		t.Fatal(err)
	}
	resolve := func(who string, values map[string]string) {
		t.Helper()
		if values[state.DirEnv] != base {
			t.Errorf("%s was given %s=%q, want the base %q", who, state.DirEnv, values[state.DirEnv], base)
		}
		t.Setenv(state.DirEnv, values[state.DirEnv])
		t.Setenv(state.RoomEnv, values[state.RoomEnv])
		got, err := state.Dir()
		if err != nil || got != dir || strings.Count(got, string(filepath.Separator)+"v2"+string(filepath.Separator)) != 1 {
			t.Fatalf("%s resolves %q, %v; the wrapper's room is %q", who, got, err, dir)
		}
		if state.AuthorityAddress(got, fake.launch.Epoch) != state.AuthorityAddress(dir, fake.launch.Epoch) {
			t.Fatalf("%s asks another grant authority than the wrapper's", who)
		}
	}

	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("the shell's environment: %q", raw)
	}
	resolve("a shell of the session", map[string]string{state.DirEnv: lines[0], state.RoomEnv: lines[1]})

	harness := map[string]string{}
	for _, entry := range launch.SessionEnv(fake.launch, nil) {
		if key, value, ok := strings.Cut(entry, "="); ok {
			harness[key] = value
		}
	}
	child := map[string]string{}
	for _, entry := range endpoint.ChildEnv(func(key string) string { return harness[key] }) {
		if key, value, ok := strings.Cut(entry, "="); ok {
			child[key] = value
		}
	}
	resolve("a tool call's child", child)
}
