package cli

import "github.com/praline-labs/rewake/internal/harness"

// internalGroup holds the commands a harness, a hook or the mail tool runs,
// never a person: hidden from the guide, each with its own help page.
func internalGroup() Group {
	return Group{
		Title: "INTERNAL",
		Commands: []*Command{
			{
				Name:           "turn-ended",
				Args:           "[payload]",
				MaxPositionals: 1,
				Summary:        "Called by a harness when a turn ends; tells the sessions that wrote here.",
				Examples:       []string{"rewake turn-ended"},
				Hidden:         true,
				Handler:        handleTurnEnded,
			},
			{
				Name:     harness.Observe,
				Args:     "<socket>",
				Summary:  "Called by a Claude Code hook or rewake's plugin; passes what it was handed to the session's wrapper.",
				Examples: []string{"rewake observe /tmp/rewake-1000/rooms/default/sock/worker-claude.1.obs"},
				Raw:      true,
				Hidden:   true,
				Handler:  handleObserve,
			},
			{
				Name:    harness.GrantHook,
				Summary: "Called by a Claude Code hook before a tool call and on a permission request; gives and takes back directories granted with a task.",
				Options: []Option{{
					Flag:    "--" + harness.GrantRewakeRule,
					Summary: "The launch added the allow rule for rewake, so a plain rewake command may take a grant back.",
				}},
				Examples: []string{"rewake grant-hook --" + harness.GrantRewakeRule},
				Hidden:   true,
				Handler:  handleGrantHook,
			},
			{
				Name:     harness.StatusTap,
				Args:     "<socket> [sources] [caller-status-line]",
				Summary:  "The status line of a Claude Code session; reports to its wrapper, then runs the configured status line.",
				Examples: []string{"rewake status-tap /tmp/rewake-1000/rooms/default/sock/worker-claude.1.obs user,project,local"},
				Raw:      true,
				Hidden:   true,
				Handler:  handleStatusTap,
			},
			{
				Name:           BridgeServe,
				MaxPositionals: 0,
				Summary:        "The mail tool's server: a stdio MCP server the harness starts, which runs one rewake command per tool call under a ticket from the session's wrapper.",
				Examples:       []string{"rewake bridge-serve"},
				Notes: []string{
					"Started by the harness with the launch values in its environment: REWAKE_DIR, REWAKE_ROOM, REWAKE_SESSION, REWAKE_EPOCH and REWAKE_BRIDGE_CAPABILITY. Everything it writes to stdout is a JSON-RPC message.",
					"A call that the wrapper did not see natively runs nothing; a refusal before a command starts says so, and names the shell when the tool itself failed.",
				},
				Hidden:  true,
				Handler: handleBridgeServe,
			},
			{
				Name:     BridgeHook,
				Args:     "<socket>",
				Summary:  "Called by a Claude Code hook before and after a call of the mail tool; tells the session's wrapper what the harness recorded of the call.",
				Examples: []string{"rewake bridge-hook /tmp/rewake-1000/rooms/default/sock/worker-claude.1.ctx"},
				Notes:    []string{"Prints nothing and exits 0 whatever happens: a wrapper that does not answer within its bound leaves the call without an observation, and the call then runs nothing."},
				Raw:      true,
				Hidden:   true,
				Handler:  handleBridgeHook,
			},
		},
	}
}
