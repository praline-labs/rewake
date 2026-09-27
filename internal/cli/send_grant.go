package cli

import (
	"fmt"

	"github.com/iiiokojiadbi/rewake/internal/grantauth"
	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/state"
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
