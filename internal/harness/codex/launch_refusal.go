package codex

import (
	"fmt"
	"strings"

	"github.com/iiiokojiadbi/rewake/internal/harness"
)

// grantAdvice is where a directory beyond the launch's goes instead.
const grantAdvice = "the terminal attaches to a server rewake starts, and from Codex 0.156 on it refuses this beside --remote. Launch without it: main gives a directory for one task with rewake send --grant-dir, and a directory the whole session works in is its workspace, so launch from there"

// RefuseLaunch refuses a writable root given at launch. Beside --remote the
// terminal refuses --add-dir and a writable_roots override itself and exits 1
// before its first request (live on 0.157.1; 0.155.1 took both); refused here
// it is a call to change, with the way that works named.
func (codexHarness) RefuseLaunch(args []string) error {
	if len(harness.FlagValues(args, "--add-dir")) > 0 {
		return fmt.Errorf("--add-dir is refused: %s", grantAdvice)
	}
	for _, setting := range harness.FlagValues(args, configFlag, "--config") {
		if key := writableRootsKey(setting); key != "" {
			return fmt.Errorf("-c %s is refused: %s", key, grantAdvice)
		}
	}
	return nil
}

// writableRootsKey names the key of a -c setting that sets writable roots, or
// is "". The key is taken the way the CLI takes it, up to the first '=' and
// trimmed; a profile's key ends the same way, and an inline table of
// sandbox_workspace_write can carry the list within its value.
func writableRootsKey(setting string) string {
	name, value, found := strings.Cut(setting, "=")
	if !found {
		return ""
	}
	key := strings.TrimSpace(name)
	switch {
	case key == "writable_roots" || strings.HasSuffix(key, ".writable_roots"):
		return key
	case (key == "sandbox_workspace_write" || strings.HasSuffix(key, ".sandbox_workspace_write")) && strings.Contains(value, "writable_roots"):
		return key
	}
	return ""
}
