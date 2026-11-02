package claude

import (
	"strconv"

	"github.com/praline-labs/rewake/internal/adapter/claude/modhost"
)

// Contracts is the mod↔host protocol, which a loaded mod keeps speaking
// across a switch of the room's build, and the run's handed state, which the
// next image adopts (adapter.Contracts).
func (claudeHarness) Contracts() map[string]string {
	return map[string]string{"mod": strconv.Itoa(modhost.Protocol), "handover." + ID: strconv.Itoa(handoverVersion)}
}
