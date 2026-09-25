# Remote control: the letter

How the outcome of a compaction main asked for with `rewake compact` reaches main after
the command has ended. The commands, the control directory and each harness's served
side are in [remote-control.md](remote-control.md); the Codex side in
[remote-control-codex.md](remote-control-codex.md); the tests in
[remote-control-tests.md](remote-control-tests.md).

When a compaction the command answered `started`, `requested` or an open `failed` — one
that leaves the outcome undecided — ends, main's wrapper puts a letter into main's inbox, from
the worker, of kind `notify`:

- "Rewake: compacted `<worker>`: N tokens before, M after (compaction K)." — the counts
  when the harness gave them, and the session's count of compactions with this one, as
  the "context compacted" notice it replaces would say;
- "Rewake: `<worker>` refused the compaction you asked for: `<reason>` (`<detail>`).
  Nothing was compacted." — a refusal after the answer, such as a conversation too
  short once the host had started;
- "Rewake: the compaction of `<worker>` you asked for failed (`<detail>`)." — the host's
  error after the start, or on Codex a compaction that did not end within its bound or
  lost its connection.

**Why `notify` is safe.** The letter reuses the note kind the compaction notice has
always had, with the notice's `compaction` metadata when the count is known; no new
kind is added. A note owes nothing (`inbox.Owed` is false for it); a blocking
`--question` takes only a message in reply to it, and the letter replies to nothing;
the announcement and the rendering already know the kind. Nothing that tracks reports
treats it as one.

**Where it comes from.** The served side records how the compaction ended in the
worker's telemetry snapshot (`compactionOutcomes`: request, asker, outcome, reason,
detail, tokens, time; the last 16 within a 3 KiB budget, each detail cut to 300 bytes,
so the snapshot stays under its 16 KiB limit with 64 compaction events beside them —
a snapshot over it would not be saved at all, and the whole telemetry would stop) — on Claude Code the module's `compact.ended`, which the collector
folds; on Codex the wrapper's background wait. A final answer — a refusal, a final
failure — is recorded as the outcome too, after it is written: a command whose wait was
cut short once the request was taken holds an open answer, and its letter then comes
from this at once rather than at the bound. A command that read the final answer has
already closed its record, so for it the outcome sends nothing. main's wrapper already reads each
worker's snapshot once a second for the compaction notices; it pairs the outcome with the
compaction the telemetry counted under the same request and sends one letter for the
pair. The two halves come apart — on Claude Code the count comes from the `PostCompact`
hook, which runs in the background — so a done outcome waits up to 3 seconds for its
count, and a count for its outcome; after that the letter goes with what came and says
the telemetry has not counted it. A refusal or failure has no count to wait for. A
worker that leaves sends what it has at once. The letter's id is derived from both runs
and the request, so a scan repeated after a failed write sends it once.

**Who owes it.** main's side, not the worker's — a design choice of the orchestrating
session after the first review of September 25, 2026, not an owner decision. The worker's side
may never record an outcome: a worker killed mid-compaction, a module reloaded, a report
to the collector that failed, a Codex wrapper whose background wait died with it. So
`rewake compact` leaves a record in main's state
(`<REWAKE_DIR>/rooms/<room>/letters/<main>/<id>.json`: the request, the worker's
registry record, main's run, the time) before it writes the request, with the request's
id chosen beforehand; a command killed after the worker took the request leaves it too.
The command removes the record itself on every answer main already has in full: a
refusal, a final `failed` — the served side's word that nothing was carried out, such as
Codex's "compaction is disabled" or a host call that failed before its start was seen —
the request never written, and `done` from a served side of an earlier rewake. Only an
answer that leaves the outcome open keeps it and promises the letter: `started`,
`requested`, and a `failed` marked `open` — the command's wait ran out or was cut short
once the request was taken, which "may still be carried out", or the Codex wrapper sent
the request and lost it, with no reply or with its connection ending. A decision of the
orchestrating session on September 25, 2026, after the second review.

The command holds an exclusive `flock` on the record from before it gets its name — the
file is locked, then renamed into place — until the command has decided whether it
stays, and main's wrapper leaves a held record alone. Without it the wrapper could close
the record while the command still asks: a worker leaving during the pickup got a
"failed, left before the compaction ended" letter while the command answered a final
`not answering`, two answers that contradict each other (seen live by review-claude on
September 25, 2026, three times in three). The kernel drops the lock when the command
dies however it dies, SIGKILL included, and the record is closable then. A decision of
the orchestrating session after the third review.

The wrapper opens the record before it locks it, and a command may remove the record
and let go in between: the lock is then free, on a file no name leads to, and a wrapper
that took it for the record would send a letter the command's final answer already
made needless — seen by review-codex on September 25, 2026, with an overlay: after a
final "compaction is disabled" had closed the record, a letter with the same error
came from its outcome. So a lock counts only when the name still leads to the locked
file, and the record is read again under it (a decision of the orchestrating session
after the fourth review).

main's own wrapper closes each record kept exactly once, by the first of:

- the outcome in the worker's snapshot, as said above — the snapshot stays readable after the
  worker has gone;
- the worker's run gone with neither the outcome nor the count seen 3 seconds after its
  departure was seen — a worker that finished and left at once may publish its last
  snapshot a moment later, and the snapshot is read again meanwhile: "Rewake: the
  compaction of `<worker>` you asked for failed (`<worker>` left before the compaction
  ended: `<reason>`)." It comes after the notice that the worker has gone;
- **5 minutes** from the request with neither seen while the worker lives on: "Rewake:
  no outcome of the compaction of `<worker>` you asked for was seen within 5m0s; rewake
  list shows whether it compacted." The bound is well past the 80 seconds of the Codex
  wrapper's wait and gives a Claude Code compaction, whose host call rewake does not
  bound, several minutes.

The record goes once its letter is in main's inbox; a letter whose write failed is
retried with the same id. So the promise in the answer holds: a letter comes, whether it
compacts or not. A command that could not leave the record says so instead of
promising — "rewake could not note it for its letter (`<error>`), so none comes: rewake
list shows whether it compacted" — with the error under `--json` as `noLetter`.

The letter is delivered by main's own wrapper, not by the command, which has exited:
it lands in main's inbox as any letter does, and is announced as one. The records are
kept by main's name, not its run: a letter to an ended run is never shown (see
[delivery.md](delivery.md#reading-rewake-inbox)), so a record main's run left behind is
closed by the next run of that main when it starts, the same way, to the new run, and
the letter adds "You asked for it in an earlier run of this session."
