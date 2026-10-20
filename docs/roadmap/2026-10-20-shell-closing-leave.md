# The closing screen from main's leave of its screen

October 20, 2026. **Done.** The owner still saw main's pane empty before the spinner. Measured live
(2.1.302, one worker, the shell from source alone and inside herdr): the mark was never late on
this machine, and one closing the owner watched ran a build without it; but `/exit` with work
running opens Claude Code's "Background work is running" dialog after `command.run`, so the layer
hid it 5 s with the keys going nowhere, and after it the mark waits for the tasks to stop. The
shell now starts the layer at main's leave of the alternate screen and lifts it when main comes
back or still runs 5 s after the last sign; the mod tells `session/closing` at `session.end` alone.
The spinner at +22 ms after `/exit`, +29–36 after Ctrl+C. **Open.** Ctrl+Z inside the shell leaves
Claude Code suspended-looking; the layer shows 5 s over it.
