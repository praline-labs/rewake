# Claude Code parity probes — September 21, 2026

The first three entries of the [parity queue](harness-features.md) are closed:
HF-12, HF-07 together with HF-20, and HF-21. Orchestration moved from Codex to
Claude Code on September 21, 2026, which is what made them observable — the
configuration the earlier **impl?** marks were waiting for. The two mailbox probes
went from a Claude Code main to a Claude Code writer. HF-12 has no recipient at all:
it is a `rewake list` read performed by that main, and the side being measured is a
Codex worker. This is acceptance of three observed behaviours, not a new coverage
matrix for either harness.

## Versions and build identity

| Session | Role | Harness |
| --- | --- | --- |
| main-claude | main | Claude Code 2.1.270 |
| write-claude | write | Claude Code 2.1.270 |
| luna-codex | general | Codex CLI 0.155.1, model gpt-5.6-luna, effort low |

Every version above is a session's own answer to a request to run `--version` on
September 21, 2026; the sender never saw the command output. The delivery facts in
[research.md](research.md) were gathered on Codex CLI 0.154.0, one minor behind.

All three sessions ran the installed rewake build of commit 4cfd3cf; the working tree
was at 3d33467. No executable hash was taken for this round, so build identity here is
claimed by commit only, not by digest.

## HF-12 — a Codex worker's telemetry read from a Claude Code main

`rewake list` from main-claude while luna-codex was working:

    luna-codex  general  "codex"  working  "gpt-5.6-luna"  "low"  4% / 258K  0

The role-gated header resolved for a Claude Code main, and the Codex worker's model,
effort, context and compaction count were all populated. The compaction count read
zero, which is indistinguishable from an unfilled field: a non-zero count has not been
seen from a Claude Code main, so that part of the header stays unproven.

Claude Code sessions in the same listing reported `unknown` for every telemetry field;
a socket harness has no collector (HF-11), so this is the documented shape, not a
fault of the fetch.

A snapshot of the same session taken **before its first turn** showed `idle`, model
`gpt-6-astra`, effort `high` and unknown context — the launch defaults, not the
configuration that session was actually running. The real model and effort appeared
only once work began. A pre-turn snapshot therefore proves session existence, never
the selected model or effort; only a snapshot taken after the session starts working
is evidence of its configuration.

## HF-07 and HF-20 — grouped arrival and selected reads on a Claude Code recipient

Two messages were sent in parallel inside the 150 ms collection window. Their IDs
begin `1789938609370371597` and `1789938609370643712`, so the actual gap was 272 115 ns
— about 0.27 ms. The recipient's records, verbatim.

One notification announced both:

    Rewake: 2 new messages
      ↳ main-claude task: Письмо B: сообщи число строк в docs/flow.md. Выпо…

The group header carries no sender name; the single-message form seen earlier in the
same session was `Rewake: main-claude task, 1 new message`. The preview is taken from
the **latest** member, which is the owner's September 19 presentation correction in
[inbox-groups.md](inbox-groups.md), not a divergence.

`rewake inbox --peek`:

    2 unread messages (overview; none marked read):
    1789938609370371597-9d5c47453e20 · main-claude · task · 2026-09-21 00:10:09 · Письмо A…
    1789938609370643712-09fb390ac297 · main-claude · task · 2026-09-21 00:10:09 · Письмо B…

Peek consumed neither message. `rewake inbox --message <id>` returned the full body
for each ID in turn, and the closing `rewake inbox` reported `No new messages`. The
two obligations stayed separate: the recipient carried out both tasks the messages
asked for — `go version` and `wc -l docs/flow.md` — rather than treating the group as
one item.

## HF-21 — delivery reaching a working Claude Code session

The recipient ran four separate `sleep 10` tool calls in one turn:

| Call | Start | End |
| --- | --- | --- |
| 1 | 00:11:18.365 | 00:11:28.368 |
| 2 | 00:11:30.705 | 00:11:41.826 |
| 3 | 00:11:44.414 | 00:11:54.419 |
| 4 | 00:11:57.481 | 00:12:07.484 |

The sender logged `delivered to write-claude via socket` at 00:11:47.720 — the middle
of the third call. The notification entered the recipient's context after the third
result and before the fourth call, as its own `task-notification` block rather than
appended to the tool result:

    Rewake: main-claude notify, 1 new message
      ↳ Метка MIDTURN-PROBE-21. Письмо отправлено посреди твоей сер…

The series was not interrupted, the fourth call went out normally, and the original
task reported its own result — the closing criterion for this row. The wrapper text
around a mid-turn message differs from the between-turns form: it says the peer "sent
a message while you were working".

## The ordinary path, as a separate observation

Before the probes, at 00:08–00:09, the same task went to all three sessions asking for
name, role and harness version. The sender's output was `delivered to write-claude via
socket`, `delivered to review-claude via socket` and `delivered to luna-codex via
app-server`, and all three returned a `finished` report. That establishes launch with
registration, task delivery, a report back and — for the Codex session — telemetry
collection, on Codex CLI 0.155.1 and Claude Code 2.1.270. It establishes nothing
beyond that ordinary path: no other row of the feature map was re-verified on 0.155.1.

## What these probes do not establish

The limits matter more than the successes, because each success is a single observed
run:

- **Group formation in other orderings.** Only one grouping was observed, from two
  messages 0.27 ms apart. What the notice looks like when members arrive in separate
  turns was not observed, and neither was a gap close to the 150 ms boundary.
- **Where the preview rule comes from.** The preview matched the last member once.
  Whether the notice selects the latest member by rule or simply overwrites as members
  arrive is not distinguishable from one observation; the contract says the former.
- **Mid-turn transport timing.** From the application side only the boundary between
  tool calls is visible. Whether the harness delivered the message into the stream
  immediately or held it until the tool finished was not observed, so `priority:
  "next"` is confirmed to arrive mid-turn but its in-flight handling is not described.
- **Telemetry for Claude Code sessions.** HF-12 proves the reader, not the source. A
  Claude Code session still publishes no telemetry of its own (HF-11), and nothing
  here changes that.
- **Repetition and races.** One run each, in one room, with both endpoints healthy.
  No recovery, restart, compaction or concurrent-sender case was exercised, and no
  regression was repeated under shuffle or race conditions at the live level.
- **Build identity.** Commit 4cfd3cf is cited from the install record; no per-process
  executable hash was taken, unlike earlier acceptance rounds.
