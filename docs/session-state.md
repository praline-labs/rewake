---
description: What main sees of its workers: the state header, the session table, activity labels, availability, compaction and departure notices, and where telemetry comes from.
---

# What main sees of its sessions

State is collected for every session through its adapter's telemetry; only a caller whose
current registry name, epoch and main role are verified sees it. Workers and plain shells get
none of it, in text or JSON, and no cross-room discovery or messaging is added. An adapter
that reports nothing, and a snapshot that is absent, read as unknown.

## The state header

Every message main reads — tasks, notes, reports, and a waiting question's direct answer —
comes under a header that starts with the sender's session name and a colon:

```text
write-claude: working | context 42% used / 272K | compactions 2 | model "…" | effort "high"

from write-claude ...
<original message>
```

It stays above every message, not once per read and not only when a value changed, so main
always knows each session's state at the message in front of it. The header is tool output,
not a chat message; agent text, receipt ids and message storage are unchanged. JSON nests
`telemetry` before each message's own fields; list entries and direct answers also carry it,
only for main. Exact token counts are kept. A value that is unknown says so; one that is
stale is marked `(stale)`, a partial compaction count `(partial)`, and a snapshot that is
missing or stale altogether ends the line with `state unavailable/stale`
(`internal/cli/session_state.go`).

## The session table

`rewake list` prints one aligned row per session. The room appears once; a working directory
and a harness common to every row appear once in the header, and differing values add
Directory and Harness columns. A verified main sees Session, Role, Status, Model, Effort,
Context, Compactions and Age, a Type column when some worker was raised for a type of work,
and a Cache column when some harness reports its prompt cache
(`internal/cli/session_table.go`). Worker and plain-shell tables leave out every telemetry
column. Addresses and values are never cropped; quoted paths and settings escape control
characters, so a value cannot introduce another row. Machine-readable output keeps the full
model, exact values, field visibility and the empty list's shape.

## Activity

