# A worker's dialogs reach main; workers read main's checkout

October 23, 2026, on the owner's report: a worker in its own checkout stopped on auto mode's "Read outside the working directories" prompt, main told nothing. **Done.**
Each run's `--settings` allows `Read(//<main's root>/**)` (probed live: no prompt). A worker's engine-drawn permission dialog goes to main at `classic.PermissionRequest` as a request (once, session, refuse); any other dialog it waits at, known by `classic.Notification`, reads `waiting at a dialog` in `rewake list` and the strip and is told to main once.
Seen live (Claude Code 2.1.302): write-claude in auto mode made a Bash call whose `rm -rf` the engine asked about; main got the notify "write-claude asks permission for Bash (request b009)" with input, choices and a two-minute deadline, and `rewake answer write-claude b009 once` was taken.
review-pi accepted the additive `StateRequest.dialog` and snapshot field (99120c5, c4ba084), no blockers. Limit: a new mod with an old host is refused (`DisallowUnknownFields`), which a room never meets, one build per room.
