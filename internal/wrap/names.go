package wrap

import (
	"fmt"

	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// Role resolution precedes naming under the room lock, so an automatic name
// uses the resolved role while a custom prefix cannot select a role.
func launchName(prefix, selectedRole, harnessID string) (string, error) {
	if prefix == "" {
		prefix = selectedRole
	}
	if !state.ValidName(prefix) {
		return "", fmt.Errorf("%w: prefix %q; use 1 to 32 lower-case letters, digits, dots, dashes or underscores, starting with a letter or digit", registry.ErrUnusableName, prefix)
	}
	name := prefix + "-" + harnessID
	if !state.ValidName(name) {
		if len(harnessID) > 30 {
			return "", fmt.Errorf("%w: harness ID %q leaves no room for a nonempty prefix within 32 characters; choose a harness with a shorter ID", registry.ErrUnusableName, harnessID)
		}
		return "", fmt.Errorf("%w: assembled name %q must fit 32 characters and the session-name syntax; use a valid --name prefix of at most %d characters before -%s", registry.ErrUnusableName, name, max(0, 31-len(harnessID)), harnessID)
	}
	return name, nil
}
