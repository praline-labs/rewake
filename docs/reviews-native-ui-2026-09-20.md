# Native arrival UI review and acceptance — September 20, 2026

[Roadmap](roadmap.md) · [UI contract](native-mailbox-ui.md) ·
[Earlier review history](reviews-later.md)

## Prototype and the first owner observation

UI-event-probe-1 was isolated from production. Five Go checks and independent review
found no blocking defect. A single post-ACK commandExecution completion used the
reserved thread and real ACK turn, checked current ownership, and entered only the
TUI queue. No upstream/model input, accounting callback, fake turn terminal or child
was introduced. Tests covered scope/queue pressure, active exec and streaming.

The owner saw the three idle/exec/stream rows with the old parenthesized label.
The exec notice appeared above the completed sleep cell; the stream notice appeared
after all twelve text fragments, consistent with native deferral. The owner was
unsure of chronological ordering from the final screen. There was no complete
owner-mode event trace, so exact visual timing is not claimed. The separate synthetic
preflight provides protocol ordering evidence, not a replacement human observation.

/new worked, but /resume refused: permission overrides are not supported when
resuming a remote task. That failed attempt remains a failed fixture procedure,
not an initially successful lifecycle check or a proven renderer defect.

## Fixture correction and lifecycle repeat

UI-event-probe-2 moved the unchanged workspace-write/approval-never defaults from CLI
flags into the fixture's private user config, with no selected profile. Runtime,
binary and schema were unchanged. Independent review confirmed the native guard's
source-layer distinction, both recorded child argv without permission overrides,
and effective native workspaceWrite/never policy with networking disabled. Ordinary
exec/stream still passed; the lifecycle-only case honestly marked them not-run.

The owner completed the short /new/resume/picker sequence without reporting the
prior refusal or a picker anomaly. On row retention, the owner reported seeing it
only once at the beginning. This establishes initial visibility and no noticed
repeat, not durable retention, cache invalidation or an exhaustive duplicate check.
The owner explicitly accepted transient arrival display and deferred any storage or
reconstruction. Durable mail/read/report guarantees were not relaxed.

## Default integration and new label

UI-integration-1 promoted the same mechanism into the ordinary owned-adapter path,
removed the probe switch and changed the label to `rewake notice --display-only`.
Only reservation.go and notice_display.go changed runtime behavior from 0d4b962.
The label is not an executable CLI command; native still supplies Ran.

Independent review found no defect. It reproduced all five Go checks, targeted race
and scope controls, default publication without opt-in, accepted delivery despite
cosmetic failure, no replay and both native synthetic cases. Routing tests explicitly
consume the display frame so it cannot hide a missing native lifecycle reply.
Normal tools/config/permissions and the socket/hook adapter remained unchanged.

The owner then ran the short actual-TUI label check and saw
`Ran rewake notice --display-only` with the ordinary notice beneath it. The owner
accepted the appearance; exit and cleanup were clean. This check did not repeat
exec/stream or establish UI persistence. The old quoted label remains in its original
prototype evidence, not silently rewritten as the new label.

## Installed acceptance and closeout

The owner installed the reviewed binary, SHA-256
`5b5f859de255714604bd610ebed8649403c5d853e5cd817296d583ec301b2586`, and restarted
main/write. Main received availability, sent a short task and received/read its
automatic native finished report UI-INSTALLED-OK. Asked about the visible Ran rows,
the owner answered "вижу" ("I see it"). Installed task/report handling and visible UI are accepted.

Runtime stayed byte-identical to the reviewed source during closeout. Final
production-mirror checks cover the exact agreed source, excluding ignored research
and unrelated ideas. No further model or TUI test was needed to record acceptance.
The [current check recipe](native-mailbox-ui-check.md) retains private permission
defaults and the short lifecycle-only boundary for a future acceptance need.

## What the evidence was

The local package holding it was deleted on September 22, 2026 — raw runs and
fixtures, never in git. The observations it carried are quoted here.

**The owner's decision on transience**, September 20, 2026, after the renderer and
lifecycle experiments:

> да и это нормально для решения. Когда-нибудь позже подумаем как сохранить, но
> сейсчас и так отлично

(Fine as a solution; how to keep the row can be thought about some day, for now it is
good as it is.)

**What the owner saw**, running the reviewed lifecycle fixture, with exit 0 and
empty controller and endpoint error lists:

```text
• Ran rewake notice --display-only
  └ 🟢 Rewake: shell task, 1 new message
      ↳ UI probe: idle mailbox notice
```

**How long it stayed.** Asked specifically about retention after `/resume`, the
owner answered "я видел только 1 раз в самом начале" ("I saw it only once, at the
very beginning"). Only the initial appearance is
confirmed, and no repeated row was noticed — which is not evidence of durable
retention, of cache invalidation, or of an exhaustive duplicate check.

**Where the row appeared.** Rows appeared for the idle, exec and stream phases. The
exec row appeared above the completed command's row; the stream row appeared after
all twelve fragments, which reads as deferred rendering rather than display at
arrival. The owner reported no malfunction but was not certain of the ordering from
the final screen.

**A fixture defect, not a product one.** `/resume` refused with "Permission
overrides are not supported when resuming a remote task", because approval and
sandbox flags were on the TUI argv. The repaired fixture put those in the private
user config instead, and the refusal was not seen again.

**Installed acceptance.** Binary SHA-256
`5b5f859de255714604bd610ebed8649403c5d853e5cd817296d583ec301b2586`; a task between
the restarted sessions returned `UI-INSTALLED-OK` as a `rewake_mailbox_notice`,
read through inbox, and the owner confirmed the visible row with a single word:
"вижу".

This closeout neither accepts UI-history persistence nor broadens the existing
transport ownership and recovery scope.
