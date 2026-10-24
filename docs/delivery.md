---
description: How mail reaches a session: sending, the kinds of message, the mailbox lock, the wrapper's servicing, the notice and its preview, the coalescing window, and reading.
---

# Message delivery

How a message is sent, kept, announced and read. Every path and lookup below is relative to
the current room's state directory, `<REWAKE_DIR>/v2/rooms/<REWAKE_ROOM>/` (a command outside
any session takes the room of its working directory when the variable is absent). Send, read,
identity and turn reports never fall back to another room or to 1.x's records beside the
root. How a notice reaches each harness is [delivery-adapters.md](delivery-adapters.md); what
happens at a reader's turn end is [delivery-turn-end.md](delivery-turn-end.md).

## Delivery

A harness is never handed the text of a message. It is told that mail is waiting, and the
agent fetches the text with `rewake inbox`. Two reasons, both the owner's: the agent should
know the message came through a tool rather than from the person at the keyboard, and the
person watching the session should see a short notice and preview rather than the full body.

### Sender (`rewake send <name> <text>`)

1. Parse arguments: exactly one name and one text (`-` reads the text from stdin). Extra
   positional arguments are the error "Quote the text as one argument". `-h` is `--help`,
   before the command word as after it and checked the same way, and any other short flag
   before `--` is refused rather than sent as the text; a text beginning with a dash goes
   after `--`. The kind and `--wait` are checked next, before any session is looked up, so a
   wrong flag is named as such.
2. Look up a live session; if there's none, fail and list the live names. A session naming
   itself is refused with exit 2: a task to itself would owe a report to the turn that sent
   it, and a question would wait for an answer only its own blocked turn could give.
