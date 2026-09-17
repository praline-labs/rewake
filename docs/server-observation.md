# Observing fresh-thread results

Verified September 17, 2026 against CLI 0.154.0, source snapshot
`44b9011611e1f4213ef34bd51b33476475803a94`. Paths below are inside codex-rs.
No native harness or model was started for this follow-up.

Known limitation found later the same day: loaded-root metadata does not establish
terminal ownership. Obsolete observer subscriptions are now released on established
root changes; the server may still retain detached roots during its unload delay.
See [ownership investigation and mitigation](thread-ownership-investigation.md).

## Subscription is separate from identity

`app-server/src/request_processors/thread_processor.rs:1549,1621` attaches the
thread/start caller, then broadcasts thread/started. That broadcast does not
subscribe other connections. `thread_lifecycle.rs:335–344` snapshots subscribed
connection ids for each event; `outgoing_message.rs:195–205` routes the typed
notifications only to that set. turn/start does not attach its caller.

Running thread/resume first reads stored metadata
(`thread_processor.rs:4180–4220`) and attaches only after that succeeds
(`thread_lifecycle.rs:675–705`). excludeTurns=true avoids history but does not
bypass the metadata requirement. Metadata-only thread/read can describe a loaded
fresh thread before persistence (`thread_processor.rs:2779–2856`). Therefore
identity suffices for delivery, while resume may still return -32600 no rollout.

The owner's [fresh-thread log](/tmp/rw13/live-new-ok.jsonl) confirms both facts:
turn/start reached the TUI/model after /new, yet the unsubscribed observer got no
turn events; excludeTurns resume before the first turn refused for missing rollout.
Global thread/status/changed still arrived, including active and idle.

## Persistence follows turn start

`core/src/tasks/regular.rs:46–61` emits TurnStarted before waiting for prewarm or
running the turn. The latter records the user prompt later;
`core/src/session/mod.rs:4743–4773` emits its item notifications and only then
calls ensure_rollout_materialized. That function calls LiveThread.persist
(`:1334–1351`). A resume triggered by active can consequently race persistence.
Retrying the specific no-rollout error during the active interval is necessary.

`app-server/src/bespoke_event_handling.rs:153–180` marks active before sending
typed turn/started. At `:183–195` it marks completion/idle before sending typed
turn/completed; `:1302–1338` includes the final assistant message in that terminal
notification. Subscription can miss earlier items without losing the final text
if it attaches before the terminal event's subscriber snapshot.

## The short-turn race

No acknowledgement barrier delays a turn until external observers subscribe.
A fast failure, interruption or reply can finish before rollout lookup and
subscription succeed. Even a successful resume can arrive after the terminal
event captured its recipient list. History-free observation cannot recover it.

The backend retries every 50 ms during an observed active interval, limits each
RPC to 500 ms, and stops retries on idle, closure, /new or shutdown. An in-flight
RPC keeps its bounded budget across a thread change; generation guards route a
late attachment to cleanup. Other refusals stop retries for that interval.
Subscription success records the connection, thread
and generation, preventing an old acknowledgement from selecting a new target.

Idle starts a 500 ms completion grace. If no completion arrives, the backend
emits error with only the diagnostic "completion not observed", not model text
or a guessed failure cause. Normal completion cancels this fallback. If no turn
id was observed, a run-local interval id makes report retries idempotent. A late
completion is suppressed while its interval is still identifiable; a reconnect
or multiple missed intervals cannot be reconstructed reliably. Missing the whole
active/idle interval during disconnect remains unobservable. Full recovery needs
an upstream replay/subscription barrier; reading transcripts is not a fallback.

The strict fake requires a persisted rollout for resume, rejects history access,
and sends turn/item/error events only to subscribed connections. Tests exercise
delayed persistence, fresh /new, retry cancellation, normal idle-before-completion
ordering and a terminal event lost before subscription. Live acceptance remains
an owner-run milestone check.
