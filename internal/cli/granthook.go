package cli

import (
	"io"
	"os"

	"github.com/iiiokojiadbi/rewake/internal/grantauth"
	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// handleGrantHook answers a Claude Code PreToolUse or PermissionRequest hook
// for the directories granted to this session (docs/grants.md#claude-code).
// It decides nothing itself: the grants live in the memory of the session's
// wrapper, which this hook runs below, and the wrapper answers. Nothing on
// disk is read, since a file in the state directory is one a worker could
// write.
//
// Like turn-ended it never fails loudly and never answers what it cannot
// prove: it runs in front of every tool call, and a hook that printed an
// error or an allow on a guess would be a tool call decided by accident.
// Silence leaves the call to the person and the permission mode. A payload
// past maxPayload — a Write of a file that large — does not parse, and is
// such a silence.
func handleGrantHook(ctx *Context, _ Call) error {
	dir, err := state.Dir()
	if err != nil {
		return nil
	}
	self, epoch, err := ownRun(dir)
	if err != nil {
		// A hook of an earlier run answers for nobody.
		return nil
	}
	adapter, ok := harness.Find(self.Harness)
	if !ok {
		return nil
	}
	granter, ok := adapter.(harness.HookGranter)
	if !ok {
		return nil
	}
	call, ok := granter.GrantCall(readPayload(os.Stdin))
	if !ok {
		return nil
	}
	pid, start, ok := registry.ParseEpoch(epoch)
	if !ok {
		return nil
	}
	output, err := grantauth.Ask(state.KeeperAddress(dir, self.Name, epoch), grantauth.Expect{PID: pid, Start: start}, call)
	if err != nil || len(output) == 0 {
		return nil
	}
	out := io.Writer(os.Stdout)
	if ctx != nil && ctx.Stdout != nil {
		out = ctx.Stdout
	}
	_, _ = out.Write(append(output, '\n'))
	return nil
}
