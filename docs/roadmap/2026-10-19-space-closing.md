# A space closes when only zombies are left of its worker

October 19, 2026. `TestFixtureSpace` failed about one run in ten with the space left
`closing` after main ended. Once main's run ended, the space host stopped the worker's
group, and a process the hangup had orphaned was adopted by the suite's subreaper, which
reaps in batches. That zombie kept `kill(-pgid, 0)` answering, so the group never read gone,
SIGKILL changed nothing, and the host left the record closing. `groupGone` now uses
`proc.GroupAlive`, which skips zombies, and a unit test makes such a zombie under a
subreaper. The rate was 3 failures in 35 runs before and 0 in 30 after. Open: nothing.
