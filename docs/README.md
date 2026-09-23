# The documentation, mapped

Every document in this directory, grouped by what a reader wants from it, with what it
holds and when to open it. A subdirectory has an index of its own, linked here rather
than repeated: today that is the roadmap. `map_test.go` beside this file fails when a
document is on neither this map nor its directory's index, when an index is not linked
here, or when any link in `docs/` names a missing file or a heading its target does not
have. A change that adds, removes or renames a document, or changes what one is for,
updates this file in the same commit.

The order to start a session in is in `AGENTS.md`; this file is where to look for
everything else.

## How it works

- [flow.md](flow.md) — the whole path of one message as a story: a session starts, a
  message is sent, announced, read and answered, the report comes back, the session
  ends; with the files each step leaves and the exit codes and stalls a reader meets.
  Open it first, before any of the documents below, which it strings together.
- [design.md](design.md) — the specification: scope, the process model with no daemon,
  the state directory, rooms, the session record, roles and names (the one place their
  rules live), the environment a harness receives, the CLI contract and exit codes, the
  code policy, the testing layers, and dated owner decisions. Open it for why the system
  has the shape it has, or for a canonical rule.
- [code.md](code.md) — the source tree, package by package, with the harness interface
  every adapter implements. Open it to find where something lives, or before adding a
  package.
- [launch.md](launch.md) — starting a harness: the wrapper's launch sequence, signals,
  model and effort defaults from flags, environment and settings files, launch aliases,
  which flags choose a room and a role (the rules themselves are in design.md), the
  Claude Code flags and the Codex owned server with its terminal gateway, and the
  briefing a session is given. Open it when a launch flag, an alias or a startup detail
  is in question.
- [delivery.md](delivery.md) — sending, reading and reporting at the level of files and
  wire: the message record, task, question and notify, the question reservation and its
  heartbeat, the one lock per mailbox, the wrapper's servicing loop and notice texts,
  `rewake inbox`, turn-end reports and what happens on a thread change. Open it for the
  exact behaviour of any send, read or report.
- [delivery-owed.md](delivery-owed.md) — owed reports read back: `rewake inbox --owed`,
  what a session has read and still owes, and `rewake inbox --awaited`, what a run sent
  and still waits on, with the table of states and when a missing wait record means
  answered. Open it after a context compaction or when a listed state is in question.
- [delivery-adapters.md](delivery-adapters.md) — how a notice reaches each harness: the
  Claude Code socket line, its reply socket and receipts, held and late words, the startup
  gate and what a killed wrapper leaves behind; and the Codex gateway in brief. Open it
  when a delivery result for one harness is in question.
- [turn-outcomes.md](turn-outcomes.md) — the turn ends that are not an ordinary report:
  a failed turn, a keyboard stop, and a turn end marked with `rewake pending`, which tells
  the waiters the work is still going and keeps their tasks owed; including the rule that
  a failed or interrupted turn is woken only by new mail. Open it when a report is an
  error, stopped or pending, or when one of them settled or left open the wrong thing.
- [gateway.md](gateway.md) — the Codex delivery gateway from the inside: how the primary
  terminal connection and its conversation are selected and reserved before mail goes
  in, the ledger of admitted work, report publication, forks and side conversations, and
  the regression and acceptance coverage. Open it only when working on the gateway; it
  is dense with evidence, not an overview.
- [inbox-groups.md](inbox-groups.md) — the contract for grouped incoming mail: the
  session table's columns, `--peek` and `--message <id>`, the 150 ms window that fixes a
  group's members, start-or-steer dispatch without waiting for a finished turn, and the
  group digest the notice carries. Open it when grouping, the table or selected reads
  are in question.
- [native-mailbox.md](native-mailbox.md) — how the Codex adapter delivers a notice as the
  output of a `rewake_mailbox_notice` tool call in a turn start: its JSON, the briefing
  text that explains it, and the workspace-write permission it needs. Open it when a
  Codex notice arrives wrong or not at all.