3. The message: `{"id","from","fromEpoch","to","toEpoch","kind","text","createdAt","createdBoot"}`.
   `createdBoot` is the boot clock's reading (`internal/infra/boottime`), stamped where the
   letter is put into the mailbox; the coalescing window counts from it
   ([the notice](#the-notice)). It is for the processes of this machine and stays on disk:
   `inbox --json` leaves it out. `id` is time-sortable (nanosecond timestamp plus a random
   tail). `from` is `REWAKE_SESSION` or `shell`, and `fromEpoch` its run — both only when the
   run in `REWAKE_EPOCH` still holds the name. `kind` is `task` by default, `question` with
   `--question`, `notify` with `--notify` (see "Kinds"). Sending settles nothing the sender
   owes the recipient: a message sent mid-turn is not the end of the turn.
4. Write `inbox/<name>/<id>.json.tmp`, rename it to `.json`. A `notify` from a session is
   journaled first and written once under the recipient's lock, so a repeat or a retry cannot
   publish it twice ([rules/effects.md](rules/effects.md#receipts-retries-and-once-marks)).
5. Wait for `.status` up to `--wait` (5 seconds by default; a question gives its delivery 5
   seconds of its `--wait` at most, the rest being its answer's) and print the result. The
   wait ends early once the recipient run is proven ended or its name taken by another run,
   checked every half second; a run in another pid namespace cannot be judged and is waited
   for. Then print the result, then `id <id>` on a line of its own unless the send failed:
   the id is what `withdraw`, `edit` and `--to` take ([delivery-sent.md](delivery-sent.md)).
   `delivered` means the notice went out; `read` counts as delivered. `held` is not an answer
   yet and the wait goes on through it: a release or an expiry usually follows within the
   wait. A message still `held` when the wait ends prints `Rewake: held for <name>: <why>`
   and exits 3, like `pending` — accepted, not delivered. A question prints `id <id>` first
   and then waits for its answer (below), so the id is at hand for an edit or an addendum
   while it waits; it stops waiting, with exit 1, if its own status turns `failed` meanwhile:
   a hold that expired or a session that ended means no answer is coming.

### Kinds

Each kind is a file of its own in `internal/cli` (`send_task.go`, `send_question.go`,
`send_notify.go`) and a line in `sendKinds`; the flags, refusals and help come from there.

| kind | sent with | the reader owes | send returns |
|---|---|---|---|
| `task` | plain `send` | a report at the end of its turn | once delivered |
| `question` | `--question` | the same | with the answer, up to `--wait` (600 s by default) |
| `notify` | `--notify`; also main's wrapper for availability, departure and compaction notices and for the letter that ends a compaction main asked for ([remote-control.md](remote-control.md#the-letter)), and a worker's wrapper for a permission request or question main may answer with `rewake answer` ([permission-requests.md](permission-requests.md#what-main-sees)) and for a dialog it waits at that nobody is asked about ([the same](permission-requests.md#a-dialog-nobody-is-asked-about)) | nothing | once delivered, after waiting up to four seconds for company ([the notice](#the-notice)) |
| `finished` | successful turn end | nothing | — |
| `error` | a failed turn end only | nothing | failure, exit 1 for a waiting question |
| `stopped` | a turn stopped at the keyboard, or by a main's `rewake interrupt` | nothing | exit 1 for a waiting question |
| `pending` | a normal turn end marked with `rewake pending` only | nothing | — a waiting question keeps waiting |

The rule for agents, stated in the briefing and the guide: a task or a question is answered
by ending the turn with the result as the final message, and stopping. rewake delivers that
message. A notify is not answered at all.

A question needs a sender session and a recipient whose run and role both report turns. The
run is asked first, by the live set its session record carries
([adapter-api.md](rules/adapter-api.md#two-levels-offered-and-live)): a recipient whose turn
boundary is not live refuses a task and a question alike, exit 1, with nothing sent; a note
still goes. This reads the live set, never the offer: a run that has not yet proven its
boundary, or has lost it, refuses as one on a harness with none does, and the offer only
picks the words of the refusal. The set is read again once the message is in hand — reading
it from stdin lasts as long as the writer keeps the pipe open — and a boundary withdrawn
meanwhile refuses the same way; a record that cannot be read again fails the send, and a run
that ended meanwhile has nothing live. Then a silent recipient refuses `--question` before
publication, with exit 2 and a hint to use plain `send` or `--notify`.

A letter to a run whose Wake is not live is still written: `send` reports it accepted, exit
3, without waiting, and says no notice can reach the session, which reads its mail when it
runs `rewake inbox`. The recipient's wrapper keeps the letter pending rather than failing it,
so a Wake made live again carries the next attempt. It reserves nothing through that Wake and
asks it for no conversation: the letter is made readable the plain way, owed a report in no
conversation, and pinned when a later attempt finds the Wake live. A Wake withdrawn while a
reservation is asked is the same case, so the promise `send` made is never broken by a failed
letter.

The live set reaches the record through a write that can fail like any other. A failed write
is not taken for recorded: the wrapper tries once more at once, then again with a growing wait
up to a second, until the record carries the set reported last or the run ends, and says on
stderr that it is trying. Once the report has been processed, the run's own view follows the
new set. Other sessions still read the previous set until a write succeeds, so they can admit
a task after its boundary has been lost. There is no bound on this window while
session-record writes keep failing; the one-second bound is the retry interval, not a
publication deadline. A record that has become a later run's is never written; the retries
stop there.

### A question's answer

Before publishing a question, `send` creates `answering/<question id>` in its own mailbox. A
separate heartbeat writes the boot clock's reading into the mark every second throughout
delivery, answer waiting and output, and never creates it again once it is gone. A mark whose
reading is three seconds old no longer reserves anything; one without a readable reading is
judged by its modification time. The boot clock and not the file's time, because the wall
clock can be stepped by seconds, and a step between a touch and the wrapper's look would make
a live send look gone. The wrapper links a reserved report into `unread/`, keeps its queue
entry, and checks the reservation on every tick. Reservation is not final delivery: no
delivered status or remembered outcome is recorded for it. Ordinary `inbox` and notice counts
skip reports with a fresh reservation.

The waiting send matches the exact question id in a report's `inReplyTo`. Under the mailbox
lock it prints the answer, writes a receipt to `received/<question id>` containing this
report's id, then removes its mark. Output failure exits 1 without a receipt or a read
status; cleanup releases the mark, leaving the answer for ordinary delivery. The same release
runs after a failed send or a timeout. A killed process leaves its mark to expire and the
queued report is announced.

A report may answer several questions. It moves to `done/` only after every id has a receipt
and no fresh reservation remains. An abandoned question has no receipt, so another question's
successful output cannot consume its answer. Ids belonging to plain tasks also have no
receipt: a mixed report remains for ordinary delivery once all active questions finish.
Receipts and stale marks are swept by age with the other mailbox records.

When the wait ends, send looks once more under the lock. Without an answer it prints
`Rewake: no answer from <name> yet; it arrives later as its report` and exits 3; the `--json`
detail keeps its longer wording. After the reservation is released, any report arriving
around that last look is still queued for an ordinary `finished` notice.

### One lock per mailbox

The server, `rewake inbox`, and the turn end the wrapper publishes are separate actors on the
same messages and waiters. Every change of state happens under the mailbox's `flock`
(`inbox/<name>/.lock`): making a message readable, recording what delivery did, reading, and
reporting a turn. The call into the harness is not under it — it can take seconds — so a
message can be read while its notice is on the way. For that, `read` is final: whatever the
harness says afterwards, the status stays `read` and the message is not linked or announced
again. A withdrawal is final the same way: `failed` with `withdrawn: true` stays what it is
whatever the notice's outcome turns out to be
([delivery-sent.md](delivery-sent.md#withdraw-rewake-withdraw-id)).

No wait for the lock is endless: a reader holds it while it prints, and its stdout can block
for as long as nobody drains the pipe. The server waits while its session lives and gives its
last writes two seconds; a turn end waits five and leaves the rest owed; `inbox` waits ten and
says the mailbox is busy. A lock the server cannot take — its file cannot be opened — stops
the readers, who say why, and not the server's statuses: it writes them without the lock,
along with the links that make letters readable. That an open fails does not prove nobody
holds the lock, since a descriptor opened before the file's mode changed keeps it, so nothing
that moves or removes a letter runs without it: settling waits for a pass that holds the lock.
What a settling still owes is read from the mailbox, not remembered: a status that records an
outcome beside a copy its outcome takes away. A waiting copy is found by every pass, one for
an earlier session included; a late refusal whose readable copy is the only one left is found
by its failed status beside a letter in `unread/` with no waiting copy, at a run's start and
on every pass. Such a letter is settled once the lock can be taken or its archive can be
written, and a restart loses none of it; its sender is not told again. Only a refusal owes the
archive: a letter held, delivered, made readable as a report or withdrawn stays in `unread/`,
and a status that does not read decides nothing. The status a settling goes by is the one read
under the lock that moves the letter: a withdrawal or a read that came while the lock was
waited for decides, not what was read before it. Sweeping old mail happens under the lock too.

### Servicing process (the wrapper)

Watches the inbox: a message arrives as a rename into the directory and the kernel reports
it, so delivery does not wait for a tick. A one-second poll runs alongside as the safety net —
it retries pending messages and covers a watch the kernel would not give — and every ten
minutes answered and unread mail older than a day is swept away. Messages are processed in
`id` order. For each one: hard-link it into `unread/` — readable before it is announced,
because an agent told about it may run `rewake inbox` at once — count the unread mail, call
the adapter with the notice, write the status (`.status.tmp`, then rename). `delivered`
removes the waiting copy, leaving the one in `unread/`; `failed` normally removes the
`unread/` copy and archives the message in `done/`; `pending` stays and is retried every 2
seconds. Neither removes a letter's last copy: a copy goes only beside another that stands, a
regular file (a directory or a symbolic link at its path is none), so an archive into `done/`
that fails leaves the readable copy where it is for the next pass. The letter may be the only
proof a publication landed, and retiring that proof is the sweep's alone. An accepted
finished, error or stopped report whose notification fails keeps its readable copy and failed
diagnostic, with `reportAvailable: true` in its status. Only its waiting copy is removed;
there is no automatic notice retry, and a concurrent read takes precedence. Expired reports
never gain readability through this exception. Undelivered tasks and notifications older than
the fixed TTL (30 minutes; there is no public `--ttl` flag) get `failed: expired`.

**Held.** An adapter may report that the harness took the notice and keeps it from the agent —
Claude Code's inbound gate does ([delivery-adapters.md](delivery-adapters.md#claude-code)). A
held message is not delivered: both copies stay, the waiting one and the readable one, the
status reads `held` with the harness's reason, and it is not retried. It ends when the adapter
passes on how the hold ended, as a receipt the server reads alongside the mailbox: `delivered`
settles it as delivered; `failed` settles it as failed and, for a task or question, writes its
sender a note (kind `notify`, with an `undelivered` field naming the message, its kind and
why). Its text is `Rewake: your <kind> to <name> was not delivered: <why>. Send it again if it
still matters.`, then what was sent; the rest — the agent never saw it, it is owed no report,
nothing sends it again — follows from the kind and is in the guide, not in the note. Nothing
is replayed automatically. A session that ends with messages still held fails them the same
way. The agent may still read a held message on its own — it is readable, and nothing hides
it — and that read is final like any other: a later expiry changes nothing and tells nobody.

**The first notice.** An adapter may say its harness cannot take a notice yet. Until it can,
waiting mail stays `pending` with the detail "the session is still starting", is not made
readable, and goes out on the next pass once the harness opens. The Claude Code adapter opens
three seconds after its lane starts; that bound is the only gate.

**A reservation that cannot be taken in three seconds fails the message, it does not hold
it.** The reservation a Wake makes is bounded at three seconds by the delivery context in
`internal/host/delivery/prepare.go`; a refusal or a timeout is wrapped into
`ErrThreadUnavailable` (`internal/core/mail/thread.go`), and `internal/host/delivery/batch.go`
turns that into `failed` rather than `pending`. The one error let through unwrapped is
`mail.ErrNotYet`: the Wake says the session cannot take the message yet, and it stays
`pending` for a later pass.

Only an answer accepted under a fresh reservation receives a new delivery window.
`retention/<report id>` records that reservation and, on release, fixes one deadline origin.
Retries never extend it; ordinary reports keep their original TTL. This state survives server
restarts. Receipts remain while any queued, unread or archived report still refers to their
questions.

Status: `{"state":"delivered|read|pending|held|failed","via":"socket","detail":"...","at":"..."}`,
plus optional `reportAvailable: true` for retained notification failures. Shutdown preserves
unexpired reports for the addressed epoch, including those not yet announced. A reserved
report stays queued for its waiting send; ordinary inbox still skips it until release.

## The notice

A single newly announced message keeps its own sender, kind and bounded first line:

```
Rewake: <sender> <kind>, 1 new message
  ↳ <first line>
```

**A task's preview says it is one.** The preview of a `task` or a `question` — alone, or the
latest member of a group — ends with ` (a preview only: run rewake inbox)`; a note's and a
report's do not. The bound of the line counts those words and never cuts them: the author's
text is cut first, to the cells they leave (`shownPreview` in `internal/core/mail/notice.go`).
The owner's decision, after a live run in which a worker shown a short task's whole first line
did the task from the notice and never ran `rewake inbox`: the letter stayed unread, the
turn's end answered nothing, and main got no report
([research-claude-mod.md](research-claude-mod.md#a-worker-acts-on-the-notices-preview)). The
worker's playbook says the same in its working rules.

**Fixed members.** Every notice has a fixed set of member ids. At the next ready delivery
opportunity, accumulated eligible mail gets its own notice: active work takes it where its
harness allows, idle work starts. No peek, terminal event or model-seen acknowledgment is
required. The first collection is 150 ms from the first wake of the servicing pass, never
extended by later arrivals (`collectionInterval`, `internal/host/delivery/serve.go`); mail
arriving while delivery waits to be ready joins the group. Once handed to the harness, a
group's membership, count and preview are fixed, and arrivals after that form the next group.
Old announced-but-unread messages remain readable, never inflate later notices and never wake
again. Each member keeps its own id, body, kind, epochs, status, receipts and obligation, and
is checked again for reservation, expiry and readability just before the notice goes; a
member read, reserved or expired meanwhile invalidates the attempt, and a later pass rebuilds
the set. A startup pass that publishes availability notices to main does so under one lock of
main's mailbox, so they share one wake.

**Mail that asks for nothing waits for company.** A burst of letters seconds apart — workers
restarted give main a departure and an availability each — wakes the recipient once. After the
first collection, a pass whose mail is all of kinds that ask for nothing holds it: announced
three seconds after the latest arrival, and never later than four seconds after the earliest
was written, so a steady stream cannot put it off. The cap counts from the write, or from when
the server first saw the letter if that is earlier: the pass that sees a letter comes up to a
collection after it, and a sender's wait begins at the write. The write is placed by the
letter's boot-clock reading (`createdBoot`), since a step of the wall clock between the write
and the look would cut the cap short; a letter without the reading, or with one ahead of the
clock, as after a reboot, falls back to `createdAt`. Mail that lay in the mailbox before the
wrapper started has used its cap and goes on the first pass. Four seconds keeps the whole
wait, with the first collection and a harness taking the notice, inside a sender's default
five-second wait for its status, so a lone `--notify` returns delivered rather than pending.
Whatever arrives meanwhile goes in the same notice.

What may wait is decided by kind, in `canWait` (`internal/host/delivery/window.go`), from
`AsksForWork`: `notify` and the four reports (`finished`, `error`, `stopped`, `pending`) wait.
A `task`, a `question` and a kind this build does not know are work — the same reading `Owed`
gives an unknown kind — and go at once, taking any waiting mail along in their notice. A
report a waiting `send --question` reserved as its answer does not wait either: it is linked
for that send, not announced. Nor does the note a withdrawal sends when the withdrawn
message's notice may have gone out (`recall`, [delivery-sent.md](delivery-sent.md)): it is
there to stop work a preview began. Nor does an edit's replacement (`replaces`), a notify's
included: it sends no recall, so its own preview is what sets the old notice aside.

The window lives in the recipient's wrapper, in the mailbox server every harness shares: the
adapter's notice is made after it, so every harness gets the same grouping. The server learns
an arrival the first time a pass sees the message, and the quiet counts from that sight; a
retry, or mail that waited for the session to open, keeps its first arrival and is not held
again. While a letter waits, its status is `pending` with the detail `waiting up to 4s to be
announced together with other mail that arrives meanwhile`. A wrapper that exits in the window
changes nothing about the mail: reports stay readable for their epoch and the rest is refused
as for any undelivered message on shutdown. The mailbox server a test builds has no window
unless it sets `Window`; the wrapper sets `delivery.Coalescing`. A build may shorten that
window through `-ldflags -X` on `builtQuiet` and `builtCap`, and the workflow suite's does
([testing.md](testing.md#waits-the-suite-shortens)); a release build sets neither.

**A group's notice** says `Rewake: <n> new messages`, followed by one indented first-line
preview of its latest member with sender and kind. A correcting letter — a recall, or an
edit's replacement, which previews as `Replaces <short id> (withdrawn): <first line>` — is
never that latest member: each one in the notice gets an indented line of its own, before the
preview, oldest first, and a single one is shown the same way when its notice previews a newer
unread letter ([delivery-sent.md](delivery-sent.md)). Previews remove control characters and
stay within a 100-column budget including indentation; rewake does not summarize or include
later body lines. Usage instructions stay in the guide, help and briefing, outside the notice,
but for the few words that end a task's or a question's preview. The count describes the
group, not a continuously changing count of unread mail.

## Reading (`rewake inbox`)

Run by the agent inside its session: `REWAKE_SESSION` names the mailbox and `REWAKE_EPOCH` the
run. A run that no longer holds the name is refused, and so is a process with a name and no
run — by its name alone, a leftover of an ended session cannot be told from the current one.
Reports reserved for a waiting send are skipped. A verified main caller receives a
[session-state header](session-state.md) before every message, and before direct question
answers; workers and plain shells receive none. Message text and receipt identity are
unchanged.

Under the mailbox lock, the text is printed first. Only once the output got through is each
message recorded: its sender run in `awaiting/<own epoch>/` (unless it is a `finished` notice
or has no `fromEpoch`), then the `read` status, then the move to `done/`, which is the commit.
A failure at any step leaves the message unread, and the next `inbox` shows and records it
again. Two readers at once are serialized: the second finds nothing new.

Mail an earlier run of the name was told of and never read stays in `unread/`, and the next
run reads it with its own, each such letter marked `for an earlier run` in its heading and
`earlierRun` in JSON. A task or a question in it records no wait and owes no report: the
sender's `--awaited` already counts it lost to the ended run, and may have sent it again. A new
run forgets what the earlier runs of its name owed.

### Peek and one message

`rewake inbox --peek` shows the available unread mail's metadata: its id, sender, kind,
timestamp and a first-line preview bounded at 96 cells (`internal/core/mail/preview.go`), with
the notice's sanitizer. Text and JSON leave out full bodies. It records no wait, marks
nothing read, has no delivery side effect and writes no counter, so repeated peeks leave every
message available; main's state header follows the same identity check as a read. The
overview may be out of date before the next command: selecting an id no longer available
refuses rather than choosing another.

`rewake inbox --message <id>` prints one available message in its normal full view, then
marks only that message read; output failure leaves it unread. Tasks and questions create
their obligations only when actually read; notes and reports do not. Ids are copied from peek,
not parsed as timestamps or shortened addresses.

Plain `rewake inbox` reads all available unread messages oldest first. `--peek` and
`--message` are mutually exclusive. All reading modes use the current epoch and the bounded
mailbox lock. Reserved, expired, already-read, unknown and other-epoch ids cannot be selected
as available mail. `--owed` and `--awaited`, which only look and take no lock, are in
[delivery-owed.md](delivery-owed.md); withdrawing, editing and adding to a sent message are in
[delivery-sent.md](delivery-sent.md).
