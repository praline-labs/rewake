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

- [flow.md](flow.md) — the whole path of one message as a story, Acts 1 through 5: a
  session starts, a task is sent, the wrapper announces it, the agent reads it and the
  turn ends with the report going back; with the files each step leaves. Open it first,
  before any of the documents below, which it strings together.
- [flow-endings.md](flow-endings.md) — the rest of the flow, split from flow.md by
  subject: a blocking question (Act 6), a notify (Act 7), the session's end (Act 8), one
  exchange traced through its files, where the flow can stall and what the sender sees,
  and how a report notes that the reader's conversation changed underneath it. Open it
  for a question, a notify, a session's end, or a stall a reader met.
- [design.md](design.md) — the specification: scope, the process model with no daemon,
  the state directory, rooms, the environment a harness receives,
  the CLI contract and exit codes, the code policy, the testing layers, and dated owner
  decisions. Open it for why the system has the shape it has, or for a canonical rule.
- [session-record.md](session-record.md) — the session record, split from design.md by
  subject: every field, the boot and build stamp that tell a run of this build from an
  earlier one, and how a record is published, updated and pruned. Open it when reading or
  changing what a session record says.
- [roles.md](roles.md) — roles and names, split from design.md by subject and the one
  place their rules live: the role catalogue and what each role reports and may be
  granted, what each role is told and the owner's rules behind it, how the role
  is chosen under the room lock, why main is silent, and how a name is built from role,
  prefix and harness. Open it when a launch picks the wrong
  role or name.
- [code.md](code.md) — the source tree, package by package, with the harness interface
  every adapter implements. Open it to find where something lives, or before adding a
  package.
- [legacy.md](legacy.md) — the mark on code kept for an old harness version or for
  records an earlier rewake wrote: its form, the search, when a mark is due, the order of
  clearing one, the oldest supported versions and the table of current marks that
  `legacy_test.go` keeps equal to the code. Open it before keeping something for an old
  version, or when raising the oldest supported one.
- [launch.md](launch.md) — starting a harness: the wrapper's launch sequence, signals
  and following a stopped harness, which flags choose a room and a role (the rules
  themselves are in design.md and roles.md), the
  Claude Code flags and the Codex owned server with its terminal gateway, how the main
  session gives each writer a worktree of its own, and the briefing a session is given. Open it when a launch flag or a startup detail is in
  question.
- [worktree.md](worktree.md) — the checkout `--worktree` gives a Codex or Claude Code
  launch: its branch, where it lives, where the launch starts, the continuations and
  names refused, how git runs, what `.worktreeinclude` copies, and what `rewake worktree
  land`, `finish`, `ls` and `rm` do and ask first. Open it when touching
  `internal/worktree`, the worktree command or a harness's worktree flag.
- [launch-defaults.md](launch-defaults.md) — what a launch gets that nobody typed:
  model and effort defaults from flags, environment and settings files, and launch
  aliases with the rules for which of their flags a typed one replaces. Open it when a
  session came up on an unexpected model or an alias does something other than meant.
