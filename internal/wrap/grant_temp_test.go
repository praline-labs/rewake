package wrap

import (
	"os"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/grant"
	"github.com/iiiokojiadbi/rewake/internal/grantauth/grantauthtest"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
)

// Every directory these tests grant lies in a temporary directory, which the
// hard tier refuses; that refusal has its own test in internal/grant.
//
// A test binary started again as a recipient's run confirms its grants at
// delivery, as its wrapper does, before anything is tested.
func TestMain(m *testing.M) {
	grant.TempRoots = func() []string { return nil }
	grantauthtest.Child(func(run, id, thread string, delivery grantauthtest.Delivery) error {
		letter := inbox.Message{ID: id, From: delivery.From, FromEpoch: delivery.FromEpoch, GrantDirs: delivery.Grants[id]}
		return confirmGrant(delivery.Dir, delivery.To, run, thread, letter)
	})
	os.Exit(m.Run())
}
