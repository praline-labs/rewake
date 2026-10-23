# A worker's dialogs reach main; workers read main's checkout

October 23, 2026, on the owner's report: a worker in its own checkout stopped on auto mode's one-time
"Read outside the working directories" prompt, main told nothing; workers run in auto mode, where the
mod's permission bridge carried nothing. Done: each run's `--settings` allows `Read(//<main's root>/**)`
(probed live: no prompt); a worker's engine-drawn permission dialog goes to main at
`classic.PermissionRequest` as a permission request (once, session, refuse); any other dialog it waits
at, known by `classic.Notification`, reads `waiting at a dialog` in `rewake list` and the strip and is
told to main once. **Open:** review-pi's acceptance of the additive `StateRequest.dialog` and snapshot
field; the live check through rewake.
