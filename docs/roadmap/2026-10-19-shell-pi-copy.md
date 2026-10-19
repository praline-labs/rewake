# The shell: a copy from pi reaches the person's terminal

October 19, 2026. **Done.** In the shell a copy from pi showed "Copied!" and left the person's
clipboard empty: pi 1.3.0 selects with the mouse itself and, on WSL without `WT_SESSION`, copies
with PowerShell and sends no OSC 52, so there was nothing for the shell's scanner to carry out. The
shell starts main with `PI_CLIPBOARD_OSC52=1` unless set, and main's workers inherit it; pi then
sends OSC 52 after its own write. Checked under a stand-in terminal with pi as main and PowerShell
stubbed to succeed: no OSC 52 before, the selection's OSC 52 after. **Open.** The owner's check on
their terminal, with a worker on pi; pi still runs its own clipboard write first.
