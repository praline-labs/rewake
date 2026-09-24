# Transport and launch milestones — September 17, 2026

[Back to the roadmap](roadmap/README.md). Preserved milestone history; later policy
changes remain noted in their original sections.

## Milestone 10. Session-owned server transport — done, September 17, 2026

The owned server and TUI share the wrapper lifetime. An optional Backend keeps
all process/RPC mechanics inside the adapter. Delivery uses start-or-steer,
tracks root thread events and refuses closed or ambiguous targets. Completion
callbacks share the existing receipt path; stopped keeps work owed to a human
continuation. Fake-process and mutation checks cover these mechanisms.

Acceptance requires same-turn delivery during work, delivery to a fresh /new
thread, stopped after a keyboard interrupt, error after an API failure, and no
persistent pending delivery to a live, ready session. Model runs are performed
separately by the owner; protocol tests use a fake local server.

The obsolete queue subprocess, lock-file tracker and unused process-tree/fd
helpers are removed. The complete fake-process smoke covers delivery, stopped
and continuation, /new, API errors, closed-thread refusal and server death.
The milestone is closed by the live acceptance below. One criterion was not provoked live: a failed Codex turn; the fake server covers it, and a real one is recorded when it happens.

Live acceptance, on the owner's sessions with CLI 0.154.0 and Claude Code
(September 17, 2026):

| criterion | result |
|---|---|
| delivery to a fresh thread without a first word | passed: the turn started on its own |
| a report from a fresh thread's turn | passed: `finished` arrived |
| same-turn delivery during work | passed: a mid-turn note was followed in that turn |
| error after a failed turn | passed on Claude Code (a usage-limit stop); Codex only on the fake server |
| stopped after a keyboard interrupt | passed: a yellow `stopped` line, the wait was kept |
| delivery to a thread opened with /new | passed: the new thread started a turn; its `finished` answered both the stopped task and the new one, marked `threadChanged` |

## First input and fresh-thread observation

Completed changes are recorded in [the later history](reviews-later.md).
Round fourteen found R14-2; its replacement regression is recorded in [gateway integration](gateway.md).

## Remote continuation startup — fixed, September 17, 2026

Resume/fork omit generated permission grants; caller overrides stay intact with a warning.
Fake TUI checks cover all roles; all five checks pass. [API limits](continuation-permissions.md) prevent an idle root grant. Saved roots may be superseded; live acceptance stays open.

## Git metadata grants — superseded policy, September 19, 2026

The September 17 automatic task/role policy is superseded by
[explicit orchestrator intent](git-grants.md). Metadata validation and additive root
handling remain; launch roles and no-flag tasks no longer add permissions.

## Lost report mitigation — September 17, 2026; ownership remains open

Failed report notices retain their accepted text and diagnostics in unread;
expiry, reservations and task failure semantics remain intact. Established root
changes release obsolete observer subscriptions, including fresh idle targets
and late acknowledgements. Uncertain observer RPCs retire only that connection.
[Investigation and remaining limits](thread-ownership-investigation.md): loaded
roots do not establish terminal ownership, /resume can revisit earlier roots,
and ambiguity still refuses. Regression and mutation checks cover the bounded
fix; this does not close live ownership acceptance.
All five repository checks pass; all five targeted mutations are detected.

## Launch naming — done, September 17, 2026

The role or explicit prefix receives the harness suffix; conflicts add numbers or refuse. [Names](roles.md#names) retain the contract
and [review history](reviews-later.md#targeted-review--done-september-17-2026) records
all five checks and targeted acceptance.

## Explicit main only — implemented, September 17, 2026

Only --main creates main; defaults stay general, names never choose roles. Review, five checks and 72 synchronized claims passed on `9aedd0f`.

Later scoped acceptance is recorded in the [gateway integration entry](roadmap/2026-09-19-gateway-integration.md) and
[native mailbox acceptance](native-mailbox-acceptance.md); it does not broaden the
historical ownership limits above.
