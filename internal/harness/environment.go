package harness

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/praline-labs/rewake/internal/state"
)

// SessionEnv returns the environment for a harness: the current one, with the
// markers of a parent agent session removed and the rewake variables added.
func SessionEnv(request LaunchRequest, strip []string) []string {
	drop := map[string]bool{
		state.SessionEnv: true,
		state.EpochEnv:   true,
		state.DirEnv:     true,
		state.RoomEnv:    true,
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
		case name == state.SessionEnv, name == state.EpochEnv, name == state.DirEnv, name == state.RoomEnv:
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
