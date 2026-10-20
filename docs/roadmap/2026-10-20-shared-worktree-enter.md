# Shared worktrees, step 2: a Claude Code worker enters a checkout of its own

October 20, 2026. **Built.** An alias's `enter_worktree = true` (refused on pi and beside
`worktree = true`) reaches the space host and the launch; Claude Code's run copy of the worker
settings drops `EnterWorktree` from deny and allows it, the run gets `REWAKE_WORKTREE_ENTER=1`;
`rewake worktree enter` makes and claims the worker's one checkout; the mod's `worktree_enter`
enters it by `EnterWorktree {path}`, and a fail-closed guard refuses the model's own call there.
The space host captures the claimed checkout as a launch's; no state or protocol change.
**Open.** The live check; review; leaving a checkout or holding two; parallel `worktree_enter` calls.
