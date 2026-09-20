# Native mailbox acceptance — September 20, 2026

The owned adapter's [standalone mailbox output](native-mailbox.md) is accepted for
ordinary-briefing task/report handling. The owner accepted fresh sessions, then
installed the reviewed binary and restarted the working orchestrator and writer.
The socket/hook adapter was unchanged. This is acceptance of the scoped feature,
not a new exhaustive recovery, side-selection or compaction matrix.

## Versions and source identity

Native version 0.154.0, binary SHA-256:
`3188814c35471432d4123203e0eb38e5bddc60226e3d7ddf0e59e649ea140022`.
Reference revision `44b9011611e1f4213ef34bd51b33476475803a94`; no claim of exact
source/binary equivalence. Reviewed, owner-tested and installed wrapper SHA-256:
`1278c8db23c21f0d072bae0bc211eb6652499a37ece26b922497a89625d02d01`.

Integration-1 introduced the runtime. Independent review found no blocker but
identified guard assertions that could pass on expiry, stale grouping terminology
and a sender-side proof gap. Integration-2 closed those findings without changing
any of the 121 non-test Go files. Its independent repeat review reproduced both
guard mutations and the strengthened native sender/receiver proof.

## Evidence levels

| evidence | observed result | limit |
| --- | --- | --- |
| Early unprompted standalone output controls | Two generic acknowledgements; marker_missing | Cause not established; these remain failures |
| Explicit semantic idle control | Returned the value carried only in standalone output | Used an explicit fixture task |
| Explicit active standalone control | Pending tool, same-turn ACK, gate release, matching completed turn and expected value | Deterministic ordering for that fixture, not every integrated run |
| Isolated full-wrapper synthetic run | Real send, native output, CLI inbox read, automatic finished report and matching sender-side native output | Scripted endpoint and controller choose responses/reads; no model comprehension claim |
| Fresh owner idle check | Generated briefing only; worker solved 17 times 19 as 323; main automatically read the report | Explicit workspace-write selected for the disposable projects |
| Fresh owner normal-tool check | Owner accepted sleep 30 plus notify requesting 23 times 29 in the result | No separate integrated event trace; sleep progress was unclear in the UI |
| Installed task/report check | After restart, writer returned NATIVE-INSTALLED-OK; main received native output and read inbox | Supported resumed working sessions, not an exhaustive recovery matrix |

For the fresh checks, no task or mailbox instructions were typed into the worker.
The owner reported no visible user-message bubble for notifications in either
terminal. Main also woke on availability. The normal-tool scenario was accepted as
"да, все ок"; this owner observation must not be promoted to the exact same-turn
proof supplied by the earlier standalone dynamic-tool control. No additional model
run was needed to replace that successful observation with a more elaborate trace.

Main subsequently received availability and writer departure/rejoin as actual
rewake_mailbox_notice events, read inbox, sent the installed check and read its
automatic finished report. The installed file matched the accepted wrapper hash.
This establishes both installed task and report handling without a user chat prompt
to collect the report. Existing tool configuration was not replaced by fixture
restrictions; the owner exercised an ordinary shell tool in the integrated check.

The earlier Completed-renderer experiment remains unsuitable: it created an unusable
picker entry. Neither that side effect nor either marker_missing baseline is
reclassified as success. The production path adds no synthetic delegation renderer.

## Fresh project permission lesson

The first disposable run woke main, which attempted inbox but could not write its
`.lock` under read-only permissions. Existing configuration selected neither
sandbox_mode nor default_permissions, and the new project had no trust selection.
Source check at revision 44b9011, `core/src/config/permissions.rs:51–63`, selects the
built-in read-only profile without such a project selection. This was an acceptance
environment failure, not a reproduced toolOutput defect.

The owner relaunched only the test sessions with the ordinary per-launch
`-c 'sandbox_mode="workspace-write"'`. The server receives that explicit setting.
Installed configuration and runtime were unchanged; no automatic permission grant
was added. The [updated recipe](native-mailbox-check.md) makes this prerequisite
explicit. Inbox reads write locks and read receipts, so this acceptance does not
prove operation under a deliberately read-only policy or resolve that separate
support question. User permissions must not be silently widened.

## Preserved records

Ignored research packages retain the original evidence without entering production
source. Under `handoffs/native-notices-2026-09-19/`: the two integration review
reports, integration-2's source/fixture manifests and both-direction proof,
`outputs/integration-2/owner-live-observations.md`, and `installed-acceptance.json`
record this acceptance. Earlier standalone and renderer proofs remain in their
original packages. The original owner recipe is preserved; the current recipe is
published separately rather than rewriting a hashed historical artifact.
