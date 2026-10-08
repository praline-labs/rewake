# Research: what decides the conversation a Codex delivery goes into

Split from [research-codex.md](research-codex.md) by subject on September 30, 2026: that
document carries the standing facts about Codex CLI; this one carries the source facts
read for [delivery-conversation.md](archive-1.x/delivery-conversation.md) — what states a
conversation's permissions, how the server applies a change of them and tells it, and the
warning the terminal draws without a turn. All of it was read in the source, none of it
seen live, and it ages with the version it was read in: recheck before relying on it.

The first two sections were read for a check of the sandbox before each delivery, taken
out of the failed-resume fix on September 30, 2026
([delivery-conversation.md](archive-1.x/delivery-conversation.md#when-the-worker-could-not-read-its-mail)).
The facts stay true of 0.159.0 and are kept for what comes next. What they showed is why
the check was dropped: the stream says what the settings are, not which request they
answer, and not which permissions a running turn keeps.

## What states a conversation's permissions

**[read in the source; Codex `rust-v0.159.0`; September 29, 2026]**

- The replies to `thread/start`, `thread/resume` and `thread/fork` state the
  conversation's effective permissions: `sandbox`, a legacy `SandboxPolicy` tagged by
  `type` — `dangerFullAccess`, `readOnly {networkAccess}`, `externalSandbox`,
  `workspaceWrite {writableRoots, networkAccess, excludeTmpdirEnvVar, excludeSlashTmp}` —
  with `activePermissionProfile {id, extends}`, `cwd` and `runtimeWorkspaceRoots`
  (app-server-protocol v2).
- `thread/settings/updated` restates them when settings change:
  `params.threadSettings.{sandboxPolicy, activePermissionProfile, cwd}`, without the roots
  (`app-server/src/bespoke_event_handling.rs:1243`). It is sent only when the applied
  settings differ from the last ones noted for the conversation
  (`thread_state.rs:220`, `note_thread_settings`): an update that changes nothing is
  never stated, and one the core refuses is told only as an error event
  ([below](#settings-updates-are-queued)).
- The reply to `thread/settings/update` is no statement: the handler queues
  `Op::ThreadSettings { reply: None }` and answers `{}` at once
  (`request_processors/turn_processor.rs:953–971`; its own comment at `:810–812` says
  clients should wait for `thread/settings/updated`). The application comes later, in
  that notification.
- A turn may replace the runtime roots: `turn/start.runtimeWorkspaceRoots` sets them for
  that turn and the ones after it (`protocol/v2/turn.rs:207`), and nothing restates them,
  so roots a lifecycle reply stated go stale with the next turn that sends its own.
  `thread/read` with `includeTurns: false` names the current ones in its environments.
- `thread/read` states no permissions: its `Thread` carries none.
- The legacy policy is a projection of the permission profile, and a lossy one for a
  profile of the user's own: `compatibility_sandbox_policy_for_permission_profile`
  (`sandboxing/src/manager.rs`) drops its deny rules. The built-in profiles are
  `:read-only`, `:workspace` and `:danger-full-access` (`protocol/src/models.rs:410–416`).
- `thread/shellCommand` runs outside the sandbox (`protocol/v2/thread.rs:1160–1165`), so
  it cannot test what the sandbox allows.

## Settings updates are queued

**[read in the source; Codex `rust-v0.159.0`; September 30, 2026; write-claude and
review-codex]** How a change of permissions is applied and told:

- The core takes queued operations one at a time in its submission loop
  (`core/src/session/handlers.rs`). `Op::ThreadSettings` is applied there and, on
  success, emits `ThreadSettingsApplied` with the new snapshot (`:522–550`,
  `session/thread_settings.rs`, `emit_applied`); on failure, with no reply channel — the
  case of `thread/settings/update` — it emits an `Error` event, `invalid thread settings
  override: …`, `codexErrorInfo` `BadRequest`. A `turn/start` goes through the same loop
  as `Op::TurnInput`, and applies the settings it carries before its turn, emitting
  `ThreadSettingsApplied` too (`session/turn_input.rs:139–178`).
- The app-server's listener for a conversation takes its events one at a time and awaits
  each one's notifications before the next (`request_processors/thread_lifecycle.rs`, the
  `next_event` arm): `ThreadSettingsApplied` becomes `thread/settings/updated` when the
  snapshot differs from the last one noted, and `TurnStarted` becomes `turn/started`
  (`bespoke_event_handling.rs:155`, `:1234`). The noted snapshot starts as the one the
  listener was set up with, on a start or a resume (`thread_state.rs:137`).
- `thread/resume` of a loaded conversation reads `config_snapshot()` as it is
  (`thread_processor.rs:4337`): an update still queued is not in its reply.
- The reply to `turn/start` is sent by the request's task once the core has taken the
  turn, not by the listener, so it may reach the client before a notification of an
  update the core applied first.
- `SandboxPolicy` in v2 fills an omitted field with its default (`serde(default)`,
  `protocol/v2/permissions.rs:541–567`): `writableRoots` empty, `networkAccess` false or
  `restricted`, the two exclusions false.
- `thread/settings/updated` carries the conversation's `config_snapshot()` read when the
  notification is built (`bespoke_event_handling.rs:1235–1237`), not the snapshot the
  event was emitted with, and names no request: equal values do not show which queued
  update they come from.
- Settings apply to the turns after them; a running turn keeps the context it started
  with (`core/src/session/mod.rs:1898–1899`, `session/turn_input.rs:193–201`). Input sent
  while a turn runs goes into that turn, so the permissions last stated need not be the
  ones it runs under.
- `turn/start` answers a new turn and input steered into a running one alike, with the
  turn's id (`request_processors/turn_processor.rs:672–715`): the reply does not tell a
  new turn from a continued one.

## A warning shown without a turn

**[read in the source; Codex `rust-v0.159.0`; September 29, 2026; not seen live]** The
server notification `warning` `{threadId?, message}` is drawn by the terminal as a warning
line without a turn of the model (`tui/src/chatwidget/protocol.rs:233`, routed by thread in
`app/app_server_event_targets.rs:185`). rewake sends one through the gateway to show the
person why deliveries wait ([delivery-conversation.md](archive-1.x/delivery-conversation.md#who-is-told)).
