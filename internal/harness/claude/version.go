package claude

import "github.com/praline-labs/rewake/internal/harness"

// MinimumVersion is the oldest Claude Code the adapter starts
// (docs/v2/design-api.md#minimum-versions).
const MinimumVersion = "2.1.287"

var _ harness.LaunchVersionReader = claudeHarness{}

// ReadLaunchVersion reads the installed version before the claim by asking
// the program the launch will start, and refuses a launch below the minimum
// or without a version it could read.
func (h claudeHarness) ReadLaunchVersion(program string, env []string, dir string) (harness.Version, error) {
	version, err := harness.ReadVersion(program, env, dir)
	if err != nil {
		return version, err
	}
	return version, harness.RequireVersion(h.Title(), version, MinimumVersion)
}
