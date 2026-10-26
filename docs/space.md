---
description: A space — main starting a worker through a detached space host, the worker's launch, the space record and its states, the listing, closing, kept checkouts and what a restore would read.
---

# A space for a worker

A space is main starting a worker: `rewake space open` runs a space host outside main's process
tree, which runs an ordinary launch of the worker in a pseudo-terminal it owns, serves that
terminal to the shell's pane, watches main's run and ends the worker when that run ends. The
rules a space keeps are SP1–SP10 ([rules/space.md](rules/space.md)); how the shell shows the
worker is [shell.md](shell.md); the checkout a worker gets is [worktree.md](worktree.md).

## Opening

`rewake space open <name> --alias <alias> [--worktree[=<name>] | --no-worktree] [--type <type> | --no-type] -- <harness args>`
is main's way to start a worker. A worker is raised only from a template: `--alias` is
required, `rewake workers` lists the person's worker aliases, and the alias supplies the
harness, role, command wrapper, args and applied settings; there is no `--harness` or `--role`,
and a call with either is refused as an unknown flag. The space owns the name and the room. The
fields and their precedence are in [launch-defaults.md](launch-defaults.md#the-worker-catalogue),
worker types in [worker-types.md](worker-types.md).

Opening is a change, refused unless main's run has `Spaces` live, and refused for a name whose
session is already alive. It writes the space's record and re-executes rewake as a launcher in a
session of its own (`Setsid`); the launcher starts the space host and exits, so the host is
reparented away from main. The space host takes the room's build lease as any writer, records
its own run, and refuses the open — the record closed, the worker never started — unless its
whole chain of parents reads up to the root without meeting main's wrapper (SP9). Then it opens
a pseudo-terminal of 120 by 40 with `host/terminal`'s pair and starts its own executable there
as `rewake [--command <wrapper>] <harness> [--worktree…] <harness args>`, in main's working
directory.

The command waits until the record is open and the worker's session is registered with the
worker's run, at most 30 s, and fails with the record's reason otherwise. The view socket is
listened on before the worker starts; an open that cannot serve it is refused. The space host's
standard streams are `/dev/null`: what goes wrong shows in the record's end or as the open's
timeout.

## The worker's launch

The worker's environment is the space host's less what names a session or a transport —
`REWAKE_SESSION`, `REWAKE_EPOCH`, `REWAKE_ROOM`, every `REWAKE_BRIDGE_` variable — and less every
catalogue harness's `EnvNotInherited` (`spacehost.WorkerEnv`). The host then sets `REWAKE_DIR`
to main's base, so the worker's room is main's room under the same base, `TERM`, and
`REWAKE_SPACE=<room>/<space id>`. Beside it, as the open asked: `REWAKE_SPACE_TEMPLATE=<alias>`,
`REWAKE_SPACE_ENTER=1` for a template that lets the worker enter a checkout mid-session, and
`REWAKE_SPACE_TYPE=<type>` with `REWAKE_SPACE_TYPE_TEXT=<its text>` for a worker raised for a
type, which `space open` resolved in main's directory. The person's preferences
(`REWAKE_CLAUDE_MODEL`, `REWAKE_WORKTREES` and the like) stay.

The launch is that worker, not main, only when `REWAKE_SPACE` names a space of an existing room
that is `opening`, or already `open` on the launch's own run, and whose recorded host run — pid
and start time — is the launch's parent. It then takes the name, role and room from the record,
the template and the type from their companions, and unsets them; the type, else the template,
names the worker's memory on Claude Code. Anything else is refused, exit 1, pointing to
`rewake space open`: a person who set the variable would start a worker past main.

Where the harness takes a conversation id (`ConversationOffer`,
[rules/adapter-api.md](rules/adapter-api.md#conversation-names)), rewake draws a random UUID
before the launch, records it, and inserts the harness's flag before any prompt terminator:
`--session-id <id>` on Claude Code and on pi. On pi it names or creates that conversation and is
not by itself a continuation; resume, continue, session and fork flags are still refused. A
harness without it gets no id, and its record says so. The worker's session is
`<name>-<harness>`; the address is not stored, it follows from the name and the harness.

What a harness needs to start unattended beyond what its adapter sets — a Claude Code worker's
onboarding and folder trust are set by the workers' profile ([launch.md](launch.md)) — main
passes after `--`, a permission mode among it. rewake types nothing into the worker on its own,
only the person's keys from main's shell.

## The stream to the shell

The space host serves `sock/space.<id>.sock` to the shell as a raw stream: a preamble from the
emulator that keeps the worker's screen, then the worker's bytes, and the keys and sizes the
shell sends back; the last size set is the worker's ([shell.md](shell.md)). The socket is open
to any process of the user, as every room file is, and takes no request of the mod.

## The record

One JSON file per space, `spaces/<id>.json` in the room ([design.md](design.md#state-directory)),
written only through `internal/infra/state`, so it refuses without the room's lease and in a
view ([rules/state.md](rules/state.md)). The id, 32 hex digits drawn at random, also names the
space host's socket: a closed record stays for what a restore reads, and its name is free again
for a new space. The name is unique among the room's spaces that are not over, checked when a
record is created. Every step runs under the room's space lock, `spaces/.lock`: it reads the
record, checks the step against its state, and replaces the whole file in one write, so a crash
between two steps leaves the earlier one whole.

| Field | What |
|---|---|
| `id`, `name` | the space |
| `state` | below |
| `main` | the run that opened it: name, epoch (the wrapper's pid, start time and boot) and pid namespace |
| `worker` | harness id, name, role, worker type, the session id rewake drew before the launch (absent for a harness that takes none), and its run once its wrapper is seen running |
| `checkout` | path and branch of the worktree the worker's launch made, once its session is seen there; absent for a worker started without one |
| `host` | the space host's run, as an epoch, recorded before it checks anything |
| `openedAt`, `closing` | when it opened; why it began to close |
| `end` | what closing observed: when, why, the worker's end, the checkout as found — there, gone or unknown, never left out |

A field a later build adds is read past, since a record outlives the build that wrote it.

### The states and their steps

`opening` → `open` → `closing` → `kept` or `closed`; a step out of this order is refused naming
the state, and every step is written before the act it names is answered (SP2).

- **Create**: the record in `opening`, with the worker fixed — harness, name, role, session id —
  before anything of it starts.
- **SetHost**: in `opening` only, once — the space host's own run.
- **Opened**: `opening` to `open`, with the worker's run; refused without the space host's run,
  written once the worker's wrapper is started and before the open is answered.
- **SetCheckout**: in any state not over, once, when the worker's session is seen to claim a
  worktree — `closing` included, since the worker can claim it after the space host last looked.
- **BeginClosing**: `opening` or `open` to `closing`, with why; a second one keeps the first.
- **Close**: any state not over to `kept`, when the checkout was found still there or could not
  be looked at, or to `closed`, when it was seen gone. It takes only an observation: the worker
  `ended` (its wrapper, which had a run, seen gone) or `never-started` (no run recorded, so
  nothing of its harness ran), and the checkout's presence, always stated — `gone` (or never
  made), `there` with what was in it (changes, commits beyond its base, a Git lock), or
  `unknown`. An end that says nothing of either, or contradicts the record, is refused.

`kept` is final: nothing records a kept checkout cleared, and the listing stops reminding of
one once it is gone.

Every record holds its own invariants, and one read that breaks them is unreadable rather than
half believed: an end exactly when the state is final, `open` with its space host's run and its
worker's run, `kept` exactly while the checkout was not seen gone at the close.

## The listing

`rewake space list`, a view, reads every record in the order the spaces opened, with a record
that does not read named apart and never taken for absent. For a space not over it says whether
its main still runs — "main ended", or "main unknown" when that main's pid namespace is not this
one's. A kept checkout is listed while it is there or cannot be looked at, with what was found in
it and the Git commands that clear it once what is wanted is merged — `git worktree unlock` for
a locked one, `git worktree remove`, `git branch -d` — each of which refuses rather than lose
work; rewake runs none of them. A kept checkout removed by hand is no longer listed as kept work,
though its record still reads `kept`.

## Closing

**Main ends.** The space host reads main's run once a second (`ObserveRun` through
`space.MainRuns`). Once it reads ended, it sends SIGHUP to the worker's process group, SIGTERM
after 1 s, SIGKILL after 5 s more, waits up to 2 s for the group to be gone, records the end it
observed and exits; a run read as unknown ends nothing. The sequence usually starts earlier:
main's mod sends `session/closing` at `session.end` of an exit, and a main in the shell then moves
each of its open spaces to closing with "main's session ended" (`space.BeginClosingFor`), which
the hosts read at their next tick (SP1).

The group is gone once only zombies are left in it: an orphan of the worker waits there for
whoever adopted it to reap it, and counting it left records closing with nothing of the worker
running. A process whose leader thread is a zombie is not one while another of its threads
runs, as after `pthread_exit` of its first thread. A group not seen gone, or a checkout that
could not be looked for, leaves the record closing rather than record an end nobody saw.

**Main closes one.** `rewake space close <name>`: a worker is never switched or restarted in
place; when another one is needed main closes it and raises a new one from a template, and there
is no restart verb. Close is a change, refused as a wrong call from any role but main and for a
name no open space holds, and refused, exit 1, for a space another run of main opened or one
still opening. It moves the record to closing with "main closed it with rewake space close" and
signals nothing itself: the space host re-reads the record at each tick and ends the worker by
the same sequence, recording that reason with the signal that took. A space whose host is proven
gone is finished by the close itself, by the same sequence (`spacehost.FinishGone`). Close
answers once the record is closed or kept, at most 20 s, and says so otherwise. A task the worker
still owed reads as lost in main's `rewake inbox --awaited`, as any ended worker's does.

**The checkout.** The close removes the worker's checkout and its branch when nothing in them
would be lost, every commit already on another local branch (SP7); a kept one, its reason on the
answer's second line, stays until `rewake worktree finish <topic>` or `rm`
([worktree.md](worktree.md)). The worker's `worktree_status` tool tells it what the close would do
([worktree-tools.md](worktree-tools.md)).

**A conversation ends, the process goes on.** `clear` and `resume` end main's conversation, not
its run. Spaces belong to main's run, so they close nothing: workers go on, and their mail with
them.

**A worker ends by itself.** The space closes the same way, with its exit and its last line.

Known limit: a record whose space host died with the machine stays open in the listing; no start
of main closes it (SP8 is not built).

## Restore

Not built. Every record keeps what a restore would read — the worker's harness, name, role,
session id and checkout — and `rewake space list` shows closed spaces with their session ids.
