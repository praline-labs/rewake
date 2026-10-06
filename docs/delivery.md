# Message delivery

Owned native announcements use the [mailbox output](native-mailbox.md);
actual inbox reads still establish reporting obligations. A separate
[transient display-only row](native-mailbox-ui.md) follows a successful ACK on the
primary TUI; losing that row never retries mail or changes its delivered status.

[Back to the design](design.md).

Every path and lookup below is relative to the current room's state directory,
`<REWAKE_DIR>/rooms/<REWAKE_ROOM>/` (`default` when the room variable is absent).
Send, read, identity and turn reports never fall back to another room or to
legacy records directly under the state root.

## Delivery

A harness is never handed the text of a message. It is told that mail is
waiting, and the agent fetches the text with `rewake inbox`. Two reasons, both
the owner's: the agent should know the message came through a tool rather than
from the person at the keyboard, and the person watching the session should see
a short notice and preview rather than the full body.

### Sender (`rewake send <name> <text>`)

1. Parse arguments: exactly one name and one text (`-` reads the text from
   stdin). Extra positional arguments are the error "Quote the text as one
   argument". `-h` is `--help`, before the command word as after it and checked the
   same way, and any other short flag before `--` is refused rather
   than sent as the text; a text beginning with a dash goes after `--`. The kind and
   `--wait` are checked next, before any session is looked up, so a wrong flag is named
   as such.
2. Look up a live session; if there's none, fail and list the live names. A session
   naming itself is refused with exit 2: a task to itself would owe a report to the
   turn that sent it, and a question would wait for an answer only its own blocked turn
   could give. `rewake edit` refuses to replace such a message, which only an earlier
   build could have written, and names `rewake withdraw` for it.
