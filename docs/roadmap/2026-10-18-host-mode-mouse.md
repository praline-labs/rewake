# The host mode: mouse, paste and focus

October 18, 2026, step 2 of [the host mode](../v2/host-mode.md): the person's terminal keeps SGR
mouse and bracketed paste on while the host runs; a click on the bar switches tabs; a mouse event
in the region reaches the shown program re-encoded for its tracking mode and 1006, dropped when
it turned the mouse off; a paste is framed only for a program with 2004; focus reports stay
mirrored. Claude Code starts fullscreen in host mode unless the person set the variable, and its
workers inherit it. Checked live on 2.1.302 with the cheapest model: the wheel scrolled a
worker's history, a drag selected and copied in it, a paste stayed framed.
Open: steps 3–4 (the sidebar, events and colors), the review, acceptance.
