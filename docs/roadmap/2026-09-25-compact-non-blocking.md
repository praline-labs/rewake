# `rewake compact` that does not wait for the end — September 25, 2026

`rewake compact` waited for the compaction to end, up to 80–90 seconds, which stalled
the orchestrator on one shell call. It now returns once the request's outcome is known —
refused at once, or the compaction started — and the result reaches main later as a
letter, the way `send` returns at once and the report comes later. The same on both
harnesses; `rewake interrupt` is unchanged. The design as built is in
[remote-control.md](../remote-control-letter.md) and
[remote-control-codex.md](../remote-control-codex.md).

**The owner's decision** (September 24, 2026), recorded in
[remote-control.md](../remote-control.md#owner-decisions): the command does not block;
the outcome comes as a letter.

**What was built:**

- Two new answers in `internal/control`: `started` and `requested`, both exit 0. The
  command's outcome limit is 10 seconds for a compaction, as for an interrupt; the pickup
  limit stays 5.
- **Started, on Claude Code.** The host's `$.session.compact` resolves only at the end,
  and the module sees none of its own compaction's hooks. The collector does: when a
  `PreCompact` arrives while a main's word waits for its compaction, it writes
  `<id>.started` into the control directory. The module races the call, that file and a
  3-second bound: the file or a settled call answers `started`, a refusal before either
  is the answer, the bound answers `requested`.
- **Started, on Codex.** The reply to `thread/compact/start` only says the request was
  queued, and a `turn/started` alone does not say whose turn it is. The tie — the
  compaction's `contextCompaction` item in a turn with no other item — answers
  `started`. No tie within 3 seconds
  answers `requested`, saying whether the server had taken the request.
- **The end.** The module goes on waiting for the call and sends `compact.ended` with the
  outcome and the host's counts; the Codex wrapper waits in the background as before,
  with the same mark and 80-second bound. Both record the outcome in the worker's
  telemetry snapshot (`compactionOutcomes`, the last 16).
- **The letter.** main's wrapper, which already reads each worker's snapshot once a
  second for compaction notices, pairs the outcome with the compaction the telemetry
  counted under the same request, and puts one `notify` from the worker into main's
  inbox: "compacted `<worker>`: N tokens before, M after (compaction K)", or the refusal
  or failure with its reason. A half waits up to 3 seconds for the other; a worker that
  leaves sends what is there at once. It replaces the "context compacted" notice main
  never got for its own compaction.
- **Why `notify`.** The letter reuses the kind the compaction notice always had: it owes
  nothing, no blocking `--question` takes it — it replies to nothing — and the
  announcement and rendering know it. No new kind, so none of the places listed in
  AGENTS.md under "Adding a role or a message kind" needed a decision.
- A second request while the module's compaction runs is refused as `in a turn`: its
  word would take the first compaction's place.

**Tests.** Unit: the command's `started` and `requested`; the module under node — an
answer before the end on the start mark, the end told with the counts, `requested` on the
bound, a failure after the answer told as failed, a second request refused; the
collector's start mark and `compact.ended` decoded and kept; the gateway answering on
the tie and `requested` without one; the letters — one with the numbers, a refusal and a
failure, the wait for a half, a worker leaving. The workflow cases `claude-steered` and
`codex-steered`: the fixtures' compaction takes five seconds, the command answers
`started` within three before the telemetry counts it, and the letter arrives with the
numbers. New mutants: the module and the Codex wrapper answering only at the end, the
module ending without the counts, the Codex wrapper not keeping the outcome, and a letter
without the count; thirteen on the Claude Code side and seven on Codex.

**Review round 1** (review-claude, live on Claude Code 2.1.280, evidence kept by the
reviewer): started in 0.34 s, one letter with the numbers and no notice; mid-turn, a
second request and a conversation too short refused. Two findings:

- *Medium — after `started` or `requested` the letter could never come.* main's wrapper
  learnt of a request only from the worker's snapshot; a worker whose claude was ended 2
  seconds into a 15-second compaction left main only "Session is no longer available".
  The same by the code for a report to the collector that failed, a module reloaded,
  and a Codex background wait that died with its wrapper. Fixed by the orchestrating session's design choice, not an owner decision: main's
  side owns the request. The command leaves a record in main's state
  (`internal/control/pending.go`), and main's wrapper closes it exactly once — the
  outcome from the snapshot, a failed letter when the worker's run is gone without one,
  or a letter at 5 minutes; a record an earlier run of main left is closed by the next.
  A command that could not leave the record promises no letter.
- *Minor — an answer the module could not write stranded `compacting`*: the rest of the
  compaction was started only after the write, so every later request was refused as
  `in a turn` and no `compact.ended` came. It now starts in `finally`.

Tests: the command's record and its absence on a refusal, the promise withdrawn without
a record; the module under node with the answer's write failing; the letters of a
worker gone, of the bound and of an earlier run; the `claude-steered` case ending calm
mid-compaction, and its mutant — fourteen on the Claude Code side.

**Review round 2** (review-claude live on Claude Code 2.1.280; review-codex reproduced
his two with overlays). Four findings, each first as a failing test:

- *A `failed` answer after the request was taken left no record* — the command's own
  signal ended it 0.22 s after the pickup, the compaction ran 5 s later and counted as
  main's, and main got neither a notice nor a letter. The command now leaves the record
  before it writes the request, with the id chosen beforehand, and removes it only on a
  refusal, a request never written, or an earlier rewake's `done`; every other answer
  promises the letter, and a command killed after the pickup leaves the record too.
- *A worker that finished and left at once was reported as having left mid-compaction*
  from a snapshot read before its last one was published. main's wrapper now waits 3
  seconds after a departure, reading the snapshot again, before the failed letter.
- *Codex: a compaction lost sight of before its item tied the mark answered
  `started`*, and its letter then said it failed. An end of the wait before the tie —
  sight lost, or the mark's bound — now answers `requested`, "its start was not seen:"
  and the reason.
- *Codex: sixteen outcomes with long details pushed the snapshot past 16 KiB*, and a
  snapshot that does not save stops the whole telemetry. The outcomes now keep within a
  3 KiB budget, oldest dropped first, each detail cut to 300 bytes and a reason to 64; a
  snapshot at its worst case saves.

The `claude-steered` case now kills calm's harness in the middle of its second
compaction rather than stopping it: stopped, it sometimes finished the compaction first.
Two controls that make the scenario slower — an answer at the compaction's end, and
any role steering, which adds a pickup wait — then broke the letter too: the Claude
Code fixture ends itself after 25 seconds, and took main down before the letter was
due. A fixture session serving the scenario's requests stays up 85 seconds now, as on
the Codex column. Under the load of the full suite, the first of the two let the
compaction end before the kill, and the letter came from the count alone, which the
case did not take for one; it now takes any new letter of calm's compaction.

`remote-control.md` passed 400 lines, so the letter moved to
[remote-control-letter.md](../remote-control-letter.md).

**After round 2** (the orchestrating session's decision): a final `failed` from the
served side — nothing carried out, as Codex's "compaction is disabled" or a host call on
Claude Code that failed before its start was seen — is an answer main already has, like a
refusal: the command removes its record and promises no letter. The answer gains `open`,
which marks a failure that leaves the outcome undecided: the command's own, once the
request was taken, and the Codex wrapper's two for a request that went out and got no
reply or lost its connection. Only `started`, `requested` and an open `failed` keep the
record. The known limit of a 5-minute letter after a final failure is gone. Tests: the
command with a final and an open failure against a fake served side, `Ask`'s own failure
open, the Codex refusal final and the unanswered request and the ended connection open,
the module's failed host call final.

**Review round 3.** review-codex accepted the Codex side with no findings; review-claude's
live re-run passed all three cases and found a race. The orchestrating session's
decisions:

- *main's wrapper closed a record while its command still asked.* A worker that left
  during the pickup got a "failed, left before the compaction ended" letter while the
  command answered a final `not answering` — seen live three times in three, a `--bare`
  worker whose harness got SIGTERM 0.05 s after `rewake compact`. The command now holds
  an exclusive `flock` on the record — taken before the file gets its name — until it has
  decided whether the record stays; main's wrapper leaves a held record alone, and the
  kernel lets go of a killed command's. Tests: the race, and a command killed with
  SIGKILL whose record is then closed.
- *An open answer followed by a refusal got its letter only at the 5-minute bound*: the
  module sent `compact.refused`, not an outcome. Every final answer to a compaction is
  now told as its outcome as well, once written — by the module as `compact.ended`, by
  the Codex wrapper into the snapshot — so that letter comes at once. A command that read
  the answer has closed its record, and the outcome sends nothing.
- Two stale comments corrected: the order of a letter and the departure notice, and
  which answers keep the record.
- The suite takes about fourteen minutes: 13m36s for a hundred cases on September 25,
  against four minutes for 63 on September 23 and seven for 85 on September 24.
  `claude-steered` grew most — from 8 cases of about 10 seconds to 15 of about 23, six
  minutes in all — then `codex-steered`, about two minutes. `docs/testing.md` says so;
  nothing was made faster.

**Review round 4.** Two findings, both fixed:

- *review-codex: a record closed between the wrapper's open and its lock was taken for an
  open one* — reproduced with an overlay: after a final "compaction is disabled" had
  closed the record, main got a letter with the same error from its outcome. The lock
  now counts only when the record's name still leads to the locked file, and the record
  is read again under it. Tests: a file removed, and one replaced, between the open and
  the lock; the record read again under the hold.
- *review-claude, minor: two final answers on Codex were not recorded as the outcome.*
  A request withdrawn before it was taken, which `control.Serve` answers itself, is now
  handed back to the wrapper once written and recorded. "The app-server is not connected
  yet" is not: it is not reached, as the control directory is served only once the
  gateway is there, and without the gateway nothing holds the telemetry.

**What stays open.** The live run on both harnesses, and the Codex-side acceptance: the
Codex path changed. A module reloaded mid-compaction sends no `compact.ended`, and the
letter then goes from the count alone, without the tokens (Known limits in
[remote-control.md](../remote-control.md#known-limits)).
