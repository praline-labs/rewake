# Context and remote control through the mod

October 18, 2026, Claude Code 2.1.302, with the `mods` skill snapshot of that version.
This is a source and test-kit check, not live acceptance. The mod API remains in that
skill; rewake's choices and wire protocol are in
[the adapter design](v2/design-claude.md#telemetry).

- `claude plugin validate --strict` accepts the module and its closures over the native API.
- The kit exercises initial context, measurement and turn-end samples, unknown usage after
  compaction, compact with focus while the heartbeat continues for 45 seconds, refusal
  during a turn, native/manual completion, a subagent excluded, and a skipped compaction.
- Plugin-origin compaction is counted through the returned result; native compaction is
  counted through its event. The kit counts the remote completion once.
- Interrupt waits for the matching aborted turn, sends the requesting actor as the stopped
  report, and appends it to native history without a new turn; a subagent's end cannot settle it.
- Host tests exercise the control files through heartbeat dispatch and the final outcome,
  deduplication, stale samples, unknown tokens, reload and lapse, wrong conversation,
  unsolicited replies and a canceled command that was never dispatched.

The validator warns that an organization-managed installation skips a locally installed
plugin's `session.compact` observer. Complete native manual/automatic compaction coverage
is not promised there. Remote completion still comes from the explicit call's result.
The event hook only observes and passes through; it adds no policy veto.

Effort is unknown: no current-effort getter was verified. Rate limits are excluded by
main's decision for this step. No transcript, summary or full context breakdown is sent.
Reload or transport loss may lose a compaction's final outcome; the new path adds no durable
outbox. Main owns live acceptance of context, focus, interrupt and post-control mail on the
installed harness before the room moves to this build.
