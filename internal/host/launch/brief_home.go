package launch

import (
	"fmt"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/core/taskbrief"
)

// BriefHome is where the room's briefs are looked for: the room, and the
// folder the project's [briefs] dir names, read from the directory main runs
// in (docs/briefs.md#where-a-brief-lives). Any run asks the same, so a worker
// outside the project's directory finds what main put there. A project file
// that does not read, or no main to ask, leave the room alone; brief new
// refuses the first, since a brief made then would land in the wrong place.
func BriefHome(dir string) (taskbrief.Home, error) {
	home := taskbrief.Home{Room: dir}
	sessions, err := registry.ListReadOnly(dir)
	if err != nil {
		return home, nil
	}
	for _, session := range sessions {
		if session.Role != role.Main.ID || session.CWD == "" {
			continue
		}
		set := LoadIn(session.CWD)
		if err := set.Err(); err != nil {
			return home, fmt.Errorf("the project's .rewake/aliases.toml in %s does not read, so where its briefs live is not known: %w", session.CWD, err)
		}
		home.Folder = set.BriefsDir(session.CWD)
		break
	}
	return home, nil
}

// QuietBriefHome is BriefHome for a reader of briefs that exist: a file that
// does not read leaves the room alone and refuses nothing.
func QuietBriefHome(dir string) taskbrief.Home {
	home, err := BriefHome(dir)
	if err != nil {
		return taskbrief.Home{Room: dir}
	}
	return home
}