- [native-mailbox-ui.md](native-mailbox-ui.md) — the arrival row a Codex terminal shows
  (`rewake notice --display-only`): where it is inserted, why it bypasses admission and
  telemetry, that it is not persisted, and its installed acceptance. Open it for what the
  person at a Codex terminal sees when mail arrives.
- [report-publication.md](report-publication.md) — how a finished turn's report is
  captured and published: the completion handler's capture and publish steps, the shared
  sequence word a reader snapshots without a lock, receipt keys that tell a stop from a
  finish or an error, and the single shutdown drain budget. Open it when a report is
  lost, doubled or late.
- [git-grants.md](git-grants.md) — `--grant-git`: who may attach it, to whom, and how the
  Codex delivery reservation appends the missing Git metadata roots without touching the
  sandbox policy; persistence through queueing and the refusals. Open it for anything
  about giving a session write access to Git metadata.
- [session-state.md](session-state.md) — the telemetry line main sees for its workers
  (activity, context used, compactions): what is shown to whom, the availability
  notifications main receives when a worker becomes ready, the context formula, how
  model and effort are confirmed, the compaction counter, snapshot staleness, with its
  acceptance log. Open it when `rewake list`, a header or an availability notice shows
  a wrong or stale value.
- [claude-telemetry.md](claude-telemetry.md) — how a Claude Code session's telemetry is
  collected: the background hooks, the status-line tap that runs the person's own status
  line in its place, the settings layers it resolves at each call, the collector in the
  wrapper, and the owner decisions that bound it. Open it when a Claude Code row of
  `rewake list` is wrong or unknown, or when a person's status line misbehaves under
  rewake.
- [session-activity.md](session-activity.md) — extends session-state: the activity labels
  and how fresh they must be, compaction notices, and how a worker's departure or
  replacement is detected and announced to main, with owner-run acceptance. Open it when
  a label, a compaction notice or a departure notice misbehaves.
- [install.md](install.md) — building and installing rewake locally with an atomic
  replace, and a dry run of the npm packaging with `scripts/pack.sh`. Open it after a
  change you want to run as the installed binary.

## Facts about the harnesses

- [research.md](research.md) — what only a running Claude Code session shows: the socket
  line and its priority, waking an idle session, what the inbound gate did to rewake's
  line at startup and in each permission mode and the receipts it sends to a reply
  socket, the Stop and StopFailure hook payloads rewake reads, what the status line and
  hooks hand a command about model, context and compactions, and why a legacy notify
  cannot be trusted as an error signal. Open it before touching delivery or wake
  behaviour, before changing the Claude Code collector, and after a harness update. Its companions follow.
- [research-claude-control.md](research-claude-control.md) — what reaches a running Claude
  Code session from outside besides a message, split from research.md by subject: what an
  Esc or a Ctrl+C leaves for rewake to hear (nothing short of the transcript), what `/clear`
  and `/resume` do, why no slash command runs from the inbound socket, and how text can be
  put into the input box by the launch flag or a plugin. Open it before working on
  `stopped` for Claude Code, on conversation tracking, or on sending a session a command.
- [research-codex.md](research-codex.md) — what only a running Codex session shows,
  split from research.md by subject: `codex queue`, thread identity and terminal events,
  the sandbox as a running session meets it, environment and instructions, and the
  session-owned app-server. Open it before touching the Codex adapter or after a Codex
  update.
- [research-launch.md](research-launch.md) — what an installed binary answers when run:
  model and effort catalogues and the usable context window, which flags may repeat,
  undocumented aliases, when a `--help` probe can be trusted, how each harness is
  published on npm, whether Claude Code has a client-server split to stand between, a
  prompt draft at launch,
  the hook options and settings order the launch layer relies on, and what the bundled
  source states about Claude Code's cross-session inbound gate — `crossSessionInbound`,
  permission-mode classes, holds, deadlines and receipts. Open it when adding a launch flag or when a model, an effort or a
  version is refused.
