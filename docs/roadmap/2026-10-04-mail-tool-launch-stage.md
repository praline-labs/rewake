# Stage M3 of the mail tool: the launch injection, built and its code accepted

The third of the three stages of [mail-bridge.md](../mail-bridge.md) — giving the
harness the mail tool's server at launch, and keeping the run's mail channel — was built
on October 4, 2026 against rules accepted after five design passes
([mail-bridge-launch.md](../mail-bridge-launch.md),
[mail-bridge-launch-codex.md](../mail-bridge-launch-codex.md),
[mail-bridge-channel.md](../mail-bridge-channel.md)). Its code was accepted the same day
in the fourth Codex-side round. **The stage is not closed**: the live checks of
[mail-bridge-live.md](../mail-bridge-live.md) are open, and until gates G2 and G7 close a
launch gives neither harness the tool and says why.

## Five design passes

The rules came before the code, as for stage 2. The first pass (October 1) wrote the
injection, the name check by source, the safe diagnostics and the gates; the review
found the scheme sound and asked for nine corrections. The second split the run's mail
channel into a document of its own; the third took the owner's decisions below and moved
the live plan into one more; the fourth and fifth narrowed step 0 of the Codex thread
check and G9 — the terminal's keys are a fact, the safety of their values is not, and the
trust Codex grants by itself is a rule with a refusal of its own. The fifth review
accepted the rules on October 4, 2026; open gates by themselves did not stand in the way
of that, only of the stage.

## What main and the owner decided on October 4, 2026

