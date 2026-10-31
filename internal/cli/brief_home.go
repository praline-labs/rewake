package cli

import (
	"github.com/praline-labs/rewake/internal/core/taskbrief"
	"github.com/praline-labs/rewake/internal/host/launch"
)

// briefHomeOf is where the room's briefs are looked for (launch.BriefHome).
func briefHomeOf(dir string) (taskbrief.Home, error) { return launch.BriefHome(dir) }

// quietHomeOf is briefHomeOf for a command that reads or changes a brief that
// exists: a file that does not read leaves the room alone and refuses nothing.
func quietHomeOf(dir string) taskbrief.Home { return launch.QuietBriefHome(dir) }
