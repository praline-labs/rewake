# Native evidence for the selected-conversation gateway

Verified September 18, 2026. Native executable version 0.154.0, SHA-256
`3188814c35471432d4123203e0eb38e5bddc60226e3d7ddf0e59e649ea140022`.
The local source reference is revision `44b901161`; matching version strings and
source fingerprints are not proof of exact binary/source equivalence.

The owner ran isolated native-terminal fixtures with the reviewed V5 gateway and
only a loopback synthetic model service. Fresh and CLI-resume lifecycle, /new and
A-B-A retained the accepted primary and usable terminal. Subsequent owner runs
confirmed a visible first response before any user message or seed/naming operation,
NEW/A/B/A responses, same-turn steering, native error and keyboard interruption.
Matching scoped callbacks were checked separately. Model HTTP request counts are
not task counts; the real terminal can make helper requests too.

The captured evidence is in the persistent gateway task package: V5 owner-fresh
and owner-resume assessments, and native-tui-delivery/owner-assessment.md with
proofs `delivery-fresh-owner-kiye4w0d`, `delivery-switches-owner-ze21u_2_` and
`delivery-outcomes-owner-0inuifgr`. Original source, builds, manifests and captures
remain immutable inputs. No durable mailbox or registered peer claim was made by
those fixtures. ND-1 fixed the fixture's final grading to validate the stable
post-shutdown metadata; it was not a gateway defect.

`server-message` metadata proves an upstream native event was observed. It is
recorded before the downstream write and cannot prove terminal receipt. Owner
confirmation of visible markers and usable input supplies that separate evidence.

## Source-backed ordering used in integration

