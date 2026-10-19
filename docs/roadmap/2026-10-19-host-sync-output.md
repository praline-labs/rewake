# The host answers DECRQM 2026 as known

The owner saw a worker's pane flicker fast while the worker worked; main's pane did not. The
host's emulator library answers a mode it has not seen as "not recognized" (0), the widget of
main's pane says "reset" (2), so a program that asks before drawing syncs in main and not in a
worker. `newVT` now sets 2026 to reset at creation, so the host answers 2 and 1 inside a frame;
the bytes already reach the widget unchanged. Unit-tested (the answer); the cause is the lead of
the review, not captured from a Claude Code worker.
The live run found the real cause: [the host answers XTVERSION](2026-10-19-host-xtversion.md). Open: a full reset (ESC c) forgets the seed.
