# The host mode removed

October 19, 2026, step 4 of [the shell](../v2/shell.md), after the owner checked step 1 on their
own terminal. Deleted, not archived: `internal/host/hostmode`, the canvas, the space socket's view
mode `v` with `spacehost.View` and `Panel`, the screen frames and events of `internal/host/terminal`,
`--host` and its refusals, the host mode's workflow scenario and `docs/v2/host-mode.md`; its facts
the shell relies on moved into `shell.md`. `LaunchRequest.Host` is now `Shell`. Bell and
notifications are not delivered, by the owner's decision. About 4,200 lines gone. Live on 2.1.302:
the shell started, a worker opened by main showed, Shift+Enter in both, `/exit` left nothing.
Open: the owner's check that closing the window leaves nothing, steps 2–3.
