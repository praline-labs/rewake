package cli

import (
	"io"
	"os"
	"slices"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/grant"
	"github.com/iiiokojiadbi/rewake/internal/harness/claude"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// handleGrantHook answers a Claude Code PreToolUse or PermissionRequest hook
// for the directories granted to this session (docs/grants.md): it gives a
// write inside a live grant, and takes back what a settled task was given.
//
// Like turn-ended it never fails loudly and never answers what it cannot
// prove: it runs in front of every tool call, and a hook that printed an
// error or an allow on a guess would be a tool call decided by accident.
// Silence leaves the call to the person and the permission mode.
func handleGrantHook(ctx *Context, _ Call) error {
	dir, err := state.Dir()
	if err != nil {
		return nil
	}
	name, epoch := os.Getenv(state.SessionEnv), os.Getenv(state.EpochEnv)
	// The common case costs one small read: a session with nothing granted
	// answers nothing.
	if !slices.ContainsFunc(grant.Load(dir, name, epoch), func(entry grant.Entry) bool { return entry.Live() || entry.Revoking() }) {
		return nil
	}
	if _, _, err := ownRun(dir); err != nil {
		// A hook of an earlier run answers for nobody.
		return nil
	}
	payload := readPayload(os.Stdin)
	var decided claude.GrantAnswer
	err = grant.Update(dir, name, epoch, func(entries []grant.Entry) ([]grant.Entry, bool) {
		decided = claude.DecideGrant(payload, entries)
		if len(decided.Removed) == 0 {
			return entries, false
		}
		now := time.Now()
		for index := range entries {
			if entries[index].Revoking() && slices.Contains(decided.Removed, entries[index].Path) {
				entries[index].Outcome, entries[index].EndedAt = grant.Revoked, &now
			}
		}
		return entries, true
	})
	if err != nil || len(decided.Output) == 0 {
		// Unrecorded, a removal would be asked again at every call: say
		// nothing rather than answer what the journal will not show.
		return nil
	}
	out := io.Writer(os.Stdout)
	if ctx != nil && ctx.Stdout != nil {
		out = ctx.Stdout
	}
	_, _ = out.Write(append(decided.Output, '\n'))
	return nil
}