- **No shell after a denial**: a call the harness denied for permission is a defined
  outcome; main is told once, the worker is given no shell advice, and nothing is built
  to detect or probe it beyond that (the owner's decision).
- **`--no-mail-tool`** is rewake's launch flag, the person's way past a name check they
  cannot satisfy at once; rewake never takes it on its own (the owner's decision).
- **The live checks run in the owner's own logged-in harnesses**, in scratch project
  folders, with the user configuration read only (the owner's decision).
- **Gates**: G2 open on Codex and G7 open on Claude Code keep the tool out with a note
  naming the gate, never a refusal; until G5 closes, a parent directory's `.mcp.json`
  naming `rewake` refuses, and one that cannot be read refuses as a failed check.
- **`REWAKE_GATES_ASSUMED`** takes named gates as closed, for the live checks only; it is
  said on stderr and kept in the session record.
- **The run's end** is only what the wrapper knows: the harness's exit, an accepted
  SIGTERM or SIGHUP, and its own teardown before ending the child; no EOF and no
  `SessionEnd` are taken for an end.
- **A repeated denial** moves the block to the latest one by event time; only a ticket
  issued after it clears it.

## What was built

- **The gates** (`internal/harness/gates.go`): the table, each launch's open gates, and
  `REWAKE_GATES_ASSUMED` for the live checks, said on stderr and kept in the session
  record (main's decision of October 4, 2026).
- **The name check before the claim**: on Claude Code `mcp get rewake` under a bounded,
  reaped run, every `--mcp-config` value and every parent's `.mcp.json` (G5's interim,
  main's decision of the same day); on Codex a separate app-server's `config/read` and
  requirements, judged layer by layer. Diagnostics from a closed list
  (`internal/harness/check_diagnostic.go`).
- **The injection**: Codex leaves after every `-c` of the caller's, `omit_tools_from`
  on our server; on Claude Code a 0600 server file, an allowance of our tool and the
  hooks in rewake's one settings layer. `--no-mail-tool` in the launch flags.
- **The gateway's check at every thread**: step 0 over the terminal's fourteen keys,
  the trust rule, refusals of what G3 and G9 leave unknown.
- **The per-call limits on Claude Code** (`internal/bridge/endpoint/limits.go`): the
  deadline from the hook's timeout, and no read acknowledged until G8 closes.
- **The channel record** (`internal/channel`, the keeper in `internal/wrap/channel.go`):
  two observations and a block, notices with their windows and publication identity,
  the display in `whoami`, `rewake list` and main's header, the shell's observations
  through `receipts/<epoch>/channel/`, and the briefing's transport sentence.
- **`tools/standin`**: a stand-in API answering with a call of the tool, for G8b.

## How it was checked

Generated spaces, each against an oracle written from the rules: 34,560 Claude Code
launch lines through the name check (`mcp get` asked in 1,152), sixteen answers of a
fake `mcp get` (killed, hanging, ignoring TERM, leaving a server behind), eight parent
layouts; 37,467 written timeouts and 1,352 limit pairs; 33,684 Codex layer sequences,
338,912 `-c` lists, ten modes of a fake preflight server, 393,216 key sets through step
0, 1,080 entries the server may hold and 512 thread requests through the check;
981,640 channel event sequences, its eighteen failure points and 19,448 landing
histories (the channel's counts at the first build; the rounds below give the current
ones) for the notice windows; 1,171 shell-observation directories and forty shell
calls across channel records. The launch adds nothing else, compared byte for byte on a
temporary HOME, configuration home and launch directory.

Forty-one mutants of the main guarantees: thirty-nine killed, two equivalent. One
redundant check found that way was removed (a digits test that base-10 parsing already
makes); five survivors became tests that kill them (a caller's value on one of our keys,
both notice windows at their edges, G8 deciding the endpoint, a timer event before its
time).

Defects found by the spaces on the way: a `Scope:` line read by any word rather than its
first, a launch that checked for injection where the settings merge would leave the tool
out, a test frame reader that misread a 16-bit length of 127, and a `send` accepted but
not delivered that the shell observation did not count.

## Review round 1

The Codex-side acceptance did not accept the build: seven defects confirmed by probes,
and oracles that repeated the code under test. Fixed the same day:

- An explicit empty `cwd` is named — the server resolves it against its own directory —
  so it falls under G9 and the trust rule; before, it passed as no cwd at all.
- A check runs in a session of its own and is reaped by that session: a descendant that
  moved to another group survived `End()`.
- An empty table naming `rewake` in another layer refuses; flattening had dropped it.
- The keeper holds what arrives while a close waits and folds by event time, a close
  only once a heartbeat old; the interval starts at the earliest failure and the class
  follows the latest, whatever order they arrive in.
- One notice in flight per recipient, and whether a notice may still be written is
  decided under the mailbox lock: one planned before the end is not written after it.
- The shell observation carries the time of its write, taken where the write happens.
- The block is the latest denial, moved by event time; only a ticket issued after it
  clears it (main's decision of October 4, 2026).
- The run's end is what the wrapper knows: the harness's exit, an accepted SIGTERM or
  SIGHUP, and its own teardown before ending the child (main's decision of the same
  day). No EOF and no `SessionEnd` are taken for an end.
- A launch reads its harness's version when `closedGates` names one for it, so a gate
  closed per version takes effect.
- The oracles: step 0's keys come from the rules' text, the thread space resolves an
  empty cwd by path rather than by the production condition, the channel space carries
  the notices and the landing of each (1,627,600 sequences), and a new space folds every
  arrival order against event time (3,240 orders).

## Review round 2

The second Codex-side acceptance took the fixes of round 1 and confirmed four defects,
two of them the limitations the build had left open. Fixed the same day:

- A check's end killed an unrelated child of the wrapper living in a session of its own:
  the wrapper, as subreaper, took any child in another session for an adopted one. A
  check now runs under a holder of its own that starts nothing else and is the
  subreaper of the check's tree; the wrapper is no subreaper, as the grants decision
  already required ([grants](../grants.md#what-the-owner-decided)).
- A denial held behind a waiting close let shell advice be written: denials are no
  longer held.
- A ticket folded late moved the interval to the latest failure and lost a shell success
  after the first one; the record now keeps the interval's failures.
- A ticket folded after a later close on Claude Code showed working and told main of a
  recovery; it now leaves the tool as the close left it. The order space compares the
  tool state in every order, and the sequence space checks the interval against the
  path's own tickets and failures, without the exceptions they carried.

## Review round 3

The third acceptance took round 2's fixes and found the transport's history still
depending on arrival: a startup failure folded after a later hello vanished, a failure
folded after a later hello erased it, and a hello of one connection told after another
connection's close left a false "server gone". The endpoint serves each connection on
its own goroutine and the gateway reads startup statuses on its own, so no single
ordered stream of events exists to rely on. Fixed the same day:

- The record keeps the transport's history — connections by generation, the other
  transport events since the last ticket — and derives the tool observation from it by
  event time on every fold, with a fixed rank for a tie. Liveness is per connection; a
  close of one is a "server gone" only when no other is live at its time, whichever
  generation is older. A ticket stops the hello timer, and the timer passes at its end
  once a later event shows it.
- The keeper holds only a close for its heartbeat; nothing else waits behind it.
- The order space permutes four events at a time with hellos, closes of two
  connections, startup failures and the timer among them (2,649,876 orders); the
  sequence space folds every path again by event time and in reverse (2,549,680
  sequences); an endpoint test holds one connection's hello, or a startup status,
  behind another connection's events.

## Review round 4

The fourth Codex-side acceptance accepted the code on October 4, 2026: round 3's blocker
closed, no new defect in the changed paths. Its probes fold eight events of two
connections across a ticket in all 80,640 orders on both harnesses against states
written by hand, test the timer's end to the nanosecond and a ticket stopping it, and a
late hello undoing conditional failures until the last close. It accepted the changes of
behaviour round 3 brought: the last of two closes is "server gone" whichever generation
is older, "no hello observed" is stamped at the timer's end, a ticket stops the timer,
and the run's exit stays the one place where arrival order decides.

The reviewers' probes of all four rounds were then moved into the project's tests, so
none is covered only from outside the tree: round 4's into
`internal/channel/connections_test.go`, round 1's repeated denial into the failure-point
table. The rest duplicated a test already there and were not moved: round 1's close
folded after a later failure (the order space and the keeper's
`TestACloseFoldedAfterLaterFailuresLandsWhereItHappened`), empty `cwd` and empty table
(`TestEveryThreadRequestThroughTheCheck`, `TestTheStepsOverEveryEntryTheServerMayHold`),
the shell evidence's time (`TestTheShellEvidenceKeepsTheTimeOfItsWrite`), the descendant
in another group (`TestWhatACheckLeftInAnotherGroupOfItsSessionIsEnded`), the notice
bounds and the notice after the exit (`TestNoticesWaitForTheOneInFlight`,
`TestANoticeFixedBeforeTheExitIsNotWrittenAfterIt`); round 2's late tickets
(`internal/channel/late_test.go`), the held denial
(`TestADenialToldWhileACloseWaitsStopsAdviceAtOnce`) and the child in its own session
(`TestACheckLeavesOtherChildrenAlone`); round 3's startup failure and hello told late
(the failure-point table and the order space) and its endpoint probe
(`TestAHelloDeliveredLateKeepsItsConnectionLive`).

## Open

- The live checks ran on October 4, 2026
  ([the live checks](2026-10-04-mail-tool-live-checks.md)): G1, G5 and G7 closed, S1
  passed with a corrected form; what stays open, and four findings that need code, are
  listed there. The stage closes when they are settled.
- The record's transport history has no upper bound: one entry per connection of the run
  and per transport event since the last ticket.
