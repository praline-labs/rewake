# Stage 3: the run directory

What one run writes and how long it lives: the directory step 4 of S17 adds, part of
[stage3-state.md](stage3-state.md), whose sequence and decisions it follows.

## Where it is and when it goes

L3: what an adapter writes for one run goes with the run (D11). `run/<name>.<epoch>/` in
the room (`state.RunPath`) is created by the wrapper after the claim and before the
adapter prepares the launch, and handed to the adapter as `LaunchRequest.RunDir`, an input
of its own rather than a path it derives; a launch that cannot make it does not start.
It is removed — that epoch's directory only, everything in it — after the services,
children, sockets and mappings stop, on every path out, a failed launch included: the
removal is deferred right after the claim, so every stop registered later runs first. A plan the adapter returned is the run's from that moment:
its service is closed once on every later path out — a failed update of the record, a read
clock that cannot open, a service that fails to start, a missing program — before the
directory goes (`TestFailedLaunchRemovesOnlyItsRunDirectory`).
Durable mail, grants, receipts and recovery journals stay outside it. Socket paths keep
today's short form (`state.SocketPath`, `ContextPath`: a hash when the name would pass the
Unix limit) in `sock/`, so a long room or name cannot overflow `sun_path`; a socket inside
the run directory could.

## The qualified guarantee

A run that ends — by its harness exiting, a signal it
handles, or a failed launch — removes its directory before the wrapper exits. A wrapper killed by `SIGKILL`
runs no deferred cleanup; its directory stays until a later launch in
the room reclaims it, and only after that run is proven dead. The proof is the run's
epoch, read from the end of the directory's name: the wrapper it names is gone, another
process holds its pid, it is a zombie, or it ran in another boot — the judgment that
prunes a dead record (`registry.ObserveRun`). A run still running, one that cannot be
told, and an entry that names no run are kept. The launch reclaims the run's control
directory (`control/<name>.<epoch>/`) too; sockets keep their own upkeep, since a harness
may be the one listening. Like pruning, the proof assumes the wrappers' pid namespace. A
room with no later launch keeps a killed run's directories.
