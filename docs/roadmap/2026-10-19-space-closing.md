# A space closes when only zombies are left of its worker

October 19, 2026. `TestFixtureSpace` failed about one run in ten with the space left
`closing` after main ended: an orphan the hangup left was adopted by the suite's subreaper,
which reaps in batches, and as a zombie it kept `kill(-pgid, 0)` answering through SIGKILL.
`groupGone` now uses `proc.GroupAlive`, which skips zombies; a unit test makes one under a
subreaper. A process whose leader thread is a zombie counts once its other threads ended: review
saw a worker whose first thread ended by `pthread_exit` taken for gone after the hangup, and a
test now makes one. The rate was 3 failures in 35 runs before, 0 in 30 after and 0 in 20 with
the thread check. Open: nothing.