- [research-protocol.md](research-protocol.md) — what the generated Codex schema and the
  reference source state: the flags schema generation needs, required fields of the
  types the adapter uses, how start-or-steer forks, where `canAcceptDirectInput`
  lives, and what compaction and the terminal's other commands send over the protocol. Open it when the adapter or the fixture has to match a protocol change.
- [research-permissions.md](research-permissions.md) — the Codex sandbox: network and
  filesystem limits on Linux, why `.git`, `.agents` and `.codex` are protected, how
  `--add-dir` grants access, worktree quirks, and a short account of remote resume
  refusing overrides. Open it when a Codex session cannot write where it should.
- [continuation-permissions.md](continuation-permissions.md) — the evidence behind remote
  resume and fork refusing permission overrides, the protocol calls ruled out for adding
  Git roots to an idle session, and the exact sequence `--grant-git` uses instead,
  including its race with a turn typed by hand. Open it when working on grants or on
  resume and fork.
- [harness-features.md](harness-features.md) — the capability matrix: one row per feature,
  one column per harness, each cell live, implemented, missing, researched or not
  applicable with dated evidence, and the ordered parity queue. Open it first for "does X
  work on harness Y today", and when choosing what to build next.
- [prior-art.md](prior-art.md) — how other tools deliver messages between agents
  (terminal keystrokes, brokers, a mailbox over MCP, session registries, idle detection),
  what is worth borrowing and what to avoid. Open it when a design choice needs its
  outside context.

## Testing

- [testing.md](testing.md) — the entry point: the tiers with what each proves and costs,
  the exact commands, checking a new harness version before updating, reading a summary
  and a red case's evidence, how to add a scenario, a control or a column, and the traps
  already paid for. Open it first for anything about tests; the documents below are the
  depth behind it.
- [check-runner.md](check-runner.md) — the requirements the workflow suite answers: one
  command without flooding an agent's context, the evidence tiers from pure Go to a
  person at a terminal, the output and evidence contract with its outcome names, isolation
  per tier, and the lessons of earlier false-green tests. Open it for the terms the suite
  uses and the contract it must keep.
- [check-runner-proposal.md](check-runner-proposal.md) — the decision that answered it:
  `go test` with a switch and shared helpers in `test/workflow`, the alternatives rejected
  and why, the harness as a parameter, and the conditions for the paid tier. Open it to
  learn why the suite is shaped as it is.
- [check-runner-scenarios.md](check-runner-scenarios.md) — the scenarios themselves: each
  one's invariant, observations and negative controls, links to how it was built, the
  deferred ones, and the scenario-by-harness matrix with Codex as the gate and Claude Code
  as the search column. Open it when adding a scenario or reading what one proves.
- [native-mailbox-check.md](native-mailbox-check.md) — a runbook for repeating the
  owner-facing check of Codex mailbox delivery: the environment, the permission it needs,
  the exact launch commands and the idle and active delivery script. Open it to run that
  check again by hand.
- [native-mailbox-ui-check.md](native-mailbox-ui-check.md) — what the deleted fixture for
  the arrival-row check did — isolation, a scripted model endpoint, permission defaults,
  the three phases a person judged by eye — kept as history, not as a request to rerun.
  Open it when rebuilding that check on the suite.

## Records of acceptance and observation

Dated records of what was seen. Their observations are never edited afterwards; a dated
note ahead of them may say what has changed since, and where one has been superseded,
the newer document says so.

- [claude-parity-2026-09-21.md](claude-parity-2026-09-21.md) — three parity rows closed
  with Claude Code in the roles they waited on: telemetry of a Codex worker read by a
  Claude Code main, grouped arrival and selected reads on a Claude Code recipient, and
  mid-turn delivery to a working Claude Code session; with what the probes do not show.
- [gateway-native-evidence.md](gateway-native-evidence.md) — the source citations for how
  the Codex terminal picks its primary conversation, and the owner's fork and side
  sequence and the installed primary and side delivery acceptance of September 19. Open
  it when the question is why rewake trusts a conversation as the terminal's own.
