# A worker's dialogs reach main; workers read main's checkout

October 23, 2026, on the owner's report: a worker in its own checkout stopped on auto mode's one-time
"Read outside the working directories" prompt in its pane, and main was told nothing. **In progress.**
Workers run in auto mode, Claude Code's default, where the mod's permission bridge carries nothing.
Done: each run's `--settings` allows `Read(//<main's repository root>/**)`, so reading main's
checkout passes with no prompt (probed live); the engine's own permission dialog of a worker, in auto
mode or any mode the check does not carry, goes to main at `classic.PermissionRequest` as a permission
request (once, session, refuse), the dialog held on screen. **Open:** a notice to main for a dialog
nobody can answer from outside; the live check through rewake.
