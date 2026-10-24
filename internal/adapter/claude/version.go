package claude

import "github.com/praline-labs/rewake/internal/adapter"

// MinimumVersion is the oldest Claude Code the adapter starts
// (docs/rules/adapter-api.md#minimum-versions).
const MinimumVersion = "2.1.287"

// Version: Claude Code answers --version with its version and its name,
// "2.1.287 (Claude Code)".
func (claudeHarness) Version() adapter.VersionGate {
	return adapter.VersionGate{Minimum: MinimumVersion, Args: []string{"--version"}}
}

// EnvNotInherited are the markers a Claude Code session exports to its
// children.
func (claudeHarness) EnvNotInherited() []string { return childMarkers }
