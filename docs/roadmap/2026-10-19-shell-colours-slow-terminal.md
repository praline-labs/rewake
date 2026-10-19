# The shell's colours from a slow terminal

`renderer.getPalette` gives up after a fixed 300 ms before the first reply (its `timeout` does
not reach that probe) and returns nothing without failing, so a terminal slow right after start
left main's pane black and `REWAKE_HOST_COLORS` unset. `askColors` now writes OSC 4/10/11 once
and waits up to 2.5 s for the replies; with no background reply the panes are
`transparentBackground`. Checked under a pty driver: replies at 0, 1 and 2 s give the colours,
no reply sets none and paints no background colour.
Open: a silent terminal now delays the start by 2.5 s.