Each snapshot carries the session's activity, with its own observation time and freshness.
What it amounts to is judged once, in `internal/core/session` (`Snapshot.Judge`, the
**activity verdict**), and every reader renders that verdict and derives nothing: `rewake list`'s
Status and the letters' state header take its label, main's report to herdr its activity and
what it waits on ([launch.md](launch.md#herdr)), and the shell's strip its label and the worker's
button its mark ([shell.md](shell.md#the-bar)). The verdict is judged at the read, never stored,
and JSON carries it as the telemetry's `verdict` — `activity`, `label`, `blocked`, `stale`,
`mark` — beside the fields it was judged from. The activity is one of:

- `idle` — no turn running; `working` — a turn running, with nothing it is known to wait on.
- `compacting` — a known compaction in progress, in place of either.
- `waiting for a permission` — a worker's permission request is held for main to answer, and
  `waiting for an answer` — the model's own question is
  ([permission-requests.md](permission-requests.md)); the snapshot's `waiting` names which
  (`permission`, `question`) from the mod's state, and the label clears when the request ends.
- `waiting at a dialog` — a dialog no request carries to main, which main is told of once
  ([permission-requests.md](permission-requests.md#a-dialog-nobody-is-asked-about)).
- `unknown` — no activity reported yet, a word the verdict does not know, or an adapter that
  reports none.

A waiting activity wins over the others, and `blocked` names what it waits on (`permission`,
`question`, `dialog`; empty otherwise). The label is the activity, with `; interruptions
unheard` when the snapshot says its interrupted turns go unheard (no adapter of this build sets
it) and ` (stale)` when the verdict is stale. The mark is the shell's button edge: `blink` for a
waiting activity or a `working` or `compacting` one gone stale — a turn stopped where nobody
sees —, `steady` for a fresh `working` or `compacting`, `off` for `idle` and `unknown`, stale or
not.

**One freshness rule.** An observed activity is stale unless both the snapshot is fresh
(`fresh`) and its activity observation is (`activityFresh`); an activity never reported is
`unknown`, never stale. Decided October 27, 2026, when the rule was found written four ways: the
CLI asked for both, herdr only for `activityFresh`, the shell only for `fresh`. The flags have two
meanings — the run is live and publishing, and the activity was observed — and every path that
marks a snapshot old (`Load` past the two-second heartbeat, a run no longer current in the
registry, a departure, an adapter that lost its link) clears both, so asking for both is the one
reading that stays right whichever a future source clears, on the stored snapshot and on the run's
own alike. Since the flags move together on every adapter of this build, no reader's verdict
changed; the shell's label gained `; interruptions unheard`, which the CLI already showed.

pi's extension sees no dialog — pi 1.2.0 loads no permission gate or question tool — so a pi
session reads `idle`, `working` or `compacting`. Unknown activity is never taken for idle.
Since October 25, 2026 the labels `working (waiting state unknown)`, `not loaded` and
`system error` are gone: no adapter of 2.0 reported them.

Current activity is not last outcome. A failed or interrupted turn does not latch an error
label: a later idle displays idle. A completed compaction does not force idle or working;
the header shows the latest state at retrieval. Idle alone does not establish that an
assignment was abandoned, and triggers no restart or resend.

## Availability notices

After the harness and the inbox service start, the wrapper records the session's messaging
readiness: an adapter must also have an accepted first delivery target, and a socket harness
its messaging socket. This needs no model turn. A failed start is not announced. The marker
is no claim of continuous connectivity, and a reconnect does not make a new launch.

Only the current room's main wrapper scans readiness records, once a second, and publishes an
ordinary `notify` with the frozen registered identity: its first line, then name, role,
harness, room and working directory, one per line, so main never has to ask for them. At
fetch, the header adds model, effort, context and compactions from that exact epoch's latest
snapshot. JSON includes `availability` identity and the usual `telemetry`. The note owes no
reply.

A main started later labels workers already ready as already available, rather than new
launches. Ids include both main's and the worker's epochs, and publication goes once through
`PutOnce` under main's mailbox lock, so a retry or a reconnect never repeats it; a reused name
and a replacement main have different identities. Main waits for its own delivery target
before queuing availability notices; storage and publication failures retry without blocking
the start, and a failed announcement cannot abort either wrapper or alter tasks and reports.

## Compaction notices

Each completed compaction of a session increments its observed count, once per completion
(deduplicated by the wrapper for its run; a new wrapper starts at zero). Main receives a
no-reply notify, `Rewake: context compacted (compaction <n>).`, whose header shows the
session's current activity and count; the worker is not woken by it. A compaction main asked
for itself with `rewake compact` is the exception: main's letter of that compaction's outcome
carries the count instead ([remote-control.md](remote-control.md#the-letter)).

The adapter keeps a bounded tail of 64 completion cues in memory, which the snapshot worker
persists and the main observer turns into notices. Each cue has its observation time and the
wrapper's sequence, and a notice's identity includes both epochs and the sequence, so retries
and reconnects publish it once. A late main ignores completions observed before its own start
and learns current totals through availability. The tail is a best-effort handoff, not an
event log: if observation or persistence lags past its capacity, older cues are not replayed;
the cumulative count is independent of it.

## Departure notices

The main observer remembers the available worker epochs and their last snapshots. Registry
removal, a definite replacement of the name's epoch, or confirmed process death produces one
no-reply departure notice with the same identity block as availability. A socket reconnect or
missing telemetry proves nothing. An absent pid, a different recorded start time or a zombie
confirms departure; unreadable process identity and an unjudgeable namespace stay unknown,
and the notice waits for a later scan. Exit causes are never labelled as crashes. A new main
does not announce workers it never observed. The departure keeps the old name, epoch, role,
harness, room and working directory, and its saved state is marked stale and never borrows a
replacement run's. Publication rechecks the evidence inside main's mailbox, never takes a
sender's name lock or cleans up registry records, and goes once through `PutOnce`.

## Claude Code source

The mod reports the native context use, the model's window and the percentage, the
configured model, activity, compaction state and the prompt cache, sampled after its hello,
measurements, turn and compaction events, and each heartbeat answer; its observations stay
fresh only while the current mod's heartbeat holds ([claude.md](claude.md#telemetry)).
The window and percentage are the harness's, unless the launch limited the window below the
model's: that window is shown, and the percentage is of it. A fresh conversation or a
completed compaction may have a known window and unknown usage until another response. Effort
is live: the status line's `effort.level` follows `/effort` — typed with a level or chosen in its
picker — at once, before any request, and the main loop's latest model request gives its own;
whichever source last changed its value is shown. Before either reports, it is the launch's own
`--effort`, shown as `"high" (launch)` and in JSON with `reasoningEffortSource: "launch"`, and
unknown when the launch passed none. The model's display name (`configuredModelName`) and
the session's thinking setting (`thinking`, on for a model without thinking as well) come from
the same status line, main's as a worker's;
`rewake list` drops a trailing parenthesis from the name. Not live: a session whose status line
does not run — a launch with no run directory, an untrusted workspace, a main launched with a
`--settings` of its own — shows the request's effort only, a turn late, the model's id and no
thinking ([research-claude-mod.md](research-claude-mod.md#effort)). The session's directory now
(`cwd`, `$.session.cwd()`, which a worktree entry moves) and the account's rate-limit windows
(`rateLimits`: `kind`, whole `percent`, `resetsAt`, from `$.session.usage().rateLimits`, none off a
subscription) come with every sample; a sample without them clears them. A
reload of the mod makes compaction coverage partial. On an organization-managed installation
the harness skips a local plugin's compaction observer, so a native compaction is not counted
there; one main asked for is still counted from its result.

## pi source

The per-run extension reads native context usage, window and percentage, model and thinking
level, pushes changes and answers a one-second poll
([research-pi.md](research-pi.md#telemetry-and-remote-control)). The thinking level is live: the
extension pushes a change on `thinking_level_select`, before the next request. `thinking` is
whether that level is other than `off`, told only for a model with reasoning
([research-pi.md](research-pi.md#telemetry-and-remote-control)). The percentage is rounded and
bounded to 100. Missing usage after a compaction stays unknown until the next usage update. No
limits are reported. The wrapper keeps only the current connection's increasing sample
sequence; a lost connection or three seconds without a sample makes its values stale. A model
change or a replaced conversation takes the new snapshot whole, unknown fields included.
Manual and automatic completions increment one count for the wrapper's life; a reconnect
makes coverage partial. No transcript file is read.

## Persistence and freshness

No adapter builds a snapshot. Each hands the session's observer (`internal/core/session`,
`Observer`) what its harness reported, as typed observations, and the observer alone merges
them; the fixture's adapter uses the same observer as the real ones. A sample is one report of
the primary state: it replaces the last whole, and stamps the observation time and the freshness
of each part it carries — activity, context, settings —, a part it leaves out staying unobserved.
Standing facts from a second source between samples (Claude Code's status line: the effort, the
model's name, thinking, the cache's life) are kept across samples and reloads and shown over a
sample's own. The effort shown is the one whose source changed last, else the sample's, else the
launch's, marked. Compactions are counted from one source at a time, by its running count (the
mod's, per load) or by numbered ends (pi's, per connection); a source following another, or a
dropped connection, makes coverage partial. At a read the adapter says only whether the source of
the last sample is still live and whether compactions are counted; the observer marks a sample
stale when it is not, or when it is older than the source's expiry (pi's three seconds).

One wrapper-owned worker, independent of the socket readers, report publication and mailbox
delivery, samples the adapter's snapshot at most four times a second and replaces one file of
at most 16 KiB, `observations/<name-and-epoch-digest>.json` in the room. There is no event
backlog or monitoring process; a storage failure loses that sample and is retried on the next
without failing delivery. Shutdown waits at most 500 ms for this worker.

Visibility, sender-state and identity checks are registry reads that take no cleanup or name
lock while main's mailbox is held. Inbox and direct-answer lookups use the message sender's
exact epoch, never a reused name's new run. A stopped sender, a live registry epoch that no
longer matches, or a publication heartbeat older than two seconds marks the cached data stale.
The heartbeat's age is read from the boot-clock reading the wrapper publishes with it
(`publishedBoot`), and from `publishedAt` only for a snapshot without one, since the wall
clock can be stepped between the publisher and the reader. The snapshot can lag native events
by a sampling interval. JSON separates publication time, last observation, the context and
settings observation times, field freshness and compaction coverage; stale values may be
shown with their labels, never as a fresh measurement.
