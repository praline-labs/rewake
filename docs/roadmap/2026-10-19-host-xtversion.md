# The host answers XTVERSION

A Claude Code worker's pane still flickered after the DECRQM 2026 answer. Captured live (Claude Code
2.1.302 on haiku, rewake's shell in a pty, main and a worker, the space socket's `b` stream): main's
program sent `CSI > 0 q` and then `CSI ? 2026 $ p`, and wrapped 354 frames in `?2026h/l`; the
worker's sent `CSI > 0 q`, got no answer, never asked DECRQM and sent no `?2026h/l`, only `?25l`
… `?25h` around each frame (368 pairs in 30 s). `newVT` now answers XTVERSION as `libghostty`, as
the widget does for main. Same task, worker pane focused: of the shell's frames, 315 of 637 ended
with the cursor hidden, 25 distinct cursor states; after, 334 frames, one state, none hidden.
Open: the owner's look; the DECRQM answer was not tried without XTVERSION's.
