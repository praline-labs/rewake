# Ctrl+Z in main inside the shell, and main's pane after an editor

October 21, 2026. **Done.** Ctrl+Z in main left Claude Code 2.1.302 on "has been suspended" until
Ctrl+C: it waits for `SIGCONT` and sends `SIGTSTP` to its group, which on the shell's pane is
orphaned, so the kernel dropped the stop and nothing continued it. Main's wrapper in the shell now
catches that `SIGTSTP` and sends the harness `SIGCONT`; live, Claude Code's screen is back 19–21 ms
after the key and takes keys. The 250 ms late pane after an editor or Ctrl+Z was the shell's hold
of main's bytes, gone since October 20: vim's return draws at +19–22 ms, against +256–259 ms with
it. **Open.** One frame of the normal screen shows at Ctrl+Z; pi's suspend, the same call, not
tried live.
