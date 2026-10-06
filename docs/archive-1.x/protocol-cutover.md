# The protocol cutover

How a mailbox passes from an earlier build's protocol to the one of
[turn-end-recovery.md](turn-end-recovery.md), under clause 8-cutover of rule 8 in
[mail-bridge-cli.md](mail-bridge-cli.md). It was written on September 30, 2026, before
its code, which landed the same day — the look in `internal/cutover`, the launch order in
`internal/wrap/claim.go` and `internal/wrap/cutover.go`, the refusals in
`internal/cli/self.go` — and follows the owner's decision of that day: a session started by an earlier
build stays on that build's protocol until it is restarted by resuming its conversation,
and nothing is restarted automatically.

A run's protocol is its whole life (8-cutover): the earlier build's wrapper keeps that
build's code in memory, while its hooks and its shell calls run whatever binary is
installed, so this build's code refuses to act for it at all. Which build a run followed
has to be known after its session record is gone, since the registry keeps only the
current run of a name.

## The launch

A launch of this build, in this order:

1. Proves every earlier-build writer of the name stopped, or refuses, exit 1, naming
   each process it could not clear and why (below). Everything before the proof only
   reads, the choice of the name included: a dead session record pruned there would
   take with it the namespace the proof refuses on.
2. Writes its run record, `runs/<name>/<boot>/<epoch>`, with the build stamp and its
   start on the boot clock. An epoch is a pid and its start in ticks since boot, and
   after a restart of the machine the same pair can name another process; the boot is
   the kernel's boot id, drawn at random at every boot, so the two together never recur.
   A launch that cannot read the boot id refuses. Run records are never swept.
3. Binds itself as the successor of the name's earlier-build runs, by creating
   `runs/<name>/successor`, holding its boot and epoch, only if it does not exist. The
   first run to create it is the successor for good; a later run never replaces it, even
   once it has ended. One found there that cannot be read refuses the launch, and is
   left as it is.
4. Publishes its session record, carrying its boot, its epoch and the build stamp: from
   here it is ready.
5. Takes the reports held for its predecessors: under each sender's mailbox lock, the
   journal records it as the report's recipient, then the report is published under its
   marks. Then it adopts as before.

The name's successor is in one of these states, and a sender's barrier tells them apart
under its own mailbox lock:

| Successor | Proven by | A report held for its predecessors |
|---|---|---|
| none bound | no successor record | stays held |
| starting | bound; its wrapper alive — the current boot, its pid and start — with no session record of its boot and epoch yet | stays held: the successor takes it at step 5, or the sender's next barrier once it is ready |
| ready | its session record, of the current boot, names its boot and epoch, and its wrapper is alive | published to it |
| gone | its boot is not the current one, or its pid and start are not alive | moot: the journal records that, and main is told once |
| not known | its wrapper's state cannot be read, or the successor record cannot | stays held, naming the path |

Only gone makes a held report moot; a missing session record is never taken for an
ended run, since between steps 3 and 4 it is missing by design. A launch that dies
before step 3 never became the successor, and the next one may; one that dies after
it stays the successor and is gone. The acceptance runs a sender's barrier between each
two of steps 3, 4 and 5, and a crash at each of them: a held report is published once
to a ready successor, stays held for a starting one, and is moot only for a gone one.

## Naming a run across restarts

Every run this build records — in a journal, a wait, a report, a held report, a
receipt, a once mark, a read-clock record or boundary, the successor record, the session
record — it names by boot and epoch, and a run of this build is running only when the
registry's session carries both and the boot is the current one. An epoch without a boot
is therefore the earlier build's, and names a run of that build: this build neither
writes into an earlier-build mailbox nor sends to one, and the letters an earlier-build
wrapper writes into this build's mailboxes are reports and notices that owe nothing, so
no earlier-build record names a run of this build. Such an epoch is never looked up
among run records, where after a restart it could match another run. A record that
names a boot whose run record is missing or cannot be read is unknown, never read as
the earlier build's. A test over every writer of a run's address keeps the boot in each.

## Proving the earlier writers stopped

The automatic cutover holds where the upgrade replaces the rewake binary at the paths
the earlier runs use; that is its boundary, and the look below is complete only within
it. An earlier run starts rewake by a path fixed at its launch: its hooks, its plugin
and its status line keep the absolute path `os.Executable` gave its wrapper, and the
directory of that path is in its `PATH`, put first when it was not there already
(`internal/harness/environment.go`). The reviewer confirmed this from the source of
v1.0.3 on September 30, 2026, and found no production code there that runs its own
image again through `/proc/self/exe`; a Codex app-server is the harness's own program,
and the relay runs inside the wrapper. So once those paths hold this build, whatever an
earlier run starts next is this build, which refuses to act for it, and the earlier
build's rewake processes can only grow fewer. Another installation, a local build or a
saved copy still at a path an earlier run uses is outside the boundary, and the
automatic cutover does not promise to cover it; so is a rewake binary given setuid or
file capabilities (below).

Within it the launch lists `/proc` once, over the person's processes, and refuses, exit
1, naming each pid, the reason and the next step — to stop that process or wait for it,
never killing anything itself:

- Namespace: the name's session record, if one is left, names the pid namespace the
  launch sees. One naming another, or none, refuses: its run is out of sight.
- Wrappers: every epoch of the name that a record names is checked by its own pid and
  start. `registry.Session.Alive` is not this check: it answers for the harness, and
  says no while the wrapper may still publish.
