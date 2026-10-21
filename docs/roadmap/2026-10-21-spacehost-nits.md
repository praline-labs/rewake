# The space host's small faults

October 21, 2026. **Done.** (1) `rewake space close` of a space whose host process was killed left
it closing for good: when the host's run reads ended (`space.HostRuns`), close itself stops what
runs of the worker's group, releases the checkout by SP7 and records the end, the reason adding
"; its space host was gone" (`spacehost.FinishGone`); live on the fixture harness, `kill -9` of
the host then close, 100 ms. (2) Main's end: the hosts began the workers' close after main's run
read ended; a main in the shell now moves its open spaces to closing at `session/closing`
(`space.BeginClosingFor`, in `internal/host/wrap.go`). Live, `/exit` with two haiku workers, last
worker closed: 2.6, 2.6, 3.0, 3.4 s after the key before, 1.5, 1.8, 1.9 s after. **Open.** A harness
process that outlived a dead host's worker wrapper; the dead host's socket file stays.
