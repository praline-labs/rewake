# The closing screen seen on a real close

October 19, 2026. **Done.** The owner saw no spinner on `/exit` or Ctrl+C twice. Measured live with
Claude Code 2.1.302 and one worker: Claude Code leaves the alternate screen at once, main's process
exits 340–420 ms later, and the shell exited 30 ms after it, before the layer drew a frame, while
the worker ended about 0.9 s later unseen. The shell now keeps the layer until each worker it
showed reads closed (at most 10 s) and at least 600 ms: live, the spinner showed from main's exit
until the shell's, about a second, for both ways of ending.
**Open.** The 340–420 ms between Claude Code leaving its screen and its exit show its last lines:
no honest earlier sign was found (Ctrl+Z leaves the alternate screen too). Ctrl+Z in main leaves
Claude Code on "has been suspended" though its process was continued; not looked into.