- Every rewake process of the earlier build — known by the module path in the build
  information of `/proc/<pid>/exe`, without this build's stamp, which reads even once
  the file is replaced — including one that no session record names. It is cleared only
  when it belongs to another session by its whole address, state root, room and name:
  its environment names that address, or a record under that address names its own pid
  and start. One of this name, one that cannot be attributed, and one whose environment
  cannot be read refuses. A wrapper whose session record was pruned while it publishes
  is caught here.
- The person's processes are the ones whose `Uid:` line in `/proc/<pid>/status` names
  the person, by any of its four ids — never judged by the owner of the files under
  `/proc/<pid>`, which the kernel gives to root while a process is not dumpable. A
  status that cannot be read refuses: the process is not proven another user's.
- A process of the person whose executable cannot be read is cleared only when its
  status proves a cause an earlier-build rewake inside the boundary cannot have: its
  permitted or inheritable capabilities are not a subset of the launch's own, or it is
  not dumpable — its status owned by root while its `Uid:` is the person's. Any other
  unreadable executable refuses — dumpable, no capability beyond the launch's, and still
  refused, as a security module may: it is not proven to be something other than
  rewake. The ground, verified on September 30, 2026 by searching the production code
  of v1.0.0 to v1.0.3: no earlier build calls `prctl(PR_SET_DUMPABLE)`, sets a user or a
  group id, or touches capabilities (the one `prctl` is `PR_SET_CHILD_SUBREAPER`, in the
  workflow suite), so a rewake the person's harness or shell started holds no
  capability beyond theirs and stays dumpable. A rewake binary carrying setuid or file
  capabilities is outside the boundary. The rule was set by the owner's side on
  September 30, 2026, after the first build of the look refused every launch on a
  standard systemd desktop: there the person's own `systemd --user` (pid 971, `CapPrm`
  `0x800000000`) and its `(sd-pam)` (pid 972, `CapInh` `0x800000000`, its status owned
  by root) run under the person's id and hide their executables, and they were the only
  such processes found. A process proven to be another program is not a writer,
  whatever variables it carries: what it starts later is this build.
- A refusal for an earlier-build rewake process whose executable is still on disk at its
  path names that path too, as one the upgrade did not replace. One attributed to
  another session is foreign to this name and blocks nothing here.
- A process that ended during the look is not a writer: its start is read before and
  after its files, and one gone, a zombie, or replaced under its pid meanwhile is
  cleared, while one whose state cannot be read is judged by what its files say.

The refusal names each process by its pid and one of these reasons: an unknown user, an
unknown executable, an earlier rewake, a run out of sight, or an earlier run still
running (`internal/cutover`). One pass over `/proc` took about half a second on a
desktop with a few hundred processes, measured on September 30, 2026.

No test depends on the processes of the machine it runs on: the look's judgements are
tested over process trees the tests describe (`internal/cutover/scan_test.go`), the tests
of a package that launches replace `cutover.Tree` with an empty tree, and the workflow
suite builds its binary with `builtProcRoot` set to an empty tree of its own
(`test/workflow/timings_test.go`). A release build sets none, and lists `/proc`.

A process of another user cannot write the state directory, which only its owner may;
root, a run in another namespace whose record is gone, and an earlier-build binary left
at a path in use are beyond the look. The barrier is the signal that the boundary was
crossed, not a proof after the fact: an earlier-build record found after the successor
is bound — a receipt the conversion journal does not hold, a wait without its
read-clock position — stops the mailbox (8-stop), naming it. An earlier-build hook can
still publish and leave no record at all
([turn-end-recovery.md](turn-end-recovery.md#what-stays-open)).

## What this build refuses or holds

- A call made by a run of the earlier build that would change any mailbox — its hooks'
  turn ends, `rewake inbox`, `rewake pending`, `rewake send` — is refused, exit 1:
  rewake was upgraded after this session started, and the session must be restarted by
  resuming its conversation; nothing is lost. Reading without changing still answers.
- A call of this build that would change the mailbox of a running earlier-build run — a
  send, a question — is refused the same way, naming the session.
- A report this build publishes to such a run is held, which is neither unknown nor a
  stop (step 5 of the [reconciliation](turn-end-recovery.md#reconciliation)): the
  journal keeps it whole and carries its obligations. It goes to the successor, recorded
  in the journal as the recipient before it is published under the successor's marks,
  and the report keeps the original recipient run beside it, since that run is what its
  obligations were owed to. What happens to it follows the successor's state (the
  launch, above): held while none is bound, while it is starting and while its state is
  not known, published once it is ready, and moot only once it is gone, with the
  journal recording that and main told once.
- Main is told once per earlier-build run, by a note whose id is named by that run, from
  the first refusal. A main of the earlier build hears nothing until it is resumed; its
  successor's start sends itself the same note for every earlier-build run still
  running.

## What the earlier build leaves

Letters another session's earlier-build wrapper still delivers — its completions, the
plugin's stop — are letters like any other in a mailbox of this build; that wrapper
writes none of the mailbox's own records. The reviewer confirmed this from the earlier
build's source on September 30, 2026: its completion, plugin stop, availability and
departure paths write into another session's mailbox only letters, through `PutOnce`,
and `PutLocal` only into their own. When the earlier run has stopped, a run of this
build may take the name, and its adoption runs the barrier over what the earlier run
left: its receipts through the conversion journal (8-origin), and its kept answer,
pending marks and interim record as another run's, never taken, whatever their text. The
waits such a kept answer might have answered stay in the wait records, where adoption
finds them, and the new run's next end answers them in its own words.
