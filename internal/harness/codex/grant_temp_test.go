package codex

import (
	"os"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/grant"
	"github.com/iiiokojiadbi/rewake/internal/grantauth"
	"github.com/iiiokojiadbi/rewake/internal/grantauth/grantauthtest"
)

// Every directory these tests grant lies in a temporary directory, which the
// hard tier refuses; that refusal has its own test in internal/grant.
//
// A test binary started again as a recipient's run confirms its grants at
// delivery, as its wrapper does, before anything is tested.
func TestMain(m *testing.M) {
	grant.TempRoots = func() []string { return nil }
	grantauthtest.Child(func(run, id, thread string, delivery grantauthtest.Delivery) error {
		_, err := grantauth.Confirm(delivery.Address, grantauth.Expect{PID: delivery.MainPID, Start: delivery.MainStart}, id, delivery.To, run, thread)
		return err
	})
	os.Exit(m.Run())
}
