# Message delivery

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
   argument".
2. Look up a live session; if there's none, fail and list the live names.
3. The message: `{"id","from","fromEpoch","to","toEpoch","kind","text","createdAt"}`.
   `id` is time-sortable (nanosecond timestamp plus a random tail). `from` is
   `REWAKE_SESSION` or `shell`, and `fromEpoch` its run — both only when the
   run in `REWAKE_EPOCH` still holds the name. `kind` is `task` by default,
   `question` with `--question`, `notify` with `--notify` (see "Kinds").
   Sending settles nothing the sender owes the recipient: a message sent
   mid-turn is not the end of the turn.
4. Write `inbox/<name>/<id>.json.tmp`, rename it to `.json`.
5. Wait for `.status` up to `--wait` (5 seconds by default) and print the
   result. `delivered` means the notice went out; `read` counts as delivered.
   A question then waits for its answer (below).

### Kinds

Each kind is a file of its own in `internal/cli` (`send_task.go`,
`send_question.go`, `send_notify.go`) and a line in `sendKinds`; the flags,
refusals and help come from there.

| kind | sent with | the reader owes | send returns |
|---|---|---|---|
| `task` | plain `send` | a report at the end of its turn | once delivered |
| `question` | `--question` | the same | with the answer, up to `--wait` (600 s by default) |
| `notify` | `--notify` | nothing | once delivered |
| `finished` | successful turn end | nothing | — |
| `error` | failed turn hook only | nothing | failure, exit 1 for a waiting question |

The rule for agents, stated in the intro and the guide: a task or a question is
answered by ending the turn with the result as the final message, and stopping.
rewake delivers that message. A notify is not answered at all.

A question needs a sender session and a recipient whose role reports turns.
A silent recipient refuses `--question` before publication, with exit 2 and a
hint to use plain `send` or `--notify`.

Before publishing a question, `send` creates `answering/<question id>` in its
own mailbox. A separate heartbeat touches the mark every second throughout
delivery, answer waiting and output. A mark older than three seconds no longer
reserves anything. The wrapper links a reserved report into `unread/`, keeps
its queue entry, and checks the reservation on every tick. Reservation is not
final delivery: no delivered status or remembered outcome is recorded for it.
Ordinary `inbox` and notice counts skip reports with a fresh reservation.

The waiting send matches the exact question id in a report's `inReplyTo`.
Under the mailbox lock it prints the answer, writes a receipt to
`received/<question id>`, then removes its mark. Output failure exits 1 without
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
exits 3; after the reservation is released, any report arriving around that
last look is still queued for an ordinary `finished` notice. No atomic removal
and final peek are needed to prevent an unannounced answer from being stranded.

### One lock per mailbox

The server, `rewake inbox`, and `turn-ended` are separate processes acting on
the same messages and waiters. Every change of state happens under the
mailbox's `flock` (`inbox/<name>/.lock`): making a message readable, recording
what delivery did, reading, and reporting a turn. The call into the harness is
not under it — `codex queue` can take seconds — so a message can be read while
its notice is on the way. For that, `read` is final: whatever the harness says
afterwards, the status stays `read` and the message is not linked or announced
again. The Codex sandbox allows `flock` on files in `/tmp`.

No wait for the lock is endless: a reader holds it while it prints, and its
stdout can block for as long as nobody drains the pipe. The server waits while
its session lives and gives its last writes two seconds; `turn-ended` waits
five and leaves the rest owed; `inbox` waits ten and says the mailbox is busy.
A lock nobody can take — its file cannot be opened — stops nobody but the
readers, who say why: the server is then the only writer and carries on
without it. Sweeping old mail happens under the lock too.

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
removes the `unread/` copy and archives the message in `done/`; `pending`
stays and is retried every 2 seconds. Undelivered tasks and notifications older
than `--ttl` (30 minutes by default) get `failed: expired`. Only an answer accepted under a fresh reservation receives a new delivery
window. `retention/<report id>` records that reservation and, on release, fixes
one deadline origin. Retries never extend it; ordinary reports keep their
original TTL. This state survives server restarts. Receipts remain while any
queued, unread or archived report still refers to their questions.

Status: `{"state":"delivered|read|pending|failed","via":"socket|codex queue","detail":"...","at":"..."}`.

### The notice

A header and optional preview, the same for every harness:

```
Rewake: <sender> <kind>, <n> new message(s)
  ↳ <first line of the latest available letter>
```

`n` counts this run's unread mail, the new message included. The notice carries
the bounded first line of the latest available letter on a second line.
The author writes that line; rewake does not summarize. The full text remains
in inbox. An empty first line produces no preview. Control characters are
removed; the indent and preview fit a conservative 100-column budget, ending
with an ellipsis when truncated. Start messages and final replies with their
point on the first line.

### Claude Code adapter

Connect to `claude.socket` with a 2-second timeout, write the line
`{"type":"user","message":{"role":"user","content":<notification>},"priority":"next"}`
followed by `\n`, then close. The content is

```
<task-notification>
<task-id>rewake-<short id></task-id>
<status>completed</status>
<summary><the notice></summary>
</task-notification>
```

