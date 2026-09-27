package wrap

import (
	"slices"

	"github.com/iiiokojiadbi/rewake/internal/grant"
	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
)

// checkGrant checks a message's granted directories again at delivery, with
// this session's own view of the machine: its home, its PATH, where each
// harness keeps its configuration. What the sender's view allowed, the
// receiver's may not, and the receiver's is the one the worker writes in.
func checkGrant(root string) func(inbox.Message) error {
	return func(message inbox.Message) error {
		rules := grant.CurrentEnv(root, harness.AllProtectedDirs()).Rules()
		for _, dir := range message.GrantDirs {
			if err := rules.Recheck(dir, slices.Contains(message.GrantBroad, dir)); err != nil {
				return err
			}
		}
		return nil
	}
}
