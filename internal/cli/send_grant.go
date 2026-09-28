package cli

import (
	"fmt"

	"github.com/praline-labs/rewake/internal/grantauth"
	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// registerGrant hands the grant a message carries to this session's wrapper
// before the message is written. The recipient's wrapper asks that wrapper to
// confirm it and refuses a grant it does not hold, so a letter nobody
// registered — one a worker wrote into its own mailbox, or sent with main's
// variables — carries none (docs/grants.md#who-can-grant).
func registerGrant(dir string, self registry.Session, epoch string, message inbox.Message) error {
	if !inbox.CarriesGrant(message) {
		return nil
	}
	adapter, _ := harness.Find(self.Harness)
	if issuer, ok := adapter.(harness.GrantIssuer); !ok || !issuer.ReachesWrapper() {
		return &FailedError{Message: fmt.Sprintf("a grant: %s runs %s, whose commands cannot reach its wrapper to register one — its sandbox refuses unix sockets — and a grant nobody registered is refused on delivery. Send the task without it, or ask the owner to grant the access from a main on a harness that can.", self.Name, self.Harness)}
	}
	err := grantauth.Register(state.AuthorityAddress(dir, epoch), grantauth.Grant{
		ID: message.ID, To: message.To, ToEpoch: message.ToEpoch,
		Dirs: message.GrantDirs, Broad: message.GrantBroad, Git: message.GrantGit,
	})
	if err != nil {
		return failf("a grant: this session's wrapper did not register it, and its recipient would refuse it unregistered: %v", err)
	}
	return nil
}

// carryGrant gives an edit's replacement the grant this session's wrapper
// holds for the message replaced, registered under the replacement's own id:
// the original's registration confirms the original only. What the letter on
// disk says is no source for it — a worker can rewrite its unread mail and
// would otherwise have main register a grant main never gave — only a check:
// a letter that names a grant the wrapper does not hold was rewritten, and
// the edit is refused rather than sent without the grant main may think it
// carries.
func carryGrant(dir string, self registry.Session, epoch string, old inbox.Message, replacement *inbox.Message) error {
	adapter, _ := harness.Find(self.Harness)
	if issuer, ok := adapter.(harness.GrantIssuer); !ok || !issuer.ReachesWrapper() {
		// Such a main registers nothing, so nothing here can be carried.
		if inbox.CarriesGrant(old) {
			return failf("your %s %s names a grant this session never registered — %s cannot reach its wrapper — so its letter was written by someone else; withdraw it with rewake withdraw %s and send the task again", inbox.KindOf(old), old.ID, self.Harness, shortRef(old.ID))
		}
		return nil
	}
	carried, held, err := grantauth.Carry(state.AuthorityAddress(dir, epoch), old.ID, grantauth.Grant{ID: replacement.ID, To: replacement.To, ToEpoch: replacement.ToEpoch})
	switch {
	case err != nil && inbox.CarriesGrant(old):
		return failf("a grant: this session's wrapper did not carry the grant of %s over to its replacement, and its recipient would refuse it unregistered: %v", old.ID, err)
	case err != nil:
		// Nothing to carry by the letter, and nothing learned from the
		// wrapper: an edit of a task with no grant goes on without one.
		return nil
	case !held && inbox.CarriesGrant(old):
		return failf("your %s %s names a grant this session's wrapper does not hold, so its letter was rewritten since it was sent; withdraw it with rewake withdraw %s and send the task again", inbox.KindOf(old), old.ID, shortRef(old.ID))
	case !held:
		return nil
	}
	replacement.GrantDirs, replacement.GrantBroad, replacement.GrantGit = carried.Dirs, carried.Broad, carried.Git
	return nil
}