- [delivery.md](delivery.md) — sending and reading at the level of files and wire: the
  message record, task, question and notify, the question reservation and its heartbeat,
  the one lock per mailbox, the wrapper's servicing loop and notice texts, and `rewake
  inbox`. Open it for the exact behaviour of any send or read.
- [delivery-turn-end.md](delivery-turn-end.md) — what happens once the reader's turn
  ends, split from delivery.md by subject: how `turn-ended` turns a finished turn into a
  report, the report's derived id and what a retry replays, and how a report notes that
  the reader's conversation changed underneath it. Open it for the exact behaviour of a
  turn-end report.
- [delivery-owed.md](delivery-owed.md) — owed reports read back: `rewake inbox --owed`,
  what a session has read and still owes and how many tasks wait unread, and
  `rewake inbox --awaited`, what a run sent
  and still waits on, with the table of states and when a missing wait record means
  answered. Open it after a context compaction or when a listed state is in question.
- [delivery-sent.md](delivery-sent.md) — what a sender can still do with a message it
  sent: `rewake withdraw` while unread, with the tombstone and why withdrawn is final
  like read, `rewake edit` as a withdrawal plus a new letter, and `rewake send --to` for
  an addendum; who may act and which id forms are taken. Open it when a withdrawn,
  replaced or added-to message is in question.
- [delivery-adapters.md](delivery-adapters.md) — how a notice reaches each harness: the
  Claude Code socket line, its reply socket and receipts, held and late words, the startup
  gate and what a killed wrapper leaves behind; and the Codex gateway in brief. Open it
  when a delivery result for one harness is in question.
- [mail-bridge.md](mail-bridge.md) — design, all three stages built, stage 3's live checks open: one MCP tool carrying CLI words
  outside the sandbox, its allowed operations, native call identity, per-launch approval
  and configuration preservation, refusal on an occupied name, bounded reads, receipts
  and fallback; the two live probes and remaining acceptance gates. Open it before
  building the bridge or when judging its read and retry boundaries.
- [mail-bridge-cli.md](mail-bridge-cli.md) — the CLI side of that tool as built: bridge
  mode and its ticket, bounded parts, reads frozen and marked only on the wrapper's
  evidence, claims against withdrawal, the receipt journal behind notify and pending,
  `inbox --next` and `rewake retry` in either channel, and the readings of the
  specification it chose. Open it when a read in parts, a receipt or a retry is in question.
- [mail-bridge-server.md](mail-bridge-server.md) — the server of that tool, built and
  started by a launch only once stage 3's gates allow: its eleven rules and where each lives in the code, how a
  call is matched to a native observation and given a ticket, the call's binding to its
  operation, the child it runs, the bounded answer, the failure points of a call and the
  channel record left to stage 3. Open it before changing the server or the context
  endpoint.
- [mail-bridge-turns.md](mail-bridge-turns.md) — the same design after the answer: which
  turn a call belongs to, reads acknowledged on the call's own answer, a turn's end that
  waits for no call, a pending mark that meets its turn's end, every wait's bound, and
  the failure points after the answer. Open it before changing the observer or an adapter.
- [mail-bridge-checks.md](mail-bridge-checks.md) — building and checking that design: what
  it changes in stage 1, the tests without a live harness and how they are built (the
  fault test from logged steps, the generated orders, the fault build's holds), the live
  checks left to stage 3, and what the reviews of the rules found. Open it before
  changing those tests or reviewing the stage.
- [mail-bridge-launch.md](mail-bridge-launch.md) — stage 3 of that design, built, live
  checks open:
  how each harness's launch injects the server and approves only its tool, which sources
  of a server named `rewake` each check covers before the run is published and what is
  done where coverage is unknown, what a diagnostic may show, how the person's
  configuration is proven untouched, the gates with the action each takes while open, and
  `REWAKE_GATES_ASSUMED` for the live checks. Open it before changing the injection.
- [mail-bridge-launch-codex.md](mail-bridge-launch-codex.md) — the Codex part of stage 3:
  the `-c` values the launch adds, the name check by a separate app-server before the
  claim, the injection check at start and at every thread with its step 0 for the thread
  request itself (the terminal's keys, the trust rule), and the output limit. Open it
  before changing the Codex injection or the gateway's thread check.
- [mail-bridge-version.md](mail-bridge-version.md) — the harness version a launch takes
  for the closed gates and the L5 bounds: when it is taken, from where without running
  anything extra (Codex's one `--version` read, Claude Code's install path), what an
  unknown one means, and a read whose cleanup failed; built, with how the code reads
  it. Open it before changing `gates_version.go` or how a launch chooses the tool.
- [mail-bridge-channel.md](mail-bridge-channel.md) — the rest of stage 3, built: the
  run's mail channel as two observations, tool and shell, and a policy block, with
  connections, generations, the failure interval and the hello timer, the derived
  display, the notices with their suppression and publication identity, and the
  briefing's sentence, and how the code reads it. Open it before changing the channel
  record (`internal/channel`) or its notices.
- [mail-bridge-channel-codex.md](mail-bridge-channel-codex.md) — the channel's
  conversations on Codex, where every thread runs its own server: which connections
  serve the conversation, how a call's `_meta.threadId` binds one, and how selecting
  another conversation moves the timer, held events and an open interval; built, with
  the choices the code made. Open it before changing how the endpoint, the gateway's
  selection steps or the keeper count Codex connections.
- [mail-bridge-channel-failures.md](mail-bridge-channel-failures.md) — the channel's
  failure points, one row each, with what is proven, what is unknown and what rewake
  does, including the sequences of Codex's conversation connections. Open it when
  writing or checking a channel test against the text.
- [mail-bridge-live.md](mail-bridge-live.md) — the plan of stage 3's live checks: scratch
  project folders in the owner's own logins, where each case sets its conditions, how
  preservation is proven with the user configuration read-only, what only the user layer
  could show, and the cases with the gates they close. Open it before running them.
- [turn-end-recovery.md](turn-end-recovery.md) — how a turn end is recovered under rules 7
  and 8 of mail-bridge-cli.md, as the code keeps them: the operation's identity and
  scope by event form, the read clock for holds, pending marks and the interim record,
  the evidence for each effect, the reconciliation order, the stop on an unknown and
  `rewake settle`, and the table of every record and state. Open it before changing the
  turn end's recovery.
- [turn-end-recovery-findings.md](turn-end-recovery-findings.md) — what the acceptances
  and reviews of the turn-end recovery found, each with the clause that closes it, and
  what the probes of the earlier acceptances expect now. Open it when a finding comes
  back, or before reading an old probe's verdict.
- [mailbox-records.md](mailbox-records.md) — every kind of file a mailbox holds, as the
  list in code names them; the reading of the whole mailbox and of every effect's marks
  before any effect, the test that keeps the list complete, the stop on record and how
  it goes, and the notes to main a journal owes. Open it before a writer adds a path
  under a mailbox.
- [protocol-cutover.md](protocol-cutover.md) — how a mailbox passes from an earlier
  build's protocol to that one, as the code does it: the launch order, the states of
  the successor, run records named by boot and epoch, the upgrade the automatic cutover
  is bounded by and the look for earlier-build writers within it, and what this build
  refuses or holds for a run of the earlier build. Open it before changing a launch or
  anything that tells builds apart.
- [delivery-conversation.md](delivery-conversation.md) — why a Codex message stays
  pending in a conversation the launch did not ask for: the launch's intent, `rewake
  accept`, the record that keeps the worker's inbox closed meanwhile, and how the sender,
  main and the person at the terminal are told; and why a worker whose sandbox closes
  rewake's state directory is not handled there yet; and, for either harness, how a
  resumed run takes over the waits of the earlier runs in its conversation. Open it when a
  Codex delivery waits after a resume, a worker cannot read its mail, or a resumed run's
  report does not settle what an earlier run read.
- [turn-outcomes.md](turn-outcomes.md) — the turn ends that are not an ordinary report:
  a failed turn, a keyboard stop, and a turn end marked with `rewake pending`, which tells
  the waiters the work is still going and keeps their tasks owed, with the confirmation
  Claude Code's Stop hook asks once after it when a later turn ends unmarked; including the rule that
  a failed or interrupted turn is woken only by new mail. Open it when a report is an
  error, stopped or pending, or when one of them settled or left open the wrong thing.
- [gateway.md](gateway.md) — the Codex delivery gateway from the inside: how the primary
  terminal connection and its conversation are selected and reserved before mail goes
  in, the ledger of admitted work, report publication, forks and side conversations, and
  the regression and acceptance coverage. Open it only when working on the gateway; it
  is dense with evidence, not an overview.
- [inbox-groups.md](inbox-groups.md) — the contract for grouped incoming mail: the
  session table's columns, `--peek` and `--message <id>`, the 150 ms window that fixes a
  group's members — and, since September 25, the longer wait of notifies and reports for
  company, detailed in delivery.md — start-or-steer dispatch without waiting for a finished turn, and the
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
- [grants.md](grants.md) — `--grant-dir` and `--grant-dir-broad`: the owner's decisions,
  the hard and broad tiers a directory is checked against at send and again at delivery,
  the idle wait, how Codex adds the directory to its roots, the journal and how a grant is
  taken back after the report, and how long a grant lives, with a short account of who
  can grant and of Claude Code. Open it first for anything about giving a session write
  access to a directory outside its workspace.
- [grants-authority.md](grants-authority.md) — who can grant: main's wrapper registering
  a grant from below itself, holding it while its task is open and confirming it at
  delivery by pid, start time and namespaces, the conversation it keeps for a resume, and
  what the scheme holds against and what not. Open it when a grant is refused as not
  confirmed, or to judge whether a worker could grant itself one.
- [grants-claude.md](grants-claude.md) — a grant to a Claude Code session through its
  permission hooks: the keeper in the worker's wrapper, the idle wait, giving on the first
  file-tool write, what stays with the person, taking back and the modes it works in, and
  why it is a courtesy rather than a boundary. Open it when a Claude Code worker is asked,
  or not asked, about a write in a granted directory.
- [grants-resume.md](grants-resume.md) — a grant after a cold resume: finding the
  conversation and the task's wait, the journal copy, main's wrapper confirming it again,
  what each harness does with the answer and what is not restored. Open it when a resumed
  session gains or lacks a granted directory.
- [permission-requests.md](permission-requests.md) — design, not built: a worker's
  permission request held while main answers with `rewake permission`, refused with the
  next step past rewake's limit; the message kind, races, security, the live probe and the
  owner's open choices, with its sources file by file in
  [permission-requests-sources.md](permission-requests-sources.md). Open it before
  building it or when a worker stalls on a prompt.
- [conversation-reset.md](conversation-reset.md) — design, not built: `rewake clear` from
  main and a conversation marker in `list`, notices and `--awaited`; what `/clear` and
  `/new` do on each harness, why Codex cannot be switched from outside, races, the probe
  and the owner's open choices. Open it before building either.
- [session-state.md](session-state.md) — the telemetry line main sees for its workers:
  who sees what, availability notifications, the context formula, confirmed model and
  effort, the compaction counter, staleness, and its acceptance log. Open it when
  `rewake list`, a header or an availability notice shows a wrong or stale value.
- [claude-telemetry.md](claude-telemetry.md) — how a Claude Code session's telemetry is
  collected: background hooks, the status-line tap running the person's own status line,
  the settings layers, the collector and the owner decisions. Open it when a Claude Code
  row of `rewake list` is wrong, or a status line misbehaves under rewake.
- [claude-plugin.md](claude-plugin.md) — rewake's function-hooks plugin for Claude Code:
  what the launch carries, the four events it reports and the fields it never reads, how
  an interrupted turn becomes `stopped` without a second report for an ordinary one, how
  a session says whether interruptions are heard, and its limits. Open it when an Esc on
  a Claude Code worker is not reported, is reported twice, or when the plugin API changes.
- [remote-control.md](remote-control.md) — `rewake compact` and `rewake interrupt`, a
  main acting on a running worker: who may call them and every refusal, the per-run
  control directory and its files, why giving up on a request is honest in every order,
  how the Claude Code module compacts and aborts, the stopped text and notice line after main's interrupt, known limits and the owner
  decisions. Open it when a compaction or
  an interrupt from main is refused, lost or misreported.
- [remote-control-letter.md](remote-control-letter.md) — the letter that ends a
  compaction main asked for: what it says, where its halves come from, the record
  main's own wrapper owes it by — closed by the outcome, the worker's departure or a
  bound — and a record an earlier run of main left. Open it when a letter is missing,
  late or wrong.
- [remote-control-tests.md](remote-control-tests.md) — the tests of `rewake compact` and
  `rewake interrupt`, file by file: the control protocol, the commands, the module under
  node, the collector, main's letters, the Codex gateway, and the workflow cases with
  their mutants. Open it to find what proves a part before changing it.
- [remote-control-codex.md](remote-control-codex.md) — how a Codex session serves those
  requests through its gateway: the three safety properties, when a compaction is
  refused as busy, uncertain or as nothing to compact, the open operations kept across
  reconnects, the mark that ties main's compaction to its turn and what happens to it
  when the request fails, the terminal's `/compact` answered while main's runs, and the
  interrupt naming main. Open it when a Codex compaction or interrupt from main
  misbehaves.
- [remote-control-codex-limits.md](remote-control-codex-limits.md) — the known limits of
  that service: a goal's turn compacting before main's compaction, a reply after its
  turn's end, a request left unanswered, a compaction lost sight of or ending unseen,
  work accepted and lost, a running turn whose id is unknown. Open it when main's
  compaction is refused as uncertain, or its answer or letter looks wrong.
- [codex-publication.md](codex-publication.md) — which Codex turn outcomes reach the
  waiters: proof of work, the advisory report of a turn without it or of a run that
  passed unseen, a compaction's turn reporting nothing, and why no outcome is dropped
  and no compaction settles a task. Open it when a Codex report is missing, advisory
  when it should settle, or settles when it should not.
- [session-activity.md](session-activity.md) — extends session-state: the activity labels
  and how fresh they must be, compaction notices, and how a worker's departure or
  replacement is detected and announced to main, with owner-run acceptance. Open it when
  a label, a compaction notice or a departure notice misbehaves.
- [install.md](install.md) — building and installing rewake locally with an atomic
  replace, a dry run of the npm packaging with `scripts/pack.sh`, and the release to npm
  through the gate `tools/release`. Open it after a change you want to run as the
  installed binary, and before a release.

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
  Esc or a Ctrl+C leaves for a hook or the wrapper to hear (nothing), what a function-hooks
  plugin hears instead, what the built-in plugins do under the function-hooks switch,
  mermaid among them, what a Stop hook's block does to a turn, what `/clear` and `/resume` do, why no slash command runs from the
  inbound socket, and how text can be put into the input box by the launch flag or a
  plugin. Open it before working on `stopped` for Claude Code, on conversation tracking,
  or on sending a session a command.
- [research-claude-actions.md](research-claude-actions.md) — what a function-hooks plugin
  can do to the Claude Code session it runs in, split from research-claude-control.md by
  subject: compact it, abort its turn, poll a file, swallow a socket line, reload, fill
  the harness's task list — with the forms and refusals of each, as the host words them —
  and what a Stop hook that blocks does to a turn's end: the second call, its payload,
  the limit of eight; and how a permission hook adds a working directory to a running
  session and takes it out. Open it before changing the module's side of `rewake compact`
  or `rewake interrupt`, before a Stop hook that holds a turn, before touching the grant
  hook, and after a harness update.
- [research-codex.md](research-codex.md) — what only a running Codex session shows,
  split from research.md by subject: `codex queue`, thread identity and terminal events,
  the sandbox as a running session meets it, why `--worktree` cannot go with a remote
  terminal, environment and instructions, the session-owned app-server, and the model's
  plan tool. Open it before touching the Codex adapter or after a Codex
  update.
- [research-codex-live-checks.md](research-codex-live-checks.md) — dated live probes of
  delivery and conversation selection, split from research-codex.md by subject: a
  compaction held against a task, two real sessions driven end to end through steering,
  `/new`, compaction and an interrupt, and the terminal's selection between two installed
  versions. Open it before touching delivery timing or conversation selection on Codex.
- [research-codex-conversation.md](research-codex-conversation.md) — source facts about a
  Codex conversation's permissions, split from research-codex.md by subject: what states
  them, how a settings update is queued, applied and told, what a running turn keeps, and
  the warning the terminal draws without a turn; read for a sandbox check that was
  dropped, and why the event stream cannot carry one. Open it before reasoning about a
  conversation's sandbox, the delivery hold, or after a Codex update.
- [research-mail-tool.md](research-mail-tool.md) — what each harness does with rewake's
  mail tool server, from the live checks of October 4, 2026, split from research.md and
  research-codex.md by subject: the `-c` trust form and the trust key, which calls start
  servers, a resume's cwd, results and output limits, timeouts, serial and parallel
  calls, sub-agents and nested agents and their hook fields, permission signals, a
  managed MCP file, server death and turn ends. Open it before touching the mail tool's
  launch, endpoint or channel, and after a harness update.
- [research-launch.md](research-launch.md) — what an installed binary answers when run:
  model and effort catalogues and the usable context window, which flags may repeat,
  undocumented aliases, when a `--help` probe can be trusted, how each harness is
  published on npm, whether Claude Code has a client-server split to stand between, a
  prompt draft at launch, where a Claude Code session launched with `-w` runs,
  the hook options and settings order the launch layer relies on, and what the bundled
  source states about Claude Code's cross-session inbound gate — `crossSessionInbound`,
  permission-mode classes, holds, deadlines and receipts. Open it when adding a launch flag or when a model, an effort or a
  version is refused.
- [research-worktree.md](research-worktree.md) — Claude Code's own worktree beside
  rewake's: the flags that continue a conversation and why a continuation leaves a new
  checkout, where trust is kept, `.worktreeinclude` as the binary and the reference
  source do it and where they differ, and what else its worktree does that rewake does
  not. Open it before changing the Claude Code worktree flag or `.worktreeinclude`.
- [research-protocol.md](research-protocol.md) — what the generated Codex schema and the
  reference source state: the flags schema generation needs, required fields of the
  types the adapter uses, how start-or-steer forks, where `canAcceptDirectInput`
  lives, what compaction and the terminal's other commands send over the protocol, and
  what a compaction or an interrupt on rewake's request would send and meet, where a
  new conversation runs, and what the protocol offers for the model's plan. Open it when the adapter or the fixture has to match a protocol change.
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
- [testing-cases.md](testing-cases.md) — what each end-to-end case claims, with its
  sessions, what the fixture plays and its controls, with a table of every control by
  scenario that `controls_test.go` keeps equal to the suite: the Claude Code telemetry budgets
  and their measurement, launching through a wrapper, pending and owed reports, the
  worktrees, a directory granted with a task, the awaited view, withdrawing, editing and adding to a sent message, a changed
  conversation, and the inbound gate. Open it before changing one of those cases or the
  behaviour it guards.
- [testing-plugin.md](testing-plugin.md) — the workflow cases that run rewake's
  function-hooks plugin under node: how the fixture hosts the module and answers its
  calls on the session, and what `claude-interrupted`, `stopped-routing` and
  `claude-steered` claim, with their controls, `codex-steered`, the same commands
  served by the Codex wrapper, and `codex-compact-hold`, a task sent into a long Codex
  compaction. Open it before changing the plugin, the control
  directory, the Codex side of `rewake compact` and `rewake interrupt`, or those cases.
- [testing-pool.md](testing-pool.md) — how the workflow suite runs its cases side by
  side: choosing `-parallel`, the pool that starts the longest scenarios first, the
  owner labels that keep one case's cleanup off its neighbours' processes, the sweep
  after the last case, and the product waits the suite's build runs shorter than a
  release. Open it before changing how a case starts or ends its processes, adding a
  scenario that must run alone, or shortening a product value for the suite.
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
  local responder, a two-way channel for Claude Code, the rest of stage 2 of the plugin
  and a focus for a Codex compaction, the parity queue, and what is
  queued without a date; and what the owner dropped, the todo list sent with a task among
  it. Open it to pick the next piece of work.
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
