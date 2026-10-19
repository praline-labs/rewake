package spacehost

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// WorkerSpaceEnv names, as <room>/<space id>, the space whose worker a launch
// is. A person's launch is always main (docs/v2/worker-profile.md); a launch
// takes a worker's name, role and room only from the record of an opening
// space whose host is its parent.
const WorkerSpaceEnv = "REWAKE_SPACE"

// WorkerTemplateEnv names the alias the space was opened with. A launch
// believes it only beside a WorkerSpaceEnv it believes.
const WorkerTemplateEnv = "REWAKE_SPACE_TEMPLATE"

// WorkerEnv is the environment the worker's launch gets: the space host's,
// less what would make the launch a child of main's session, with REWAKE_DIR
// set to main's base. The session's own variables would make the launch
// refuse as nested (docs/launch.md#no-session-inside-a-session); a
// harness's markers and the bridges' variables point at
// main's harness and transport. REWAKE_DIR is always set, never left to the
// default: main's room may live under any base, and a worker under another
// would be in another room of the same name. The space comes as
// WorkerSpaceEnv, which the host sets itself.
func WorkerEnv(env []string, base string, strip []string) ([]string, error) {
	if !filepath.IsAbs(base) {
		return nil, fmt.Errorf("main's state directory %q is not absolute", base)
	}
	drop := map[string]bool{
		state.SessionEnv: true, state.EpochEnv: true, state.RoomEnv: true, state.DirEnv: true,
		Env: true, WorkerSpaceEnv: true, WorkerTemplateEnv: true,
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
