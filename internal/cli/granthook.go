package cli

import (
	"io"
	"os"

	"github.com/iiiokojiadbi/rewake/internal/grantauth"
	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/harness/claude"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// handleGrantHook answers a Claude Code PreToolUse or PermissionRequest hook
// for the directories granted to this session (docs/grants.md#claude-code).
// It decides nothing itself: the grants live in the memory of the session's
// wrapper, which this hook runs below, and the wrapper answers. No record in
// the state directory is read — the session and its run come from the
// environment the wrapper gave the harness — since a record is one a worker
// could write, and reading the registry would tidy it on every tool call.
//
// Like turn-ended it never fails loudly and never answers what it cannot
// prove: it runs in front of every tool call, and a hook that printed an
// error or an allow on a guess would be a tool call decided by accident.
// Silence leaves the call to the person and the permission mode. A payload
// past maxPayload — a Write of a file that large — does not parse, and is
// such a silence.
func handleGrantHook(ctx *Context, invocation Call) error {
	name, epoch := os.Getenv(state.SessionEnv), os.Getenv(state.EpochEnv)
	pid, start, ok := registry.ParseEpoch(epoch)
	if name == "" || !ok {
		return nil
	}
	dir, err := state.Dir()
	if err != nil {
		return nil
	}
	call, ok := claude.GrantCall(readPayload(os.Stdin), invocation.Switch(harness.GrantRewakeRule))
	if !ok {
		return nil
	}
	output, err := grantauth.Ask(state.KeeperAddress(dir, name, epoch), grantauth.Expect{PID: pid, Start: start}, call)
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
