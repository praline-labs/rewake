# Conversation tracking for Claude Code — September 23, 2026

The owner approved, on September 23, 2026, conversation tracking for Claude Code
(HF-10, entry 6 of the [parity queue](../harness-features.md#open)). On Codex rewake
knows which conversation a message was delivered to, and a report from another one —
after `/new` — carries `threadChanged`, so the sender knows the answer may not be about
its task and can resend. On Claude Code the field was always absent: nothing recorded
where a message went, though `/clear` changes the conversation just the same.

**What was built.** The wrapper pins every owed delivery to the conversation Claude
Code's telemetry collector heard last — the `session_id` every hook and status line
carries ([entry](2026-09-23-claude-telemetry.md)). The collector answers through
`harness.ThreadSource`, an interface the wrapper asks of a session's observer, since
Claude Code has no backend to ask. At the end of the turn `rewake turn-ended` reads the
conversation from the Stop or StopFailure payload's own `session_id` and compares it
through the path Codex reports already take (`inbox.ReportThreadChanged`). Mechanism:
[delivery-adapters.md](../delivery-adapters.md#claude-code-adapter).

**Decisions.**

- Either conversation unknown leaves the field out rather than guessing: no telemetry
  socket, nothing heard before the delivery, a payload without `session_id`. The
  collector never refuses a delivery for want of one, unlike the Codex server, which
  cannot deliver without a thread.
- The turn end's side comes from the hook payload rather than from the collector's
  published snapshot: the payload names where the turn ended, the snapshot is up to a
  quarter second old. A Codex notify payload is not read for it; that conversation
  comes through the gateway.
- One false mark is accepted: a message delivered after a `/clear` but before its
  background SessionStart hook reached the collector is pinned to the old
  conversation. The mark is advisory; a false one costs a resend, a missing one would
  let an answer from a fresh conversation pass for the task's.

**Tests.** Unit tests: the collector's conversation before any event and after a
`/clear`; the wrapper pinning a delivery to an observer's conversation; the turn end
marking a report when the Stop or StopFailure `session_id` differs from the delivery's,
and leaving the field out when they match, when the payload names none and when the
delivery was not pinned; only a hook payload naming the conversation. The workflow case
`thread-changed` runs two workers on the Claude Code column, one of which plays `/clear`
after its task arrives: its report must carry `threadChanged` and still settle the task
once, and the other's must carry none. Three mutants — the delivery never pinned, the
Stop `session_id` ignored, every known conversation taken for a change — each break
exactly the observation they name ([testing.md](../testing.md)). The fixture's startup
status line used to name a conversation of its own while its hooks named the session,
which the tracker would have read as a `/clear` in every Claude Code case; it now names
the session's.

**Open.** Not observed live: HF-10 stays **impl?** for Claude Code until a `/clear`
between a delivery and its report is seen on a real session. The same day's research
saw `/clear` and `/resume` leave a session under rewake working (HF-19, now live), and
found that an interrupted turn's task is settled by the next unrelated turn end
([traps.md](../traps.md#an-interrupted-claude-code-task-is-reported-finished-with-an-unrelated-answer)):
across a `/clear` this tracking now marks that report `threadChanged`, in the same
conversation nothing does.