Claude Code picks how to draw a user message from its text, and draws this one
as `● <summary>`, with the preview on the next line. The status colors the
circle: `completed` is green and `failed` is red; the kind is named in the
summary. The short id keeps
two identical notices apart: Claude Code drops identical text from the same
sender within 30 seconds.

A successful write means `delivered`. `ENOENT` and `ECONNREFUSED` mean `pending`
as long as the harness is alive (the socket hasn't been created yet, or is being
recreated); otherwise `failed`.

### Codex adapter

1. Find the current thread: walk the process tree from `harnessPid`, collect the
   `/proc/<pid>/fd/*` links pointing at
   `<CODEX_HOME>/thread-writer-locks/<uuid>.lock`, and pick the lock with the
   latest mtime. None found: `pending: codex has not opened a thread yet`.
2. `codex queue --thread <uuid> --message <notice>` with the session's
   `CODEX_HOME`, 15-second timeout. Codex has no drawing of its own for this,
   so the notice arrives as an ordinary message, prefixed with 🟢 or 🔴 for an
   error to stand out when the conversation is scrolled.
3. Exit code 0 means `delivered`, noted with "codex checks its queue about every
   ten seconds". `no rollout found` means `pending: the codex session has no
   conversation yet; delivers after its first turn`. Anything else is `failed`
   with Codex's error text.

The thread is chosen fresh on every attempt: after `/new` or `/resume`, the
message goes to the current thread, not the one from when the session started.

### Reading (`rewake inbox`)

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

### The end of a turn

Callbacks with a nonempty `agent_id` belong to a nested agent and are ignored
before touching greeting markers, turn receipts or waits. `agent_type` alone
does not imply nesting: a root session can select an agent profile.

When a turn ends, the harness runs `rewake turn-ended` — a Stop hook in Claude
Code, the notify program in Codex — with the last reply of the turn in its
payload (`last_assistant_message` on stdin, `last-assistant-message` as the last
argument). Under the mailbox lock, for every run in `awaiting/<own epoch>/` it
leaves a `finished` message whose text is that reply, and whose `inReplyTo`
lists the messages read from that run since its last report, addressed to that run —
not to whoever holds the name now — and forgets the waiter only once the message
is written, and only if the waiter still names that run and that wait. A waiter
whose run has ended is forgotten without a message.

The report's id is derived from the wait — the reporting run, the waiting run,
and when the wait began — and a report whose id is already in the recipient's
mailbox, in any stage, is not written again. A waiter that could not be removed,
or a hook that died between writing and forgetting, therefore costs nothing at
the next turn. A read retried after its last step failed does not record the
wait a second time: the `read` status, written after the wait, says it is a
retry. A payload that does not
arrive within three seconds is treated as no payload. Their wrappers announce it:
`Rewake: cx finished, 1 new message`.

The hook only records; waking is the recipient wrapper's job, through the same
notice as any message. A hook cannot wake anything out of deep idle, and it
runs while its own session is still awake anyway.

Two rules keep this from turning into a loop:

- reading finished, error or notify asks for nothing back;
- each waiting run is told once; a new run of the name starts with no waiters.

So an exchange ends: A writes to B; B reads, answers, ends its turn and reports
to A; A reads both, ends its turn and reports to B; B reads the report, which
asks for nothing, and the exchange is over. The cost is that last wake of B. An
earlier version spared it by treating any message to a waiting run as the
answer, and lost the report whenever that message was a new request or a note
sent mid-turn.

`turn-ended` is hidden from the guide and never fails loudly: it runs inside the
harness's own machinery, where an error is noise at best.


### Reports after a thread change

Some harnesses can change conversations inside one wrapper run. Before an owed
message is made readable, the wrapper records the selected thread in
`inbox/<name>/threads/<message id>` under the mailbox lock. The queue uses that
same thread. The context survives reads and status rewrites, and is retained
while the message is queued, unread or included in an unsettled wait.

At the end of a turn the hook resolves the current thread using the harness's
thread tracker. If any known delivery thread in the wait's message ids differs,
the finished report carries `threadChanged: true`. Otherwise the field is
omitted, including when either identity is unavailable or the harness has no
thread tracker. This is an advisory comparison, not a change to report routing
or the wrapper epoch. A new conversation does not clear waits or resend tasks.

`rewake inbox` prints this line beneath a marked report:

```
the reader's thread changed after delivery; the report may not answer it, resend the message
```

A waiting `send --question` also prints the warning and preserves the boolean in
its JSON result. The caller decides whether to resend. Existing idempotent
report publication still applies.

### Failed turns

The hidden hook emits error with the harness reason unchanged. Received empty
completion after read work produces an error with empty text. Successful turns
still produce finished. Error is in the kind catalog but has no send flag;
`send --error` is refused. It creates no reply obligation.

Errors go to all live waiters for this run; with none, they go to the room's
main. A failing main, or a room with no main, retains the error in the failing
session's own unread mailbox without announcing it to itself. Identified turns
are idempotent; callbacks without turn ids are separate invocations. Delivery
uses failed task-notification status on the socket path and a red circle on
the queued path. A waiting question consumes the error and exits 1.

Failure observation depends on the harness providing a callback. The current
legacy notify schema cannot expose every failure; see the research boundary.
