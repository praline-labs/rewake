# Delivery into the conversation the launch asked for

A Codex delivery goes into the conversation the terminal has selected
([gateway.md](gateway.md#selection-and-reservation)). This document is how the wrapper
keeps a message pending while that conversation is not the one the launch asked for. It
comes from a defect seen live on September 29, 2026
([work-queue.md](work-queue.md#now-after-100)) and from the owner's decisions of that day.
Claude Code sessions hold nothing here: their adapter does not select a conversation.

## When the selected conversation is not the launch's

A launch that resumes — `rewake codex resume <id>`, `resume --last`, `resume` with the
picker or a name — asks for one conversation. The terminal can still end up in another:
its resume is refused because another program holds the conversation's writer lock, it
starts fresh because `--last` found nothing, or the person picks "start fresh" in the
picker ([research-codex.md](research-codex.md#a-conversation-another-program-holds)).
Before this change the gateway forgot what the launch asked for: a refused resume let the
binding go, the next recognized `thread/start` bound the new conversation, and the task
went there.

Now the launch passes its intent to the gateway (`gateway.LaunchIntent`, read from the
launch's arguments by `resumeIntent` in `internal/harness/codex/continuation.go`):

- `resume <uuid>` names the conversation at once;
- `resume --last`, `resume <name>` and the bare picker name none, and the terminal's
  first primary `thread/resume` names it, before its reply — so a resume that is refused
  still says what was asked for (`pin` in `internal/harness/codex/gateway/intent.go`);
- a fresh launch and a fork carry no intent and behave as before: a fork's delivery goes
  to the child of the confirmed parent ([gateway.md](gateway.md#fork-and-side-are-different-workflows)).

Until the first successful resume of the intended conversation, a reservation of any
other one is refused with `gateway.ErrUnintended`, carrying the hold. So is a reservation
while nothing is selected after the terminal's resume of it selected nothing — refused
for another writer, or opened read-only, which is what the terminal's source does on that
refusal: the hold holds with no conversation selected, and outlives the connection,
since the intent is the gateway's and not a connection's. A resume only on its way is no
hold to publish, but a delivery that outwaits it stays pending too. The Codex adapter
passes it on as `inbox.ErrNotYet`, so the message stays `pending` — never `failed` —
and it is refused before anything is linked unread or any delivery thread recorded: no
task becomes readable in a conversation nobody chose. The sender's `rewake send` exits 3
with a detail naming the expected and the selected conversation and what to do:

```text
Rewake: pending for worker: the message could not be made readable yet: the
conversation cannot take a message yet: the launch asked to resume <X>, and the terminal
selected <Y>; deliveries wait: resume <X> with /resume, or have the person accept <Y> from
a shell outside the session: rewake accept worker <Y>
```

`/new` does not lift the hold — it only selects another conversation that was not
asked for. What lifts it:

- **the intended conversation resumed** — `/resume` of it, or the terminal's own retry
  once the other writer lets go; the first ready binding of it ends the wait for good;
- **the person's acceptance**: `rewake accept <session> <conversation>`, run from a shell
  outside any rewake session. It goes through the session's control directory, which
  belongs to the current run, and the wrapper takes it only when the conversation named
  is the one selected now: an acceptance of a conversation seen before a switch, or in an
  earlier run, is refused. The check and the lift are one step under the locks every
  change of the selection takes, so the selection cannot move between them. An agent
  cannot run it — inside a session it is a wrong call, exit 2.

After either, the terminal navigates as it always did: a later `/new` or `/resume` moves
delivery with it, as for a fresh launch.

A worker already running in the unintended conversation does not read its mail there:
`rewake inbox` (and `--peek`, `--owed`) refuses, exit 1, and names what it waits for.
The published snapshot cannot decide that — it is written on a tick and reads as "no hold"
when missing — so the wrapper keeps its own record (`sessionstate.HoldMail`): a launch
that resumes writes it before the terminal starts, and the gateway rewrites it before
the hold ends, by the intended resume or the acceptance; a lift it cannot record stays a
hold and is tried again at the next publication. Until the record is written, the intended
conversation selected takes no delivery either: a lift that could not be written is the
hold `mail-closed`, naming the error, published and told like any other, and each
publication tries the write again. The record changes once, from held to admitted, and
the inbox reads it under the mailbox lock; one it cannot read refuses too.
A run with no record never asked to resume. Mail already waiting is read in another
conversation only after the person has accepted it. `--awaited`, which shows what others
owe the session, stays open.

## When the worker could not read its mail

The same incident had a second cause this document does not settle: the conversation's
sandbox left rewake's state directory read-only, so the worker could not run `rewake inbox`
and stood still, seeing only the notices. A check before each delivery that inferred the
sandbox from Codex's event stream was built with this change and taken out on September
30, 2026: three acceptance rounds did not converge, since the protocol does not state
which settings change applied or which permissions a running turn keeps. The owner chose
instead to make the mail independent of the sandbox, the wrapper doing the writes outside
it; that is the next change ([work-queue.md](work-queue.md#now-after-100), the bullet
"Mail that does not depend on the sandbox"). Until it lands, a notice goes into the
selected conversation whatever its sandbox allows. The source facts read for the check
stay true and are kept in [research-codex-conversation.md](research-codex-conversation.md).

## Who is told

- **The sender** reads the reason in its `pending` status and `rewake send`'s exit 3.
- **main** gets one notice from the worker's name when a hold appears and one each time
  its reason, its conversations or its cause change — `Rewake: deliveries to <worker> wait: …`
  (`internal/wrap/hold_notices.go`); a hold that ends says nothing, since the mail simply
  goes. `rewake list` shows a line per held session under its table, and `--json` the
  `deliveryHold` field of its telemetry, with `reason` (`unintended-conversation` or
  `mail-closed`), `expectedConversation`, `selectedConversation` and `detail`.
- **The person at the worker's terminal** is shown the same reason, prefixed `rewake:`,
  as the server's own `warning` notification, which the terminal draws without a turn of
  the model — once per hold on a connection, and to the intended conversation when a
  refused resume left nothing selected. That path was read in the source of 0.159.0
  and not yet seen live ([research-codex-conversation.md](research-codex-conversation.md#a-warning-shown-without-a-turn));
  when the terminal has no room for it, main's notice and `rewake list` still say the same.

## A resumed conversation

Moved here from [delivery.md](delivery.md#a-resumed-conversation) by subject on September
30, 2026: what a resumed run takes over, whichever harness it runs.

A cold resume starts a new run of the name in a conversation an earlier run worked in.
What that run read and had not reported on is in the conversation, and only the new run
can finish it. So each delivery pins the conversation it went into
(`inbox/<name>/threads/<id>`, written before the task becomes readable), and once the new
run's harness names its conversation, the wrapper takes over the earlier runs' waits for
the tasks pinned to that conversation, as if this run had read them when the earlier
one did
(`internal/inbox/adopt.go`). Only then does it sweep the earlier runs' records. Before it
takes over anything, it runs the barrier over what the earlier runs left
([delivery-turn-end.md](delivery-turn-end.md)): a turn end left unfinished is completed,
so a wait whose report is out is cleared, not taken over and answered a second time, and
an unknown stops the adoption with the mailbox. The
report at the new run's next turn end settles those tasks for their senders; a task
delivered into another conversation, or never pinned, is not taken over, and its sender
reads that no report is coming. A task is taken over only within a day of being read
(`resumeWindow`, counted per task from the read time its wait record keeps, since one wait
gathers what is read until the next report); until a resume or that day, the sender reads
that a resume may still report, and is told not to send the task again
([delivery-owed.md](delivery-owed.md#what-others-owe-you-rewake-inbox---awaited)).

The conversation is the link, not the name: a new conversation under the same name owes
nothing, and a fork (`--fork-session`) starts one. A harness that names no conversation
sweeps at once. The grants of those tasks are confirmed again by main
([grants-resume.md](grants-resume.md)).
