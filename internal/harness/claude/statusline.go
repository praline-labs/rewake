package claude

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/harness/claude/telemetry"
)

// The tap finds the person's status line when it runs, from the harness's own
// environment (telemetry/statusline.go). What it cannot learn there is fixed
// at launch and handed to it: which settings files this launch reads, and the
// status line of a --settings the caller passed.

// launchSources are the file layers a launch with these arguments reads.
// Restricted mode ignores all three files, and --setting-sources names the
// ones read (claude --help, 2.1.280).
func launchSources(args []string) string {
	if harness.HasFlag(args, "--restricted") {
		return telemetry.NoSources
	}
	values := harness.FlagValues(args, "--setting-sources")
	if len(values) == 0 {
		return telemetry.AllSources
	}
	var named []string
	for _, name := range strings.Split(values[len(values)-1], ",") {
		if name = strings.TrimSpace(name); name != "" {
			named = append(named, name)
		}
	}
	if len(named) == 0 {
		return telemetry.NoSources
	}
	return strings.Join(named, ",")
}

// managedDir is where Claude Code reads a machine's policy on Linux.
var managedDir = "/etc/claude-code"

// policyStatusLine reports whether the machine's managed policy names a
// status line: the file and the drop-in directory beside it. Such a policy
// wins over the tap, so the tap never runs and its values stay unknown.
func policyStatusLine() bool {
	paths := []string{filepath.Join(managedDir, "managed-settings.json")}
	if entries, err := os.ReadDir(filepath.Join(managedDir, "managed-settings.d")); err == nil {
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".json") {
				paths = append(paths, filepath.Join(managedDir, "managed-settings.d", entry.Name()))
			}
		}
	}
	return slices.ContainsFunc(paths, func(path string) bool { return len(telemetry.FileStatusLine(path)) > 0 })
}