3. The message: `{"id","from","fromEpoch","to","toEpoch","kind","text","createdAt","createdBoot"}`.
   `createdBoot` is the boot clock's reading (`internal/boottime`), stamped where the
   letter is put into the mailbox; the coalescing window counts from it
   ([the notice](#the-notice)). It is for the processes of this machine and stays on
   disk: `inbox --json` leaves it out.
   `id` is time-sortable (nanosecond timestamp plus a random tail). `from` is
   `REWAKE_SESSION` or `shell`, and `fromEpoch` its run — both only when the
   run in `REWAKE_EPOCH` still holds the name. `kind` is `task` by default,
   `question` with `--question`, `notify` with `--notify` (see "Kinds").
   Sending settles nothing the sender owes the recipient: a message sent
   mid-turn is not the end of the turn.
   A task or a question from main may carry `grantGit` ([git-grants.md](git-grants.md))
   and `grantDirs`, the resolved directories of `--grant-dir` and `--grant-dir-broad`,
   with `grantBroad` naming those confirmed as broad; the directories are checked
   before the file is written ([grants.md](grants.md#sending)). A message carrying a
   grant is registered with the sender's own wrapper before it is written, and refused
   with exit 1 when the wrapper does not take it; the recipient's wrapper confirms it
   there at delivery ([grants.md](grants.md#who-can-grant)).
4. Write `inbox/<name>/<id>.json.tmp`, rename it to `.json`. A `notify` is journaled
   first and written once under the recipient's lock, so a repeat or a retry cannot
   publish it twice ([mail-bridge-cli.md](mail-bridge-cli.md#receipts)).
5. Wait for `.status` up to `--wait` (5 seconds by default; a question gives its
   delivery 5 seconds of its `--wait` at most, the rest being its answer's) and print the
   result. The wait ends early once the recipient run is proven ended or its name taken
   by another run, checked every half second; a run in another pid namespace cannot be
   judged and is waited for. Then print the result, then `id <id>` on a line of its own unless the send failed: the id is what
   `withdraw`, `edit` and `--to` take ([delivery-sent.md](delivery-sent.md)).
   `delivered` means the notice went out; `read` counts as delivered.
   `held` is not an answer yet and the wait goes on through it: a release or an
   expiry usually follows within the wait. A message still `held` when the wait
   ends prints `Rewake: held for <name>: <why>` and exits 3, like `pending` — accepted,
   not delivered. A question prints `id <id>` first and then waits for its answer
   (below), so the id is at hand for an edit or an addendum while it waits; it stops
   waiting, with exit 1, if its own status turns `failed` meanwhile: a hold that
   expired or a session that ended means no answer is coming.

### Kinds

Each kind is a file of its own in `internal/cli` (`send_task.go`,
`send_question.go`, `send_notify.go`) and a line in `sendKinds`; the flags,
refusals and help come from there.

| kind | sent with | the reader owes | send returns |
|---|---|---|---|
| `task` | plain `send` | a report at the end of its turn | once delivered |
| `question` | `--question` | the same | with the answer, up to `--wait` (600 s by default) |
| `notify` | `--notify`; also main's wrapper for availability, departure and compaction notices, and the letter that ends a compaction main asked for with `rewake compact` ([remote-control.md](remote-control-letter.md)) | nothing | once delivered, after waiting up to four seconds for company ([the notice](#the-notice)) |
| `finished` | successful turn end | nothing | — |
| `error` | failed turn hook only | nothing | failure, exit 1 for a waiting question |
| `stopped` | a turn stopped at the keyboard, or by a main's `rewake interrupt`; on Codex also a run that passed unseen, or a turn that ended with no proof of work | nothing | exit 1 for a waiting question |
| `pending` | a normal turn end marked with `rewake pending` only | nothing | — a waiting question keeps waiting |

The rule for agents, stated in the intro and the guide: a task or a question is
answered by ending the turn with the result as the final message, and stopping.
rewake delivers that message. A notify is not answered at all.

A question needs a sender session and a recipient whose role reports turns.
A silent recipient refuses `--question` before publication, with exit 2 and a
hint to use plain `send` or `--notify`.

Before publishing a question, `send` creates `answering/<question id>` in its
own mailbox. A separate heartbeat writes the boot clock's reading into the mark every
second throughout delivery, answer waiting and output, and never creates it again once
it is gone. A mark whose reading is three seconds old no longer reserves anything; one
without a readable reading, from an earlier build, is judged by its modification time.
The reading and not the file's time since September 26, 2026: the time is the wall
clock, which can be stepped by seconds, and a step between a touch and the wrapper's look
made a live send look gone, so its answer was announced to the agent as well. The wrapper links a reserved report into `unread/`, keeps
its queue entry, and checks the reservation on every tick. Reservation is not
final delivery: no delivered status or remembered outcome is recorded for it.
Ordinary `inbox` and notice counts skip reports with a fresh reservation.

The waiting send matches the exact question id in a report's `inReplyTo`.
Under the mailbox lock it prints the answer, writes a receipt to
`received/<question id>` containing this report's id, then removes its mark. Output failure exits 1 without
a receipt or a read status; cleanup releases the mark, leaving the answer for
ordinary delivery. The same release runs after a failed send or a timeout. A
killed process leaves its mark to expire and the queued report is announced.

A report may answer several questions. It moves to `done/` only after every id
has a receipt and no fresh reservation remains. An abandoned question has no
receipt, so another question's successful output cannot consume its answer.
Ids belonging to plain tasks also have no receipt: a mixed report remains for
ordinary delivery once all active questions finish. Receipts and stale marks
are swept by age with the other mailbox records.

When the wait ends, send looks once more under the lock. Without an answer it
prints `Rewake: no answer from <name> yet; it arrives later as its report` and exits
3; the `--json` detail keeps its longer wording. After the reservation is released,
any report arriving around that last look is still queued for an ordinary `finished`
notice. No atomic removal
and final peek are needed to prevent an unannounced answer from being stranded.

### One lock per mailbox

The server, `rewake inbox`, and `turn-ended` are separate processes acting on
the same messages and waiters. Every change of state happens under the
mailbox's `flock` (`inbox/<name>/.lock`): making a message readable, recording
what delivery did, reading, and reporting a turn. The call into the harness is
not under it — an RPC can take seconds — so a message can be read while
its notice is on the way. For that, `read` is final: whatever the harness says
afterwards, the status stays `read` and the message is not linked or announced
again. A withdrawal is final the same way: `failed` with `withdrawn: true` stays
what it is whatever the notice's outcome turns out to be
([delivery-sent.md](delivery-sent.md#withdraw-rewake-withdraw-id)). A letter the mail
tool has started showing in parts is recorded `delivered` whatever the notice's outcome,
and stays in `unread/` until it is read
([mail-bridge-cli.md](mail-bridge-cli.md#reads-in-parts)). The Codex sandbox
allows `flock` on files in `/tmp`.

No wait for the lock is endless: a reader holds it while it prints, and its
stdout can block for as long as nobody drains the pipe. The server waits while
its session lives and gives its last writes two seconds; `turn-ended` waits
five and leaves the rest owed; `inbox` waits ten and says the mailbox is busy.
A lock the server cannot take — its file cannot be opened — stops the readers,
who say why, and not the server's statuses: it writes them without the lock, along
with the links that make letters readable. That an open fails does not prove nobody
holds the lock, since a descriptor opened before the file's mode changed keeps it, so
nothing that moves or removes a letter runs without it: settling waits for a pass that
holds the lock, and so does taking over an earlier run's waits. What a settling still
owes is read from the mailbox, not remembered: a status that records an outcome beside a
copy its outcome takes away. A waiting copy is found by every pass, one for an earlier
session included; a late refusal whose readable copy is the only one left is found by
its failed status beside a letter in `unread/` with no waiting copy, at a run's start and
on every pass. Such a letter is settled once the lock can be taken, an open stop naming
it is resolved, or its archive can be written, and a restart loses none of it; its
sender is not told again. Only a refusal owes the archive: a letter held, delivered,
made readable as a report, withdrawn or being read in parts stays in `unread/`, and a
status that does not read decides nothing. The status a settling goes by is the one read
under the lock that moves the letter: a withdrawal, a read or a claim that came while the
lock was waited for decides, not what was read before it. Sweeping old mail happens under
the lock too.

### Servicing process (the wrapper)

Watches the inbox: a message arrives as a rename into the directory and the
kernel reports it, so delivery does not wait for a tick. A one-second poll runs
alongside as the safety net — it retries pending messages and covers a watch the
kernel would not give — and every ten minutes answered and unread mail older
than a day is swept away. Messages are processed in `id` order. For each one:
hard-link it into `unread/` — readable before it is announced, because an agent
told about it may run `rewake inbox` at once — count the unread mail, call the
adapter with the notice, write the status (`.status.tmp`, then rename).
`delivered` removes the waiting copy, leaving the one in `unread/`; `failed`
normally removes the `unread/` copy and archives the message in `done/`;
`pending` stays and is retried every 2 seconds. Neither removes a letter's last
copy: a copy goes only beside another that stands, a regular file (a directory or a
symbolic link at its path is none), so an archive into `done/` that
fails leaves the readable copy where it is for the next pass. The letter may be the
only proof a publication landed, and retiring that proof is the sweep's alone
([stage3-publication.md](v2/stage3-publication.md#the-contract)). An accepted finished/error/stopped
report whose notification fails keeps its readable copy and failed diagnostic,
with `reportAvailable: true` in its status. Only its waiting copy is removed;
there is no automatic notice retry. Durable recovery respects this flag, and a
concurrent read takes precedence. Expired reports never gain readability through
this exception; task/notify failure behavior is unchanged. Undelivered tasks and notifications older
than `--ttl` (30 minutes by default) get `failed: expired`.

**Held.** An adapter may report that the harness took the notice and keeps it from
the agent — Claude Code's inbound gate does (below). A held message is not delivered:
both copies stay, the waiting one and the readable one, the status reads `held` with
the harness's reason, and it is not retried. It ends when the adapter passes on how
the hold ended, as a receipt the server reads alongside the mailbox: `delivered`
settles it as delivered; `failed` settles it as failed and, for a task or question,
writes its sender a note (kind `notify`, with an `undelivered` field naming the
message, its kind and why). Its text is `Rewake: your <kind> to <name> was not
delivered: <why>. Send it again if it still matters.`, then what was sent; the rest —
the agent never saw it, it is owed no report, nothing sends it again — follows from
the kind and is in the guide, not in the note. Nothing is replayed automatically. A
session that ends with messages still held fails them the same way. The agent may still read a held message
on its own — it is readable, and nothing hides it — and that read is final like any
other: a later expiry changes nothing and tells nobody.

**The first notice.** An adapter may say its harness cannot take a notice yet. Until
it can, waiting mail stays `pending` with the detail "the session is still starting",
is not made readable, and goes out on the next pass once the harness opens. The
Claude Code adapter opens on the session's first status line, or after three seconds
if none comes.

**A reservation that cannot be taken in three seconds fails the message, it does not
hold it.** September 22, 2026: the reservation is bounded at three seconds twice, by
the delivery context in `internal/inbox/reservation.go` and again by `Reserve` in
`internal/harness/codex/server_delivery.go`. On a timeout, `Reserve` in
`internal/harness/codex/gateway/reservation.go` returns a plain context error; `refuse`
in `internal/inbox/reservation.go` wraps it into `ErrThreadUnavailable`, declared in
`internal/inbox/thread.go`; and `internal/inbox/batch.go` turns that into `failed`
rather than `pending`. So a wrapper
that waited for the thread to go idle before delivering would not deliver late — it
would not deliver at all, and the sender would be told the delivery failed. That is
why the choice between an active thread and an idle one is left to the native
start-or-steer call rather than taken from a status snapshot.

The one exception is a compaction (September 24, 2026): a Codex conversation being
compacted, by main's `rewake compact` or the terminal's `/compact`, takes no work, and
the gateway's `Reserve` then returns `gateway.ErrCompacting`, which `Reserve` in
`internal/harness/codex/server_delivery.go` passes on as `inbox.ErrNotYet`. `refuse`
lets that one through unwrapped, so the message stays `pending` and goes on a later
pass, once the compaction has ended, the wrapper's wait for its end has, or its mark's bound
has passed — 80 seconds for a compaction whose turn is not seen, 10 minutes for one
seen running ([remote-control-codex.md](remote-control-codex.md)). A delivery the server
itself refuses for a compaction running (`ActiveTurnNotSteerable { turn_kind: Compact }`,
September 26, 2026) is `gateway.ErrCompacting` too, and `DeliverChecked` makes it
`pending`: the message goes once the compaction has ended. One more hold keeps a Codex
message `pending` rather than fail it (September 29, 2026): a conversation the launch did
not ask for, until it is resumed or the person accepts another
([delivery-conversation.md](delivery-conversation.md)). Nothing else holds a delivery: an
operation whose end the gateway has not read makes main's compaction refuse, not a
delivery wait.

Only an answer accepted under a fresh reservation receives a new delivery
window. `retention/<report id>` records that reservation and, on release, fixes
one deadline origin. Retries never extend it; ordinary reports keep their
original TTL. This state survives server restarts. Receipts remain while any
queued, unread or archived report still refers to their questions.

Status: `{"state":"delivered|read|pending|held|failed","via":"socket|app-server","detail":"...","at":"..."}`,
plus optional `reportAvailable: true` for retained notification failures.
Shutdown preserves unexpired reports for the addressed epoch, including those
not yet announced. A reserved report stays queued for its waiting send; ordinary
inbox still skips it until release. Retention and exact report receipts apply
unchanged. Previously archived failed reports are not automatically recovered.

### The notice

A single newly announced message keeps its own sender/kind and bounded first line:

```
Rewake: <sender> <kind>, 1 new message
  ↳ <first line>
```

Each notice has fixed members. At the next ready delivery opportunity, accumulated
eligible mail gets its own notice through native start-or-steer: active work accepts
steering, idle work starts. No peek, terminal event or model-seen acknowledgement
is required. Initial collection is 150 ms; old unread alone produces no reminder.
Arrivals after native dispatch belong to a later immutable notice.

**Mail that asks for nothing waits for company.** September 25, 2026, the owner's
request: a burst of letters seconds apart — three workers restarted gave main seven
notices, a departure and an availability each, a few seconds apart — wakes the
recipient once. After the first collection, a pass whose mail is all of kinds that ask
for nothing holds it: announced three seconds after the latest arrival, and never later
than four seconds after the earliest was written, so a steady stream cannot put it off.
The cap counts from the write, or from when the server first saw the letter if that is
earlier, not from the sight alone: the pass that sees a letter comes up to a collection
after it, and a sender's wait begins at the write. The write is placed by the letter's
boot clock reading (`createdBoot`), since September 26, 2026: `createdAt` is the
writer's wall clock, and a step of that clock by seconds between the write and the look
cut the cap short and split the burst. A letter without the reading, from an earlier
build, or with a reading ahead of the clock, as after a reboot, falls back to `createdAt`. Mail that lay in the mailbox before
the wrapper started has used its cap and goes on the first pass. The owner
asked for about five; four keeps the whole wait, with the first collection and a
harness taking the notice, inside a sender's default five-second wait for its status,
so a lone `--notify` from an agent still returns delivered rather than pending. Whatever
arrives meanwhile goes in the same notice.

What may wait is decided by kind, in `canWait` (`internal/inbox/window.go`), from
`AsksForWork`: `notify` and the four reports (`finished`, `error`, `stopped`,
`pending`) wait. A `task`, a `question` and a kind this build does not know are work —
the same reading `Owed` gives an unknown kind — and go at once, taking any waiting mail
along in their notice: a waiting worker is not slowed, and a task arriving behind waiting
notes cuts their wait short. A report a waiting `send --question` reserved as its answer
does not wait either: it is linked for that send, not announced, and holding it would
hold the send. Nor does the note a withdrawal sends when the withdrawn message's notice
may have gone out (`recall`, [delivery-sent.md](delivery-sent.md)): it is there to stop
work a preview began, and the window would be time spent on that work. Nor does an edit's
replacement (`replaces`), a notify's included: it sends no recall, so its own preview is
what sets the old notice aside.

The window lives in the recipient's wrapper, in the mailbox server both harnesses share:
the Claude Code socket line and the Codex gateway call are made after it, so both get
the same grouping. The server learns an arrival the first time a pass sees the message,
and the quiet counts from that sight; a retry, or mail that waited for the session to open, keeps its first arrival and is
not held again. While a letter waits, its status is `pending` with the detail `waiting
up to 4s to be announced together with other mail that arrives meanwhile`, so a sender
whose own wait ends first prints why. A wrapper that exits in the window changes nothing
about the mail: reports stay readable for their epoch and the rest is refused as for any
undelivered message on shutdown. The mailbox server a test builds has no window unless
it sets `Window`; the wrapper sets `inbox.Coalescing`. A build may shorten that window
through `-ldflags -X` on `builtQuiet` and `builtCap`, and the workflow suite's does
([testing-pool.md](testing-pool.md#waits-the-suite-shortens)); a release build sets neither.
A multi-message signal says `Rewake: <n> new messages`, followed by one indented
first-line preview of its latest member with sender and kind. A correcting letter — a
recall, the note that tells the recipient not to act on a withdrawn message, or an
edit's replacement, which previews as `Replaces <short id> (withdrawn): <first line>` —
is never that latest member: each one in the notice gets an indented line of its own,
before the preview, oldest first, and a single one is shown the same way when its notice
previews a newer unread letter ([delivery-sent.md](delivery-sent.md)). Usage instructions
remain in guide/help and briefing, outside the notification itself. Old accepted
unread messages are not announced again. The [grouped inbox contract](inbox-groups.md)
describes readiness, fixed membership, destination reservation and native ACK limits.
Every stored message, receipt and obligation remains independent.

Peek text/JSON contain only metadata and bounded previews, with the existing main
state visibility. They create no task read receipts, waiters or dispatch acknowledgements. Selected/read-all output
still precedes marking; active question-answer reservations remain excluded.
Previews remove controls and stay within the conservative 100-column notice budget
including indentation. Rewake does not summarize or include later body lines.

### Adapters

How a notice reaches each harness — the Claude Code socket line with its reply socket
and receipts, and the Codex gateway — is in [delivery-adapters.md](delivery-adapters.md).

### Reading (`rewake inbox`)

A verified main caller receives a [primary session-state header](session-state.md)
before every message, and before direct question answers. Latest snapshots are
looked up by the sender's exact epoch; JSON carries the same optional metadata.
Workers and plain shells receive no telemetry. Message text and receipt identity
are unchanged.

Run by the agent inside its session: `REWAKE_SESSION` names the mailbox and
`REWAKE_EPOCH` the run. A run that no longer holds the name is refused, and so
is a process with a name and no run — by its name alone, a leftover of an ended
session cannot be told from the current one. Mail for an earlier run is not
shown. Reports reserved for a waiting send are also skipped.

Under the mailbox lock, the text is printed first. Only once the output got
through is each message recorded: its sender run in `awaiting/<own epoch>/`
(unless it is a `finished` notice or has no `fromEpoch`), then the `read`
status, then the move to `done/`, which is the commit. A failure at any step
leaves the message unread, and the next `inbox` shows and records it again.
Two readers at once are serialized: the second finds nothing new.

Under the mail tool the read goes in parts instead, and a letter is read only once the
wrapper confirms every part reached the model; `inbox --next` and `rewake retry`
continue or show again a read or a long output from either channel
([mail-bridge-cli.md](mail-bridge-cli.md#reads-in-parts)).

A task that carries a directory grant is printed with one `grant: write <dir>` line per
directory between its heading and its text, and `inbox --owed` repeats them while the
task is owed: the roots changed without a word in the terminal, so the reader learns it
here ([grants.md](grants.md#delivery)). The `--owed` lines add that a turn typed in the
terminal may have dropped the grant since, or, once the journal says so, that it was
revoked or dropped ([grants.md](grants.md#taking-a-grant-back)).

### A resumed conversation

How a cold resume takes over what an earlier run of the name read and had not reported
on: [delivery-conversation.md](delivery-conversation.md#a-resumed-conversation).

### A conversation the launch did not ask for

A Codex launch that resumes delivers only into the conversation it asked for until that
one is resumed or the person runs `rewake accept`, and a worker there does not read its
mail before. How, and who is told: [delivery-conversation.md](delivery-conversation.md).

### Owed reports

What a session has read and still owes a report on, shown again with `rewake inbox
--owed`, and the sender's side — what a run sent and still waits on, with `rewake inbox
--awaited` — are in [delivery-owed.md](delivery-owed.md).

### Actions on a sent message

Withdrawing a message nobody has read, replacing its text, and adding to a task
under way — `rewake withdraw`, `rewake edit`, `rewake send --to` — are in
[delivery-sent.md](delivery-sent.md).

### The end of a turn, and after

How `turn-ended` turns a finished turn into a report, how a report notes that the
reader's conversation changed underneath it, and where the turn ends that are not an
ordinary report are described: in [delivery-turn-end.md](delivery-turn-end.md).
