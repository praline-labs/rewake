# Stage 4: the space record and `rewake space list`

What step S4-3 built, October 13, 2026: the record of a space in `internal/core/space` and
the view that lists it, `rewake space list`. The design is
[stage4-space.md](stage4-space.md) — the record in its section 1, what closing observes in
section 7, the restore it serves in section 8 — and the rules it holds are SP2 and SP7 in
[rules/space.md](../rules/space.md). The steps that write it are S4-7 (opening) and S4-9
(closing) in [stage4-space-steps.md](stage4-space-steps.md#the-build-steps); this step gives
them the record and its steps, and nothing yet calls them but the tests.

## Where it lives

One JSON file per space, `spaces/<id>.json` in the room
([design.md](../design.md#state-directory)), written only through `internal/infra/state`,
so it refuses without the room's lease and in a view ([rules/state.md](../rules/state.md)).
The file is named by the space's id, 32 hex digits drawn at random, which is also the id the
mod's protocol carries (`SpaceOpen.space`): a closed record stays for what a restore reads,
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
| `checkout` | path and branch where the worker runs |
| `keeper` | the agent id of the keeper subagent, once its spawn answers |
| `openedAt`, `closing` | when it opened; why it began to close |
| `end` | what closing observed: when, why, the worker's end, the checkout as found, and when a checkout left was seen gone |

The session id is a random UUID, the form the worker harness's `--session-id` takes
([section 4](stage4-space.md#4-the-workers-launch)). A field a later build adds is read
past, since a record outlives the build that wrote it.

## The states and their steps

`opening` → `open` → `closing` → `kept` or `closed`; a step out of this order is refused
naming the state, and every step is written before the act it names is answered (SP2).

- **Create**: the record in `opening`, with the worker fixed — harness, name, role, session
  id — before anything of it starts.
- **SetKeeper**, **SetCheckout**: in `opening` only, each once.
- **Opened**: `opening` to `open`, with the worker's run; refused without the checkout, so
  the checkout is in the record before the worker can run in it. S4-7 writes it before the
  host opens the worker's launch gate, so every run of a worker's harness has a record that
  names it.
- **BeginClosing**: `opening` or `open` to `closing`, with why; a second one keeps the
  first, since main's exit is seen both by its mod and by its harness ending.
- **Close**: any state not over to `kept`, when the checkout was found still there, or to
  `closed`. It takes only an observation: the worker `ended` (its wrapper, which had a run,
  seen gone) or `never-started` (no run recorded, so its gate never opened and nothing of its
  harness ran), and the checkout found gone, never made, or left with what was in it —
  changes, commits beyond its base, a Git lock such as a dead keeper leaves. An end that says
  nothing, or contradicts the record, is refused. Any state may close, since the next start
  closes a record whatever step its dead main reached.
- **Cleared**: `kept` to `closed`, when the checkout is seen gone at that moment; a checkout
  still there, or one that cannot be looked at, keeps the record kept.

Every record, whatever step wrote it, holds its own invariants, and one read that breaks them
is unreadable rather than half believed: an end exactly when the state is final, `open` with
its checkout and its worker's run, `kept` exactly while a checkout left is not seen gone.

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
