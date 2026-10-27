---
description: The owner's rule of building a protective mechanism only for a failure seen in real work, and when to stop and simplify; open before adding a guard, a retry, a journal or a test of a failure.
---

# Build for failures that were seen

The owner's rule, taken when the mechanics of unknown outcomes — thousands of lines of code
and tests, no live case — gave way to a simple order of steps:

- A protective mechanism is built for a failure seen in real work, or one that would lose or
  duplicate mail unseen. "What if the process is killed between two writes" is a known limit
  written in one line, not a reason for a mechanism.
- The cost of a mistake sets the depth: when main asking again or restarting a worker is cheaper
  than a mechanism, no mechanism is built.
- The simple solution first: a lock, an atomic write, a deterministic id, a retry of the same
  steps. Unknown states, journals before effects, operator decisions, matrices of worlds or
  crashes come only by the owner's decision after a live case.
- A review looks for breakage, not for imaginable orders of events: such a finding without a
  live case is a note or a known limit, never a new round.
- Stop and simplify when the tests of a mechanism outgrow its code, when each review round finds
  "another way to lose", when a feature needs numbered rules, a world model or a scenario
  generator, or when a mechanism can stop work that would otherwise have gone through.

The turn end itself: under the sender's mailbox lock it writes its reports, then clears the waits
they answer, and records itself done last; a retry runs the same steps from the disk
([delivery-turn-end.md](delivery-turn-end.md#the-end-of-a-turn)). The build lease and
epochs stay as they are.