- [inbox-acceptance.md](inbox-acceptance.md) — acceptance of revision 6 of grouped
  inbox: the candidate's hashes, a startup smoke test and a live delivery to an active and
  an idle session. The contract it accepts is inbox-groups.md.
- [native-mailbox-acceptance.md](native-mailbox-acceptance.md) — acceptance of native
  mailbox delivery: the evidence levels from unprompted controls to the installed check,
  the read-only sandbox default that failed the first run, and what the deleted research
  package showed, recounted, with the main session's own report quoted as observed.
- [native-terminal-progress.md](native-terminal-progress.md) — the captured events and
  source lines behind two rules stated elsewhere: a failed or interrupted turn is woken
  only by new mail (delivery.md), and native start-or-steer does not wait for a finished
  turn (inbox-groups.md).
- [reviews-native-ui-2026-09-20.md](reviews-native-ui-2026-09-20.md) — the review and
  installed acceptance of the arrival row: two prototype rounds, its promotion into the
  adapter, the binary's hash, and the quoted observations, with the limit that the row is
  transient.
- [startup-transport.md](startup-transport.md) — the Codex transport's startup failures
  and their repair: a request queue that overflowed in startup bursts, a message size
  limit hit later, the sizes chosen and the live acceptance. Open it for why those limits
  are what they are.
- [server-observation.md](server-observation.md) — the observer approach to reading turn
  outcomes that preceded the gateway: subscription versus identity, a race after turn
  start and the completion grace. The gateway replaced it, as the file says.
- [thread-ownership-investigation.md](thread-ownership-investigation.md) — the
  investigation that established loaded-thread metadata cannot tell which conversation a
  terminal shows, the intent-gateway design later adopted, and the observer mitigation
  shipped before it. A status note at the top says R14-1 and R14-2, open in the text,
  are closed, and the observer is gone.
- [thread-lock-probes.md](thread-lock-probes.md) — owner-run probes that extend it: file
  descriptors and writer locks across `/new` and resume, binary provenance, and a resume
  matrix, concluding that a writer lock does not identify the selected conversation. Its
  status note records the same closure of R14-1 and R14-2.

## Plans and history

- [work-queue.md](work-queue.md) — what comes next, in the owner's order: the rest of the
  workflow suite, the arrival-row check on the suite, a named harness version against a
  local responder, a two-way channel for Claude Code, the parity queue, and what is
  queued without a date. Open it to pick the next piece of work.
- [roadmap/README.md](roadmap/README.md) — what is done: the index of the roadmap, one
  file per closed milestone, review round or piece of work, named
  `YYYY-MM-DD-<subject>.md` by the day it closed, plus `risks.md` for the risks table and
  `later.md` for what is deferred without a date. The index has one line per entry and
  says what is open; it is the map of that directory.
- [reviews.md](reviews.md) — the first ten review rounds, all of September 16, 2026: each
  defect found and its fix, ending with the decision on Git metadata access by role.
- [reviews-later.md](reviews-later.md) — the review rounds and repair chains from
  September 17 to 20: expanded checks, Git write grants, rounds 11 to 14, the gateway,
  report publication and startup repairs, session state and activity, grouped inbox.
- [transport-milestones-2026-09-17.md](transport-milestones-2026-09-17.md) — the owned
  server transport milestone with its live acceptance table, and the entries of the same
  day on fresh threads, continuation, Git grants (marked superseded), lost reports,
  naming and explicit main.

## When something behaves unexpectedly

- [traps.md](traps.md) — what behaves other than expected, by symptom: roles and reports,
  tests run next to a live session, arguments and continuation, delivery and what counts
  as proof. Open it when a failure is confusing, before assuming a new defect.
- [intermittent-bugs.md](intermittent-bugs.md) — failures whose trigger is not fully
  understood, with timelines and what to capture next time. Open it when a delivery
  failure resembles an old one.
