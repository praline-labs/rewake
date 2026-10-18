# The workers' pane: fixes found live

October 18, 2026, from the owner's live look at [the panel](2026-10-18-panel.md): only `/workers`
opens the pane, with `closeOnEscape`, and answers how to use it; the key surface spans the
worker's screen, so a click anywhere there gives it the keys; the space host takes a view's frame
30 ms after a change, so a working Claude Code worker's cursor no longer blinks. Checked through a
pseudo-terminal with a haiku worker ([research](../research-claude-mod.md#the-panes-focus-closing-and-frames-later-the-same-day-21302)).
Open: typing right after `/workers` without a click (no `Client` focus in 2.1.302), Ctrl+X X while
the screen holds the keys, opening the pane by itself again once the owner accepts it; not yet
accepted.
