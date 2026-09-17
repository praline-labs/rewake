package brief

import (
	"fmt"

	"github.com/iiiokojiadbi/rewake/internal/role"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// Greeting starts a fresh conversation by teaching the command contract.
func Greeting(c Context) string {
	if c.Room == "" {
		c.Room = state.DefaultRoom
	}
	return fmt.Sprintf("You are session %q in room %q with role %s. Run rewake guide, read it, and reply with the single word ready.", c.Name, c.Room, role.Of(c.Role.ID).ID)
}