- `tui/src/app_server_session.rs`, primary start and resume construction:
  ordinary interactive selection uses numeric request IDs and runtime roots;
  startup starts use the startup-thread-start ID family. Helper operations use
  separate dynamic/temporary IDs. Correlated direct-input replies validate roots.
  In remote mode 0.157.1 sends the roots null; the ordinary configuration builder
  (`:1795` there) always writes `web_search`, which now stands in for them on start,
  on a by-ID resume (history and path null) and on fork. An ordinary resume carrying
  it is not taken for a reconnect ([research-codex.md](research-codex.md#the-terminals-selection-on-01571)).
- `tui/src/app/session_lifecycle.rs`, resume backfill: the accepted primary is
  established before a loaded-thread inventory and numeric metadata reads. A
  bounded correlated cohort authorizes those reads without selecting among them.
  The ordinary goal query follows awaited backfill. Waiting for this workflow must
  not hold the same admission gate needed by its reads.
- `tui/src/app/reconnect.rs:65` and
  `tui/src/app_server_session.rs:2095`: reconnect explicitly resumes the chosen
  thread with PreserveExistingThread, omitting root overrides. The integration
  correlates that new request with the prior closed primary, never with loaded
  roots or a generated observer resume. It requires a new matching response.
  `app/reconnect.rs:388` replays the recovered selected view; it does not run the
  ordinary resume backfill workflow, so no backfill permission is armed for this
  correlated reconnect. A new goal/get is not fabricated as a readiness condition.
- `tui/src/app/permission_shortcuts.rs:40` and protocol
  `v2/thread.rs:229`: thread/settings/update changes session settings; it does not
  choose another conversation. Admission waits for its response before reading
  replacement workspace roots. The TUI updates its local state after that ACK.

These paths are covered by deterministic transport and integration tests and
completed independent review. The production acceptance boundary is recorded below.

## Fork and side evidence and owner contract

The owner required fork investigation and then clarified the address contract:
opening a side view does not change the rewake primary. Its user can ask a side
question while main keeps working, close side, and receive the original main
result. Main messages and reports must continue throughout; side answers must not
settle primary waits. Separate side addressing remains out of scope. This supersedes
the earlier proposal to refuse all work until resume/new after a side view.

The owner ran that sequence against the unchanged V5 gateway: CLI fork, `/fork`,
explicit resume of seed A, `/side`, Ctrl+C closing side, `/quit`. All screens worked
without errors, no TUI-phase model call or message delivery occurred, and isolation
and cleanup passed. V5 did not accept fork routing. That capture alone is not a
general selection rule. (The run's files were deleted with the research packages on
September 22, 2026; what they showed is this paragraph.)

Source at `44b901161` establishes the additional workflow context:

- `app_server_session.rs:942`: regular forks read parent metadata before the shared
  numeric fork RPC; side forks omit that read. The read alone proves no selection.
- `app/startup.rs:592`: CLI fork creates the initial primary. Integration carries
  this explicit launch mode, without recognizing arbitrary first fork requests.
- `app/event_dispatch.rs:442`: /fork awaits its new child, optionally names it,
  shuts down the previous chat thread, then replaces the widget with that child.
  The integration additionally requires a matching successful old-parent detach.
- `app/side.rs:726,747`: side creation keeps primary and is the production TUI call
  site for thread_inject_items; it prepares the returned child before local side
  selection. Correlating this setup identifies the side workflow, not a new primary.
- `app/session_lifecycle.rs:523` performs side focus locally and may refuse rendering.
  Primary authority does not depend on that local focus succeeding.
- `app/side.rs:667` and `thread_routing.rs:40` distinguish side disposal from ordinary
  primary replacement. Known side-scoped reads/input/interrupt/unsubscribe do not
  become primary intent. Parent closure or a contradictory workflow prevents revival.

Positive and negative protocol tests cover launch context, child/direct-input ACK,
old-parent detach, stale replies, side setup, admission while side is visible, and
side answers excluded from primary reporting. A loopback-only actual-server/fake-TUI
fixture exercises CLI and ordinary fork, an active primary, a synthetic side answer,
primary steering during and after side, and the original primary completion.
A separate mailbox integration test checks primary waits and durable publication.
These are integration evidence. Independent review and installation completed;
the installed primary/side check below separately verifies the exercised live path.

Explicit permission profiles can replace runtime roots in native request shapes
(`app_server_session.rs:818,994`). The same primary start/fork evidence accepts that
profile field without reading or replacing its value. Generation and direct-input
ACK checks remain mandatory; role-gated root grants still obey the selected policy.


## Installed primary/side delivery acceptance — September 19, 2026

The reviewed integration and startup fixes passed all five checks. Owner fresh
and resume runs with the usual configuration were stable; [startup evidence](startup-transport.md#owner-acceptance-and-installation--september-19-2026)
records the installed binary. All three sessions restarted. The orchestrator sent
a short task to the general session and received automatic `NEW-REVIEW-OK` / `1.25`.
The write session then committed and pushed `e465729` and returned automatic
finished, verifying ordinary task/report exchange and the delivered Git grant.

For the decisive side check, the owner kept the general session's side view visible.
The orchestrator sent a new task to its primary conversation and received automatic
`MAIN-WITH-SIDE-02` / `1.25` through inbox. The owner confirmed they were still in
side when that result arrived. The owner then closed side with Ctrl+C; a new task
returned automatic `MAIN-AFTER-SIDE-OK`, without resume, restart or repair. No side
answer settled either primary task. This accepts the concrete installed-build path:
new primary work and its report while side stays visible, then another task/report
after side closes.

An earlier attempt returned its correct primary result after a delay and a view
switch. The owner reported possible upstream API trouble; that observation does
not prove side paused execution. Neither the successful repeat nor the close test
establishes the cause of every older missing report or warning. Prototype fork,
steer/error/stop evidence keeps its separate scope; untested native workflows are
not accepted by this check. The facts behind it, from the record deleted on
September 22, 2026: installed SHA-256
`b3837e9beb08f0574c79c3b1b2500e11ae41d811910b4a885e2d65017a383bc6`, production commit
`e465729`. With `/side` open in general, main's task
`1789766141205224888-fed0cdf2e99b` was accepted at 00:15:41 and pinned to the main
conversation `01a0b43d-cd0b-79b2-a589-19f8173c55fd`; it was delivered, stayed unread
through the first wait, and general returned `MAIN-WITH-SIDE-OK` automatically at
00:23:34, which main read through inbox. A second task with marker
`MAIN-WITH-SIDE-02` came back at 00:24:50 while the owner confirmed they were still
in side and had switched nothing. The first delay is not attributed to side: the
owner warned of upstream API problems that day, and that remains unresolved.
