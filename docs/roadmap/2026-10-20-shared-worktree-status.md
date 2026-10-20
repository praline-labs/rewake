# The shared worktree system, slice 1: `worktree = true` and `worktree_status`

October 20, 2026, the first slice of the design the owner accepted that day ([worktree-tools.md](../worktree-tools.md)).
**Done.** An alias may say `worktree = true`; `space open` resolves the worker's checkout once for
its launch and the space host, `--worktree[=<name>]` and `--no-worktree` over it. `rewake worktree
status`, a view, names the run's own checkout and forecasts its close by Release's policy less the
worker's own liveness (`worktree.CloseForecast`); the `worktree_status` tool runs it by argv through
`REWAKE_EXECUTABLE` in the mod and the pi extension. Live: the mod on the cheapest model, no prompt,
remove → keep after an unlanded commit → remove once landed, as the close did; pi 1.3.0 in a private
agent dir on a local stub model. **Open.** Review; main's `worktree_list` and `worktree_land`; entering a checkout in a session.
