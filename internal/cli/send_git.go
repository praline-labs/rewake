package cli

import (
	"fmt"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/role"
)

// requestedGitGrant checks --grant-git. A call to change — the wrong sender,
// kind or role — is refused with exit 2, as a question to a silent role is:
// the caller could have seen it in rewake list. A recipient whose harness
// cannot take the grant is refused with exit 1, as --grant-dir refuses one: the
// call was right, the target cannot carry it out.
func requestedGitGrant(call Call, sender, target registry.Session, senderErr error) (bool, error) {
	value, requested := call.Flags["grant-git"]
	if !requested {
		return false, nil
	}
	refuse := func(reason string) (bool, error) {
		return false, &UsageError{Command: call.Command, Message: "--grant-git: " + reason}
	}
	if value != "true" {
		return refuse("this switch takes no value")
	}
	if senderErr != nil || sender.Role != role.Main.ID {
		return refuse("only a verified current main session may request Git metadata access")
	}
	kind, err := chosenKind(call)
	if err != nil {
		return false, err
	}
	if kind.kind != inbox.Task && kind.kind != inbox.Question {
		return refuse("only tasks and questions can carry an explicit grant; notify/report paths cannot")
	}
	if !role.Of(target.Role).GitWrite {
		return refuse(fmt.Sprintf("%s is a %s session, and only a write session takes repository Git metadata grants. Send the task to a --write session whose harness takes a Git grant, or do the Git part yourself.", target.Name, role.Of(target.Role).ID))
	}
	adapter, _ := findHarness(target.Harness)
	capability, supported := adapter.(harness.GitGrantHarness)
	if !supported || !capability.SupportsGitGrant() {
		return false, &FailedError{Message: fmt.Sprintf("--grant-git: %s runs %s, which takes no per-message Git grant. Do the Git part yourself, or send the task to a --write session whose harness takes one.", target.Name, target.Harness)}
	}
	return true, nil
}
