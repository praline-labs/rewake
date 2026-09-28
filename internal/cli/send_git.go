package cli

import (
	"fmt"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/role"
)

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
		return refuse(fmt.Sprintf("recipient %s is not eligible for repository Git metadata grants", target.Name))
	}
	adapter, _ := harness.Find(target.Harness)
	capability, supported := adapter.(harness.GitGrantHarness)
	if !supported || !capability.SupportsGitGrant() {
		return refuse(fmt.Sprintf("recipient harness %s does not support explicit per-message Git grants", target.Harness))
	}
	return true, nil
}
