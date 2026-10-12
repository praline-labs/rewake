//go:build rewakefault

package legacy

import (
	"os"

	"github.com/praline-labs/rewake/internal/infra/proc"
)

// A build with the fault seam judges wrappers by the /proc-shaped tree that
// REWAKE_FAULT_PROC names, when it is set: the upgrade rig makes a wrapper's
// process data unreadable or malformed there, a case the machine's /proc
// cannot be made to show without privilege. A release is never built with the
// tag, and has no such variable.
func init() {
	if root := os.Getenv("REWAKE_FAULT_PROC"); root != "" {
		procReader = proc.Reader{Root: root}
	}
}
