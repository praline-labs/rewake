# Grouped inbox review and installed acceptance

September 19, 2026. Revision 6 is accepted for prompt active delivery, preserved
original task reporting and subsequent idle wakeup. The [delivery contract](inbox-groups.md)
has no dispatch prerequisite based on peek, terminal events or a completed task.

## Candidate identity and review

The independently reviewed source archive contained 306 files. Archive SHA-256:
`e169d4c080b6bddaac10fd90fef1981c2d5d07e834d9f7d5d53112c9ca0ac941`.
The candidate used that unchanged archive, with VCS stamping disabled and trimpath.
Executable SHA-256:
`303c1020cefb39e7a9bc28639ae42484a001143a7c8966875fb47ad01d920307`.
The pinned native CLI version was 0.154.0.

Independent review found no blocking runtime defect. Its P3 documentation finding
was corrected: main/write are eligible recipients of explicit main-authorized Git
metadata grants; choosing a role does not request access. This and the acceptance
record are documentation-only follow-ups to the reviewed runtime.

All five required checks passed independently on the archive. Focused regressions
passed three shuffled race-enabled repetitions. An additional full mailbox/native
socket/CLI/durable-report regression dispatched mixed groups during active work and
in-flight ACK, then verified one finished/error/stopped report containing all four
exact task/question IDs and sender epoch. Duplicate publication/terminal events did
not duplicate the report. This establishes fixture behavior, not live model consumption.

## Startup and installation

The complete wrapper with the native server passed an isolated automated smoke:
56 responses, no refusals, one connection, zero model turns and zero HTTP requests.
It exited successfully and removed its registry record and sockets. That fixture
used a synthetic client, not a native TUI.

The owner separately confirmed clean startup in a real terminal with the normal
configuration and a temporary state directory, then installed the candidate and
restarted the three role sessions. Installed executable SHA-256 matched the build
record. New session records were observed; no per-process executable hash is claimed.

## Active and idle delivery

A general session ran four separate sleep-10 tool calls in one task. After fresh
working telemetry, main sent a new notification marked `STEER-V6-LIVE-01`. The
recipient read it after the first sleep and before the second, acknowledged receipt
and continued the original task. The original finished report later confirmed all
four steps completed. Dispatch needed no terminal or inbox overview; the recipient
used inbox after notification to retrieve the body.

After the recipient became idle, a new task woke it and returned the requested
finished marker `IDLE-WAKE-V6-OK`. This confirms active consumption between tool
calls, preservation of the original report, and new-task wakeup from idle.

The preserved revision-6 review report, build.json, full-wrapper-smoke results,
owner-startup-acceptance.md and owner-live-acceptance.md carry the detailed evidence.
Earlier submissions and failure proofs remain unchanged. These short live checks
do not establish every race, recovery path, permission transition or telemetry
workflow. The older readiness/missing-notice incident remains open separately.
Native notifications are the next feature; persistent Git permissions and the
unified check runner remain separate roadmap items.
