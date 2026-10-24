package cli

import (
	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
)

// spacesRefusal says why session cannot open a space, nil when it can, by the
// level that is missing (docs/rules/adapter-api.md#spaces): a role that opens
// none and a harness that offers none are a wrong call; a run that has not
// proven its spaces, or has lost them, is a refusal of the target, which a
// later run may not repeat. `rewake space open` asks it before anything is
// written (S4-7), with its command for the syntax a wrong call prints.
func spacesRefusal(command *Command, catalog adapter.Catalog, session registry.Session) error {
	usage := func(format string, args ...any) error {
		return &UsageError{Command: command, Message: sprintf(format, args...)}
	}
	if !role.Of(session.Role).Spaces {
		return usage("%s is a %s session, and only main opens a space.", session.Name, role.Of(session.Role).ID)
	}
	found, _ := catalog.Find(session.Harness)
	if !catalog.Offers(session.Harness).Spaces {
		return usage("%s is a %s session, which hosts no space in this rewake.", session.Name, harnessTitle(found, session.Harness))
	}
	if !session.HasLive(string(adapter.SpacesCapability)) {
		return failf("%s's spaces are not live: its run has not proven, or has lost, what hosts them, so a space would open nowhere; nothing was opened. rewake list --json shows what each run has live.", session.Name)
	}
	return nil
}
