# Milestone 6. Ready for daily use — open

- Cleaning up `done/` by age; clear failures at the edges (a session dying
  while a wait is in progress, a taken name, an unreachable directory).
- `README.md`: what it is, installation, the three commands, the trust
  boundary.
- Switching the inbox from polling to inotify, with polling as a fallback.
- Building for linux-amd64 and linux-arm64, publishing `@iiiokojiadbi/rewake`
  with platform packages; publish only on the owner's explicit word.

Done so far: the mailbox is watched rather than polled and finished messages are
swept by age, both on September 16, 2026
([progress record](#milestone-6-progress-september-16-2026));
`README.md` exists; `scripts/pack.sh` builds the platform packages without
publishing ([local installation](../install.md)). Not done: the publication itself.

Acceptance: a week of use without manual intervention; not one case of a
message silently getting lost.

## Milestone 6 progress, September 16, 2026

- **The mailbox is watched, not polled.** A message arrives as a rename into the
  directory and the kernel says so, so delivery no longer waits for a tick. The
  poll stays at one second as the safety net: it retries pending messages and
  covers a watch the kernel would not give.
- **Finished messages are swept by age.** Delivered and refused ones, and their
  statuses, are kept a day and then let go; a message still waiting is answered
  by the TTL rather than by the sweep.

Two things the live run caught that the tests did not:

- the watch descriptor was closed by two goroutines, and once the number was
  reused that closed somebody else's file — the test framework's own directory,
  as it happened. It is now waited on rather than blindly read, and closed once;
- after the server's tick was slowed to a second, every delivery took exactly
  that long. The server was immediate; the *sender* was polling for its answer at
  the same slow interval. Measured on a live session: 0.03 s instead of 1.0 s.
