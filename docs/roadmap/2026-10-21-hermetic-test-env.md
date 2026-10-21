# Tests hermetic to a session's other variables

October 21, 2026. **Done.** The full `-race` run from main's session failed in pi's tool test:
`bun test` inherited `REWAKE_ROLE=main`, and the tool offered main's tools too. The tests clear
only the four variables naming a session; a session also carries `REWAKE_ROLE`,
`REWAKE_EXECUTABLE`, `REWAKE_WORKTREES`, `REWAKE_MOD_SOCKET` and, for a worker that may enter a
checkout, `REWAKE_WORKTREE_ENTER`. The `bun test` and `claude plugin` children now start without
any `REWAKE_*`, and the worktree enter test sets its variable empty; the whole run passes with
`REWAKE_ROLE=main` and `REWAKE_WORKTREE_ENTER=1` added. AGENTS.md's Checks say so. **Open.** None.
