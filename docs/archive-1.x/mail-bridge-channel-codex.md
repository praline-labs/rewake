# Conversations on Codex, for the mail channel

Part of [mail-bridge-channel.md](mail-bridge-channel.md): on Codex every thread starts
its own instance of our server, so the channel has to know which connections serve the
conversation the terminal holds, and how that set moves when another conversation is
selected. *Revised after the live checks of October 4, 2026; built the same day, with
the readings in [as built](#as-built).* The failure
points these rules meet are rows of
[mail-bridge-channel-failures.md](mail-bridge-channel-failures.md).

## Conversation connections

On Codex a server serves one thread for its whole life: each thread starts
its own instance, a sub-agent's among them (*live*, 0.159.0: generation 2 beside 1).
Codex names the calling thread in `_meta.threadId` of every MCP request (*source*,
`core/src/mcp_tool_call.rs` 1410–1428), so the first request over a connection that
names a thread **binds** that connection's generation to it. A binding is an event of
its own, timed when the request reached the endpoint, but what it states holds from the
connection's hello: the history is folded again with that connection's class fixed from
its hello on. A connection is, at each time *t*:

- **of the conversation** when it is bound to the thread the gateway held as primary at
  *t*, or not bound yet — the unknown case counts for the conversation, a compromise
  kept open: a sub-agent's server that never calls hides the parent's death;
- **foreign** when it is bound to any other thread. A change of the primary thread
  (`/new`, a resume) takes effect at the answer that selects another conversation; a
  connection bound to the old thread is foreign from then on, never before, so the past
  is not judged again by a later primary. While a selection is pending there is no
  primary, and what arrives is held ([below](#selecting-a-conversation)).

Wherever [the channel](mail-bridge-channel.md) says a connection lives, a hello, a
close or "no connection", it means a conversation connection: a foreign one's hello does not make the tool
connected, stop a timer or end a failure that holds while none lives, and its close
changes nothing. A failed startup status counts when its optional `threadId` names the
primary thread or is absent. So a sub-agent's call that binds its connection after the
parent's server closed shows that none of the conversation lived from that close: the
failure "server gone" is placed at the close's time and told when the binding is folded,
like any event folded late; the refusal of that call changes nothing by itself.

## Selecting a conversation

The gateway admits a
selection request — `thread/start`, a resume or a fork the terminal makes primary — at
its injection check, empties the primary when it forwards it, and sets the new primary
only on a successful answer (`gateway/gateway.go` 169–204, `gateway/state.go` 59–65,
123–128, 190–201). Each step is an event at its own time, and between the admission and
the answer the selection is **pending**:

- *The expected start* belongs to the pending selection, not to the old conversation:
  its timer opens at the admission whatever the old conversation's connections do,
  unless the request names its target and a connection bound to that thread was
  observed live at the admission (a resume of a thread already served). That needs
  only the observed connection, no claim that the harness never starts a second server
  for it: a second one is folded by its own hello and close like any other. A
  `thread/start` names no thread before its answer.
- *Held events.* From the admission until the answer, a hello or a close of a connection
  not bound to the old conversation — the target's own among them — a failed startup
  status, the timer passing and a binding are held with their own times, and the old
  conversation's display stays as it was. Its own connections' closes still count for
  it until the answer.
- *A successful answer* selecting thread B at time S: when B is the conversation
  already, the held events fold for it and its own state goes on, nothing else changes.
  Otherwise the old conversation's connections are foreign from S, an interval of it
  still open closes at S with no notice, and its working state goes with it. The next
  conversation starts from **the connections bound to B that were live at the
  admission**, taken from the shared history: with one or more it is connected, never
  working on the old conversation's proof; with none it is **starting**. Its failure
  interval is empty, and `tool last worked` is kept as a fact of the run. Then the held
  events that are B's fold at their own times — a hello of a connection unbound or
  bound to B, a close of one, a status naming B or none, the timer — and what they
  change is told at S: B connected from a hello; failing "server gone" from the close
  that leaves none of its connections live; failing "command cannot start" from its
  status; failing "no hello observed" from the timer's end. The others are dropped:
  they are another thread's, a sub-agent's among them, and change no conversation's
  channel. A hello, binding or close delivered late moves that starting set by its own
  time, like every event.
- *A refused, unknown or read-only answer*, or a lifecycle request that leaves the
  primary empty: the held events are dropped, the timer cancelled, nothing is told, and
  the display stays the old conversation's until a selection succeeds; the gateway
  already tells the terminal to select one with `/resume` or `/new`.
- *The run's first thread* is a selection with no old conversation: the display is
  "starting" until its answer, and a refused one leaves it so, with no timer running.
- *Late delivery* changes nothing of the above: the admission, the answer and every held
  event are folded by their own times, so a hello of B delivered after S but timed
  before it is B's hello at its time.

## As built

The record derives the selection by event time from the gateway's steps
(`internal/channel/selection.go`, `history.go`); the gateway tells them
(`gateway/selection.go`) and the endpoint binds and refuses
(`bridge/endpoint/channel.go`). Where the rules left a choice:

- **A second admission while one is pending replaces it** and keeps what the first
  held: no answer came for the first, and the held events are still the next
  conversation's to judge.
- **An answer with no admission before it** is taken as admitted at its own time with
  no timer: the gateway tells an admission before it forwards a request, so only a
  lost step reaches here, and a timer opened at the answer would have no hello to
  wait for.
- **A call's outcome is its connection's.** The endpoint tells a missing observation,
  a command that cannot start and a validated ticket with the generation and thread
  of the request that asked, so a wait admitted for A that ends after B is selected
  stays A's (*revised after the build review, October 4, 2026*). Each is judged as
  that connection's event: of the conversation when its connection is bound to it or
  unbound, nothing when foreign; while a selection is pending, one of a connection
  bound to the old conversation counts for it at once, as its own close does, and any
  other is held and counts for the selected conversation when its connection is
  unbound or bound to it, as a held hello does. One naming no connection is taken by
  its thread. So on Codex a ticket does not end the history as on Claude Code: every
  ticket is kept and folds by its own time — working, the interval closed, the
  conversation's timer stopped, a pending selection's only once its answer selects that
  conversation — and a later ticket of the same connection does not replace an
  earlier one, since a selection between them may hold one and not the other.
  `tool last worked` stays the run's latest ticket, whichever conversation it proved.
- **An answer selecting the conversation again refolds its pending period** from where
  the conversation stood at the first admission: the admissions, its own connections'
  events counted while it was pending and the held events, together by event time
  (*revised after the second build review, October 4, 2026*). So a hello of an unbound
  connection held before an own close shows the close left a connection live, and it
  was no failure — applying the held events over the state the own ones had already
  left reordered them. Each admission opens its own timer at its own time, so one that
  ended before a later event passes before it. While the selection is pending no hello,
  ticket or failure ends that timer: the answer may select another conversation, and
  A's proof is not B's start. Once the answer selects A again, the start the admissions
  waited for is A's, so a hello or a ticket of A's own connections ends that wait at its
  own time, as a held one does — a second server of A seen and then closed shows "server
  gone" at its close, never "no hello observed" at the timer's end (*revised after the
  third build review*). An own failure folds as it did while pending and ends no timer.
  An answer selecting another conversation never takes A's events. An answer with no
  admission opens no new wait and leaves the conversation's running timer as it was.
- **Another thread admitted while a selection is pending is dropped**: the pending
  selection already runs its own timer.
- **A connection lost while it owns the primary** tells the selection failed: the
  gateway empties the primary, and the terminal is told to select one again.
- **The primary is "" while a selection is pending**, so the endpoint refuses every
  Codex call then, the selected conversation's own included, until the answer.

Tests: `internal/channel/selection_space_test.go` folds every sequence of fourteen
selection letters — admissions with and without a target, answers selecting either
thread, a refusal, a second connection's hello and binding, a sub-agent's binding,
closes, startup statuses and the timer — after a working conversation, up to depth
five, the same letters with a missing observation, a command that cannot start and a
ticket over either connection up to depth four, and all of them with the second
connection bound to A again up to depth four six seconds apart, so a timer still runs
at the next events; each sequence also
in sorted and reversed arrival order. The oracle (`selection_oracle_test.go`) is
written from the rules above and built unlike the record: it does not hold and
release events, but first reads ahead how each selection of the whole path ended, then
folds the path once in event time, deciding for each event by the selection pending at
its time whether and how it counts — so it cannot share a mistake in the order the
record folds held events, as the earlier incremental one did. `table_codex_test.go`
holds the failure table's Codex rows by name; `gateway/selection_test.go` and
`bridge/endpoint/bind_test.go` the two sources, the latter with a wait that outlives
the primary's change.
