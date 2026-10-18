package spacehost

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// WorkerEnv is the environment the worker's launch gets: the space host's,
// less what would make the launch a child of main's session, with REWAKE_DIR
// set to main's base. The session's own variables would make the launch
// refuse as nested (docs/launch.md#no-session-inside-a-session); the room
// comes as --room; a harness's markers and the bridges' variables point at
// main's harness and transport. REWAKE_DIR is always set, never left to the
// default: main's room may live under any base, and a worker under another
// would be in another room of the same name.
func WorkerEnv(env []string, base string, strip []string) ([]string, error) {
	if !filepath.IsAbs(base) {
		return nil, fmt.Errorf("main's state directory %q is not absolute", base)
	}
	drop := map[string]bool{
		state.SessionEnv: true, state.EpochEnv: true, state.RoomEnv: true, state.DirEnv: true,
		Env: true,
	}
	for _, name := range strip {
		drop[name] = true
	}
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if drop[name] || strings.HasPrefix(name, "REWAKE_BRIDGE_") {
			continue
		}
		out = append(out, kv)
	}
	return append(out, state.DirEnv+"="+base), nil
}
