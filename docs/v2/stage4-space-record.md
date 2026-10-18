# Stage 4: the space record and `rewake space list`

What step S4-3 built, October 13, 2026: the record of a space in `internal/core/space` and
the view that lists it, `rewake space list`. The design is
[stage4-space.md](stage4-space.md) — the record in its section 1, what closing observes in
section 7, the restore it serves in section 8 — and the rules it holds are SP2 and SP7 in
[rules/space.md](../rules/space.md). The step that writes it is S4-7 — opening and closing
alike since October 18, 2026 — in [stage4-space-steps.md](stage4-space-steps.md#the-build-steps);
this step gave it the record and its steps.

**Changed October 18, 2026** by the space step (S4-7, [the entry](../roadmap/2026-10-18-space.md)):
the `keeper` field is gone and `host` replaces it; the checkout is no longer required to open,
and is recorded after the worker's launch made it. A record written before still reads, its
`keeper` read past; an `open` one without `host` does not.

## Where it lives

One JSON file per space, `spaces/<id>.json` in the room
([design.md](../design.md#state-directory)), written only through `internal/infra/state`,
so it refuses without the room's lease and in a view ([rules/state.md](../rules/state.md)).
The file is named by the space's id, 32 hex digits drawn at random, which also names the space
host's attach socket, `sock/space.<id>.sock`: a closed record stays for what a restore reads,
and its name is free again for a new space. The name is unique among the room's spaces that
are not over, checked when a record is created. Every step runs under the room's space lock,
`spaces/.lock`: it reads the record, checks the step against its state, and replaces the whole
file in one write, so a crash between two steps leaves the earlier one whole.

## What it holds

| Field | What |
|---|---|
| `id`, `name` | the space |
| `state` | below |
| `main` | the run that opened it: name, epoch (the wrapper's pid, start time and boot) and pid namespace |
| `worker` | harness id, `--name`, role, the session id rewake drew before the launch (absent for a harness that takes none), and its run once its wrapper is seen running |
| `checkout` | path and branch of the worktree the worker's launch made, once its session is seen there; absent for a worker started without one |
| `host` | the space host's run, as an epoch, recorded by the space host before it checks anything |
| `openedAt`, `closing` | when it opened; why it began to close |
| `end` | what closing observed: when, why, the worker's end, the checkout as found — there, gone or unknown, never left out — and when a checkout not seen gone was seen gone |

The session id is a random UUID, the form the worker harness's `--session-id` takes
([section 4](stage4-space.md#4-the-workers-launch)). A field a later build adds is read
past, since a record outlives the build that wrote it.

## The states and their steps

`opening` → `open` → `closing` → `kept` or `closed`; a step out of this order is refused
naming the state, and every step is written before the act it names is answered (SP2).

- **Create**: the record in `opening`, with the worker fixed — harness, name, role, session
  id — before anything of it starts.
- **SetHost**: in `opening` only, once — the space host's own run.
- **Opened**: `opening` to `open`, with the worker's run; refused without the space host's
  run, written once the worker's wrapper is started and before the open is answered.
- **SetCheckout**: in any state not over, once, when the worker's session is seen to claim a
  worktree — `closing` included, since the worker can claim it after the space host last
  looked.
- **BeginClosing**: `opening` or `open` to `closing`, with why; a second one keeps the
  first, since main's exit is seen both by its mod and by its harness ending.
- **Close**: any state not over to `kept`, when the checkout was found still there or could
  not be looked at, or to `closed`, when it was seen gone. It takes only an observation: the
  worker `ended` (its wrapper, which had a run, seen gone) or `never-started` (no run
  recorded, so its gate never opened and nothing of its harness ran), and the checkout's
  presence, always stated — `gone` (or never made), `there` with what was in it — changes,
  commits beyond its base, a Git lock such as a dead keeper leaves — or `unknown`. An end
  that says nothing of either, or contradicts the record, is refused: an observation left out
  never reads as a checkout seen gone. Any state may close, since the next start
  closes a record whatever step its dead main reached.
- **Cleared**: `kept` to `closed`, when the checkout is seen gone at that moment; a checkout
  still there, or one that cannot be looked at, keeps the record kept.

Every record, whatever step wrote it, holds its own invariants, and one read that breaks them
is unreadable rather than half believed: an end exactly when the state is final, `open` with
its space host's run and its worker's run, `kept` exactly while a checkout not seen gone at the close
is not seen gone since. A closed record whose end omits the checkout's presence does not read.

## The listing

`rewake space list`, a view, reads every record in the order the spaces opened, with a
record that does not read named apart and never taken for absent. For a space not over it
says whether its main still runs: told only from the main's own pid namespace, `unknown`
from any other. A kept checkout is reminded while it is there or cannot be looked at, with
what was found in it and the Git commands that clear it once what is wanted is merged —
`git worktree unlock` for a locked one, `git worktree remove`, `git branch -d` — each of which
refuses rather than lose work; rewake runs none of them (SP7). A kept checkout removed by
hand is no longer reminded, though its record reads `kept` until a change records it cleared:
a view writes nothing.

## Left to later steps

The writes from main's wrapper — the open's steps in S4-7, closing and the next start's
closing of a dead main's records in S4-9 — and the reminder of kept checkouts to main at its
start, S4-9. The worker's address, `<name>-<harness>`, is not stored: it follows from the
name and the harness.
