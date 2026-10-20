// Package launch is the host's part of starting a harness that is not the
// adapter's to decide: the run's environment, the version read behind the gate
// each adapter declares, the bounded probe that reads it, and the aliases a
// short name expands to.
package launch

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// ExecutableEnv carries the path of the rewake that started the run, for a
// tool of the harness's adapter to run the same rewake by argv rather than
// whichever one PATH finds first. Set at every launch, replacing an inherited
// value.
const ExecutableEnv = "REWAKE_EXECUTABLE"

// RoleEnv carries the run's role id, for a harness's front end to register
// the tools that are a role's alone — main's worktree_list and worktree_land —
// without a round trip. It chooses what is offered, not who may do it: the CLI
// commands behind the tools are open to any session.
const RoleEnv = "REWAKE_ROLE"

// SessionEnv returns the environment for a harness: the current one, with the
// variables the adapter says its runs must not inherit removed — the markers a
// parent agent session exports to its children — and the rewake variables
// added. The host builds it; the adapter starts its run with it.
func SessionEnv(request adapter.LaunchRequest, strip []string) []string {
	drop := map[string]bool{
		state.SessionEnv: true,
		state.EpochEnv:   true,
		state.DirEnv:     true,
		state.RoomEnv:    true,
		ExecutableEnv:    true,
		RoleEnv:          true,
		// The adapter sets it for a run its template lets enter a
		// checkout, never one inherited from a session that may.
		adapter.WorktreeEnterEnv: true,
	}
	for _, name := range strip {
		drop[name] = true
	}

	env := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		name, value, found := strings.Cut(entry, "=")
		if found && drop[name] {
			continue
		}
		if found && name == "PATH" {
			entry = "PATH=" + pathWithSelf(value)
		}
		env = append(env, entry)
	}
	room := request.Room
	if room == "" {
		room = state.DefaultRoom
	}
	if executable, err := os.Executable(); err == nil {
		env = append(env, ExecutableEnv+"="+executable)
	}
	if request.Role.ID != "" {
		env = append(env, RoleEnv+"="+request.Role.ID)
	}
	return append(env,
		state.RoomEnv+"="+room,
		state.SessionEnv+"="+request.Name,
		state.EpochEnv+"="+request.Epoch,
		state.DirEnv+"="+request.Dir,
	)
}

// ProbeEnv is the environment a probe before the claim runs in: the one the
// harness will have, less the run's values — no run exists yet — and less
// anything a tool call's child would carry, so a probe never passes for one.
func ProbeEnv() []string {
	var env []string
	for _, entry := range os.Environ() {
		name, value, found := strings.Cut(entry, "=")
		switch {
		case !found:
		case name == state.SessionEnv, name == state.EpochEnv, name == state.DirEnv, name == state.RoomEnv, name == ExecutableEnv, name == RoleEnv, name == adapter.WorktreeEnterEnv:
			continue
		case strings.HasPrefix(name, "REWAKE_BRIDGE_"):
			continue
		case name == "PATH":
			entry = "PATH=" + pathWithSelf(value)
		}
		env = append(env, entry)
	}
	return env
}

// pathWithSelf makes sure the agent can run the same rewake that started it.
// Found by a live run: the agent was told to answer with rewake send, tried, and
// got "command not found" because the binary was not on its PATH.
func pathWithSelf(path string) string {
	executable, err := os.Executable()
	if err != nil {
		return path
	}
	dir := filepath.Dir(executable)
	for _, entry := range filepath.SplitList(path) {
		if entry == dir {
			return path
		}
	}
	return dir + string(os.PathListSeparator) + path
}
