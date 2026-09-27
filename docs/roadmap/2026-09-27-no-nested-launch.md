# No session started from inside a session

September 27, 2026. Found by review-claude after stage 2 of the directory grant
([entry](2026-09-27-grant-dir-claude.md)), fixed the same day.

## What was found

- **A worker could start an agent without limits.** A Claude Code worker gets
  `--allowedTools "Bash(rewake:*)"`, so any plain `rewake` command runs unasked, and
  nothing refused a launch among them: `rewake claude --dangerously-skip-permissions -p x`
  from the worker's shell was an agent with none of the worker's limits. The hole
  predates the grant.
- **Low: taking a grant back trusted a rule the launch might not have added.** Rewake
  adds `Bash(rewake:*)` only when the launch carries no `--allowedTools` of the person's
  own, yet the grant hook answered a plain `rewake` command's forced question with the
  removal either way — approving a command the person's own rules may have left to them.
- **Low: `grants.md` said the removal went to the person with any other command.** It
  does not: that PermissionRequest hears silence, and the directory stays even if the
  person approves.

## What was done

- A launch — a harness word or an alias for one — from a shell where `REWAKE_SESSION` or
  `REWAKE_EPOCH` is set is refused with exit 2 before anything else is checked (main's
  decision of September 27, 2026); the environment alone decides, no live record needed
  ([launch.md](../launch.md#no-session-inside-a-session)). Two unit tests that stood for
  a launch from outside now clear the run their fake session set; the workflow suite
  already starts every case with these variables cleared.
- The grant hook's command carries `--rewake-allowed` exactly when the launch added the
  rule, the hook passes it with the call, and only then is a plain `rewake` command
  forcible ([grants.md](../grants.md#claude-code)). The payload cannot claim it.
- `grants.md` says what a command's question does while a grant is taken back: every
  shell command asks the person until the first read or plain `rewake` command, which
  for a worker under orchestration is the `rewake inbox` of its next turn.

## What stays open

- A way for main to start a worker, when there is one, is its own command.
