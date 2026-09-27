package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/grant"
	"github.com/iiiokojiadbi/rewake/internal/grantauth"
	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/harness/claude"
	"github.com/iiiokojiadbi/rewake/internal/proc"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// The hook asks its wrapper from what the environment says alone: a session
// whose record is gone, or was rewritten by a worker, is still answered by the
// wrapper that runs it.
func TestTheGrantHookAsksItsWrapperWithoutTheRegistry(t *testing.T) {
	t.Setenv(state.DirEnv, filepath.Join(t.TempDir(), "state"))
	t.Setenv(state.RoomEnv, "")
	dir, err := state.Dir()
	if err != nil {
		t.Fatal(err)
	}
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	epoch := registry.Session{ServicePID: os.Getpid(), ServiceStart: start}.Epoch()
	t.Setenv(state.SessionEnv, "worker-claude")
	t.Setenv(state.EpochEnv, epoch)

	keeper, err := grantauth.Keep(state.KeeperAddress(dir, "worker-claude", epoch), os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	keeper.Decide = func(call json.RawMessage, entries []grant.Entry) grantauth.Decision {
		return claude.DecideGrant(call, entries)
	}
	keeper.Settled = func(string) bool { return false }
	ctx, cancel := context.WithCancel(context.Background())
	go keeper.Serve(ctx)
	t.Cleanup(func() { cancel(); keeper.Close() })
	lib, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := keeper.Grant("m1", []string{lib}, time.Now()); err != nil {
		t.Fatal(err)
	}

	payload, _ := json.Marshal(map[string]any{
		"hook_event_name": "PermissionRequest", "tool_name": "Write", "permission_mode": "acceptEdits",
		"cwd": t.TempDir(), "tool_input": map[string]string{"file_path": filepath.Join(lib, "a.txt"), "content": "x"},
	})
	stdin := filepath.Join(t.TempDir(), "payload")
	if err := os.WriteFile(stdin, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	saved := os.Stdin
	file, err := os.Open(stdin)
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = file
	t.Cleanup(func() { os.Stdin = saved; _ = file.Close() })

	code, out, errOut := run(harness.GrantHook)
	if code != ExitOK || !strings.Contains(out, `"addDirectories"`) || !strings.Contains(out, lib) {
		t.Fatalf("exit %d, answered %q %q", code, out, errOut)
	}
	if entries := keeper.Entries(); len(entries) != 1 || entries[0].Outcome != grant.Granted {
		t.Fatalf("kept %v", entries)
	}
}
