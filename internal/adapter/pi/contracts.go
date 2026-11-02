package pi

import "strconv"

// Contracts is the extension's wire, which a loaded extension keeps speaking
// across a switch of the room's build (adapter.Contracts).
func (harness) Contracts() map[string]string {
	return map[string]string{"extension": strconv.Itoa(protocolVersion)}
}
