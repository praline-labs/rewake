package wrap

import (
	"os"
	"testing"

	"github.com/praline-labs/rewake/internal/core/grant"
	"github.com/praline-labs/rewake/internal/core/grant/authority/authoritytest"
	"github.com/praline-labs/rewake/internal/core/mail"
)

// Every directory these tests grant lies in a temporary directory, which the
// hard tier refuses; that refusal has its own test in internal/core/grant.
//
// A test binary started again as a recipient's run confirms its grants at
// delivery, as its wrapper does, before anything is tested.
func TestMain(m *testing.M) {
	grant.TempRoots = func() []string { return nil }
	authoritytest.Child(func(run, id, thread string, delivery authoritytest.Delivery) error {
		letter := mail.Message{ID: id, From: delivery.From, FromEpoch: delivery.FromEpoch, GrantDirs: delivery.Grants[id]}
		return confirmGrant(delivery.Dir, delivery.To, run, thread, letter)
	})
	os.Exit(m.Run())
}
