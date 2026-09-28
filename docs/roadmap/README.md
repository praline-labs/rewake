# Roadmap

Work for `docs/design.md` closes only after its acceptance criterion passes. This folder
holds the roadmap as one file per entry, each named by the day it closed and headed by
the words its section carried; this index lists them all. What is still open is named
below; the order of what comes next is in [work-queue.md](../work-queue.md). The
transport milestones of September 17 stay in
[transport-milestones-2026-09-17.md](../transport-milestones-2026-09-17.md), which
preserves owned-server acceptance, continuation permissions, lost reports, naming and
explicit main; the review rounds are in [reviews.md](../reviews.md) and
[reviews-later.md](../reviews-later.md).

Open [intermittent bugs](../intermittent-bugs.md): the unexplained two-root-thread
refusal on September 17, 2026 cleared after recipient restart.

## How the work is run

- One step, one commit; commit message short, subject line only, in English.
- Before every commit, the five checks listed in `AGENTS.md` are green: the
  formatter over the module's package list, `go vet`, `staticcheck`,
  `golangci-lint` and the race-detected, shuffled test run with the session's
  `REWAKE_*` variables cleared. `AGENTS.md` says why two of them take the form
  they do; this file does not repeat the commands, so they cannot drift here.
  No failing check is waived.
- Live runs happen in a separate `/tmp` directory, with their own `REWAKE_DIR`.
  The user's working harness sessions are left untouched.
- Live Codex runs spend subscription quota: cheap model, short messages, no
  more than one turn per check. Warn the owner before a run that spends quota.
- After milestones 3 and 5, a separate review agent passes over the diff:
  looking for defects and for checks that stay green on broken code. Findings get
  fixed before the next milestone.
- A milestone marked **live criterion** is accepted by running the real
  harnesses, not by tests.
- The working rules on delegation, the review chain and what the write session
  may commit were set by the owner on September 17–21, 2026 and live in
  `AGENTS.md`, under "Delegation and review" and "Commits".

## Open right now

- [Milestone 6. Ready for daily use](2026-09-16-milestone-06-ready-for-daily-use.md)
  — the publication is not done; the file keeps what is.
- [The workflow suite](2026-09-21-workflow-suite.md) — in progress since
  September 21, 2026; what exists and what remains are listed there and in
  [work-queue.md](../work-queue.md).

## Entries

Each file opens with the heading its section had. Two hold no dated entry:
[risks.md](risks.md), the risks table, and [later.md](later.md), what is deferred
without a date. Local installation without publishing is in
[install.md](../install.md).

| date | entry | what it records |
| --- | --- | --- |
| September 16, 2026 | [Milestone 1. Skeleton and interface](2026-09-16-milestone-01-skeleton.md) | command table, parsing, overview, help, failures, exit codes |
| September 16, 2026 | [Milestone 2. State and registry](2026-09-16-milestone-02-state-and-registry.md) | state directory, `/proc` reading, session records, liveness, `list` and `whoami` |
| September 16, 2026 | [Milestone 3. Claude Code: launch and delivery](2026-09-16-milestone-03-claude-code-launch-and-delivery.md) | wrapper, inbox, socket delivery, `send` end to end; live criterion met |
| September 16, 2026 | [Milestone 4. Codex: launch and delivery](2026-09-16-milestone-04-codex-launch-and-delivery.md) | thread lookup, `codex queue`, the sandbox pid namespace defect; live criterion met |
| September 16, 2026 | [Milestone 5. Intro and permissions](2026-09-16-milestone-05-intro-and-permissions.md) | the intro, `--no-intro`, the allowed-tools rule; met as a side effect |
| September 16, 2026 | [Milestone 6. Ready for daily use](2026-09-16-milestone-06-ready-for-daily-use.md) | open: the watcher, the sweep, `README.md` and `scripts/pack.sh` are done, publication is not |
| September 16, 2026 | [Milestone 7. Notices instead of pasted text](2026-09-16-milestone-07-notices.md) | one-line notices, `rewake inbox`, message kinds, turn-end reports; live criterion met |
| September 16, 2026 | [Milestone 8. Three kinds of message](2026-09-16-milestone-08-three-kinds-of-message.md) | task, question, notify and what each owes |
| September 16, 2026 | [Milestone 9. Roles](2026-09-16-milestone-09-roles.md) | the role catalogue, main reports nothing, explicit `--main` |
| September 16, 2026 | [Milestone 10. Rooms](2026-09-16-milestone-10-rooms.md) | rooms isolate discovery, addressing and delivery; acceptance verified |
| September 19, 2026 | [Gateway integration](2026-09-19-gateway-integration.md) | pointer to the installed startup and primary/side acceptance |
| September 19, 2026 | [Session state and service notices](2026-09-19-session-state.md) | scoped owner acceptance of state and activity |
| September 19, 2026 | [Compact tables and grouped inbox](2026-09-19-grouped-inbox.md) | grouped ready mail, native start-or-steer, installed acceptance |
| September 20, 2026 | [Native mailbox output](2026-09-20-native-mailbox.md) | native delivery accepted with its evidence limits |
| September 20, 2026 | [Native arrival UI](2026-09-20-native-arrival-ui.md) | the display-only Ran row, transient by decision |
| September 21, 2026 | [The Codex transport pin](2026-09-21-transport-pin.md) | pin moved to 0.155.1 behind one constant; a mismatch warns |
| September 21, 2026 | [Launch defaults from the environment and from settings files](2026-09-21-launch-defaults.md) | model and effort for one launch, strongest source first |
| September 21, 2026 | [The workflow suite](2026-09-21-workflow-suite.md) | in progress: what `test/workflow` runs and what it does not yet |
| September 21, 2026 | [The harness research, split by how a fact is obtained](2026-09-21-research-split.md) | three research documents by how their facts age |
| September 21, 2026 | [Launch aliases, on the project's first dependency](2026-09-21-launch-aliases.md) | `rewake <alias>`, the alias file, the dependency decision |
| September 21, 2026 | [A role-shaped first page](2026-09-21-role-playbook.md) | the role playbook in the briefing and the guide |
| September 21, 2026 | [The task-report scenario, as built](2026-09-21-scenario-task-report.md) | what building the first delivery scenario taught, and what it does not prove |
| September 22, 2026 | [The research packages, folded into the documents that outlive them](2026-09-22-research-packages.md) | archives deleted, their findings written into the surviving documents |
| September 22, 2026 | [The batch-arrival scenario, as built](2026-09-22-scenario-batch-arrival.md) | grouping, previews, per-message reads and no replay, with four product mutants |
| September 22, 2026 | [The mid-turn scenario, as built](2026-09-22-scenario-mid-turn.md) | a delivery steered into a running turn, and what the server decides rather than the product |
| September 22, 2026 | [A summary of a suite run instead of its transcript](2026-09-22-suite-summarizer.md) | `tools/checksummary`: a few lines and a summary.json, built from records rather than prose |
| September 22, 2026 | [A fixture for the Claude Code column](2026-09-22-fixture-claude-code.md) | the scenarios run twice; what the socket column cannot show, and says by name |
| September 23, 2026 | [A named harness version, cached and run in a container](2026-09-23-harness-versions.md) | `tools/harnesscache`, `REWAKE_CODEX_VERSION`, and what 0.156.0 showed |
| September 23, 2026 | [A telemetry collector for Claude Code](2026-09-23-claude-telemetry.md) | background hooks and a status-line tap into one collector; seen live the same day |
| September 23, 2026 | [Honest delivery status for Claude Code](2026-09-23-honest-claude-delivery.md) | `held` as a state, the wrapper's reply socket, the first notice waiting for the status line |
| September 23, 2026 | [Re-reading the owed task](2026-09-23-owed-reread.md) | `rewake inbox --owed`, and the rule to use it after a context compaction |
| September 23, 2026 | [A quieter feed](2026-09-23-quiet-feed.md) | every text line of the ordinary flow, kept, shortened or dropped; two kept by owner decision |
| September 23, 2026 | [Conversation tracking for Claude Code](2026-09-23-claude-thread-tracking.md) | deliveries pinned to the collector's `session_id`, compared with the Stop hook's; `threadChanged` on the fixture |
| September 23, 2026 | [What others owe you](2026-09-23-awaited-view.md) | `rewake inbox --awaited`: what a run sent and still waits on, by recipient, and the main playbook's line for it |
| September 23, 2026 | [A Claude Code plugin that hears an interruption](2026-09-23-claude-plugin.md) | stage 1, observation only: Esc gives `stopped` without waiting for the next turn, with the Codex semantics, activity follows turns, `interruptions` says whether it is heard; accepted live by the owner on September 24, 2026 |
| September 24, 2026 | [A Claude Code session's context window, as the person set it](2026-09-24-claude-context-window.md) | the auto-compact window read through the plugin and the launch and shown as the window; reviewed live in eleven runs |
| September 24, 2026 | [A main that compacts and interrupts a Claude Code session](2026-09-24-remote-control.md) | stage 2 of the plugin, part A: `rewake compact` and `rewake interrupt`, the control directory and its withdraw-then-look protocol, the module acting, the stopped text and the one-shot notice line; probed live twice, the workflow case `claude-steered` with seven mutants |
| September 24, 2026 | [A main that compacts and interrupts a Codex session](2026-09-24-remote-control-codex.md) | stage 2 part B: the Codex wrapper serves the control directory, the gateway compacts marked manual and refuses mid-turn itself, interrupts naming main; no notice line on Codex by owner decision; probed live on 0.155.1, the workflow case `codex-steered` with five mutants; nine review rounds, accepted on the Codex side after a live run on 0.155.1 on September 25 |
| September 25, 2026 | [`rewake compact` that does not wait for the end](2026-09-25-compact-non-blocking.md) | the command returns once the compaction started, or says requested; the end reaches main as a notify with the tokens and the count, from the worker's telemetry through main's wrapper; the workflow cases with new mutants for both |
| September 25, 2026 | [One notice for a burst of letters](2026-09-25-coalesced-notices.md) | notifies and reports wait up to four seconds for company in the shared mailbox server, tasks and questions go at once; the batch-arrival case with a fifth mutant |
| September 25, 2026 | [The workflow suite in three minutes instead of twenty](2026-09-25-suite-speed.md) | cases side by side in a pool sized by `-parallel`, owner labels for process cleanup with a final sweep, five product waits shorter in the suite's build; 19m22s to about 3m10s, crosswise 7m02s to 1m17s |
| September 26, 2026 | [Actions on a sent message](2026-09-26-sent-message-actions.md) | `rewake withdraw`, `rewake edit` and `rewake send --to` by the id `send` now prints, short forms included; withdrawn final like read; a recall note when a withdrawn notice may have gone out, on a line of its own in every notice; an edit's replacement previewed as replacing the old id, with no recall; the reviews' halfway failures closed; the workflow cases `withdraw-after-notice`, `edit-after-notice`, `addendum-owed` and `withdraw-mid-turn` with eight mutants |
| September 26, 2026 | [A wrapper that stops before it continues its harness](2026-09-26-stopped-wrapper.md) | the intermittent stopped-wrapper bug: the stop aimed at the calling thread with `tgkill`, a test that fails on the old code every run; `docs/launch.md` split, defaults and aliases into `launch-defaults.md` |
| September 26, 2026 | [Documentation that had drifted from the code](2026-09-26-docs-drift.md) | the controls, commands, source tree and session record set against the code; the controls as a table and the source tree under tests in `docs/`, the command list replaced by a pointer to `rewake guide` |
| September 26, 2026 | [Addenda kept with their task](2026-09-26-addendum-consistency.md) | withdraw takes a task's unread addenda along, an edit's replacement keeps them through `inbox.CurrentTask`, the `--to` checks asked again under the recipient's lock, an edited notify announced at once; after review, an id an edit replaced names its replacement for withdraw and edit |
| September 26, 2026 | [Durations between processes on the boot clock](2026-09-26-boot-clock.md) | the coalescing cap, a waiting question's answer mark and a telemetry snapshot's freshness read the boot clock a letter, a mark and a snapshot now carry, so a step of the wall clock no longer splits a burst or makes a live send look gone; the long terms stay on the wall clock |
| September 26, 2026 | [Codex's `--worktree`, researched](2026-09-26-codex-worktree.md) | the terminal refuses `--worktree` with `--remote` before any checkout, on 0.155.1 and 0.157.1, so passing the flag through cannot work; `thread/start` takes a per-conversation `cwd` and no worktree field; rewake keeps its refusal, a plain `git worktree add` is the supported way, and allocating a managed checkout in rewake is not built unless the owner asks |
| September 26, 2026 | [A pending turn end that keeps the turn's text](2026-09-26-pending-text.md) | the interim message carries the mark's line and then the turn's own text; the briefing's pending rule covers a turn a subagent or background task woke; the workflow case `pending-report` with a third mutant; a Stop-hook confirmation on Claude Code awaits the owner, a mark kept until "done" and a rule by what woke the turn rejected |
| September 26, 2026 | [A task sent into a long Codex compaction](2026-09-26-codex-compact-hold.md) | a compaction seen running holds deliveries up to 10 minutes, the server's `ActiveTurnNotSteerable { turn_kind: Compact }` leaves the message pending, a wait that ends with the compaction running answers `started` and its end is main's letter; the shim refuses input during its compaction, the workflow case `codex-compact-hold` with four mutants; after review, the compaction request bounded by the start bound again, and a late `started` no longer hides the end |
| September 26, 2026 | [The terminal of Codex 0.157.1 recognized](2026-09-26-codex-0157-recognition.md) | 0.157.1 sends the workspace roots null and was never selected; its configuration's `web_search` now marks the terminal's start, by-id resume and fork, an ordinary resume carrying it is not a reconnect; the probe's five paths replayed as unit tests, the shim's terminal speaks the 0.157.1 form, the workflow case `codex-tui-later-shape` with one mutant |
| September 26, 2026 | [Live checks of messaging on Codex, and Claude Code probes](2026-09-26-live-checks.md) | on Codex 0.155.1 with real sessions: grouped notes, a task and its addendum in one running turn, withdraw and edit before reading, an addendum after `/new`, a task held through a short compaction and a remote interrupt passed, the start-of-turn window partial, the long compaction paths not reached, and a command the interrupted turn started ran on; on Claude Code 2.1.280, a blocking Stop hook holds the turn up to eight times, a `-w` session registers but its record keeps the launch directory; traps: a nested Codex sandbox, the plugin marketplace cloned over SSH, `REWAKE_ROOM` at launch |
| September 26, 2026 | [The Stop hook asks once after a pending turn end](2026-09-26-pending-confirm.md) | on Claude Code an unmarked turn end after an interim one is held once with `decision: block` and the model asked; the held answer goes into the report of whatever end follows, a stop or failure included; one hold per turn, no turn start recorded for it; someone else's blocking hook left as it is and written down; the fixture answers a block as the harness does, the workflow case `pending-confirm` with two mutants |
| September 26, 2026 | [A worktree for a Codex launch](2026-09-26-codex-worktree-launch.md) | `rewake codex --worktree[=<name>]` makes a detached checkout of HEAD under a durable worktree directory with `git worktree add` and starts the session in it; `rewake worktree ls` and `rm`; grants and Codex trust reach the checkout as a manual worktree; the owner's variant B, Codex's private scheme left to study later; the workflow case `codex-worktree` with three mutants |
| September 26, 2026 | [Review of the Codex launch worktree](2026-09-26-codex-worktree-review.md) | rm without `--force` never loses work: reachability asked every time, any rewake session working in a checkout holds it, a missing directory refused with a `git worktree repair` hint and removed alone, ignored files kept; `--worktree` with `resume`/`fork` refused, an untouched checkout taken back on an error exit, a root inside the repository refused, git without prompts or optional locks; `codex-worktree` with four more observations and mutants; after re-review, `resume`/`fork` refused by the word wherever it stands before `--`, a record whose repository is gone removable, a dangling root link refused, and the suite's sweep pausing after its last find |
| September 27, 2026 | [Worktrees on a branch, landed and finished](2026-09-27-worktree-land.md) | `--worktree` makes a branch of the worktree's name; `rewake worktree land` fast-forwards the source's branch or `--into`, `finish` lands and removes the worktree and its branch, rm drops a branch only when nothing is lost; `rewake claude --worktree` with `-w` left native and continuations refused; `.worktreeinclude` copies named ignored files; heavy dependencies left to the agent and the repository by the owner's decision; `codex-worktree` extended with four mutants, `claude-worktree` with one; after review, land refuses a target another checkout or a rebase holds, state refusals exit 1 in rm too, and a bad `.worktreeinclude` line is skipped |
| September 27, 2026 | [Worktree names with a slash](2026-09-27-worktree-slash-names.md) | `--worktree=feat/super-feature` makes the branch of that name, by the owner's decision; names of ASCII letters, digits, `.`, `_`, `-` and `/` that git takes as a branch, up to 250 bytes; the directory and record write the slash as `+`, as Claude Code does; `<repository>/<name>` read in full first; a branch above the name refused like one below |
| September 27, 2026 | [A directory granted with a task, stage 1](2026-09-27-grant-dir.md) | `rewake send --grant-dir` and `--grant-dir-broad`, repeatable, from main on a task or question; a hard tier never granted and a broad tier confirmed by name, checked at send and again at delivery; a granted task waits for an idle Codex thread on a notice of its own, `--grant-git` included; the directory joins the thread's roots and is taken back at the first delivery after the report, from a journal `rewake list --json` shows; `rewake codex --add-dir` refused; Claude Code refused until stage 2 |
| September 27, 2026 | [A grant a worker cannot forge](2026-09-27-grant-unforgeable.md) | a grant is registered with main's wrapper from a process below it and confirmed by the recipient's wrapper from the sender's run in the same namespaces, held in memory; a Codex main cannot grant, legacy Landlock refused; the journal in the worker's wrapper, never evicted; empty value, `--to`, a live session's directory, `/tmp`, shared `.git` and a covered checkout's metadata fixed; the workflow case `codex-grant-forgery` with three mutants |
| September 27, 2026 | [A directory granted to a Claude Code session, stage 2](2026-09-27-grant-dir-claude.md) | `--grant-dir` to a Claude Code session through its PermissionRequest hook: the hook asks its own wrapper, which keeps the grants in memory and answers only below itself; a write in a live grant allowed with the root added, `.git` and harness directories left to the person, the root removed by a forced question after the report; delivery waits for idle; a courtesy, not a boundary; the workflow case `claude-grant-dir` with three mutants |
| September 27, 2026 | [No session started from inside a session](2026-09-27-no-nested-launch.md) | a launch from a shell where `REWAKE_SESSION` or `REWAKE_EPOCH` is set exits 2, by main's decision, so a worker's unasked `rewake` commands start no agent; a plain `rewake` command takes a grant back only under the rule the launch added, told to the hook with `--rewake-allowed`; `grants.md` says a command's question leaves the directory in place |
| September 27, 2026 | [Worktrees of one repository made one at a time](2026-09-27-worktree-serial.md) | launches with `--worktree` in one repository take an `flock` beside its directory under the worktree root, by main's decision, since git 2.43 dies on another add's half-written `commondir`; rm and finish take it too; a minute's wait, then exit 1 naming the holder; one retry of an add that meets an entry rewake did not write; the workflow suite keeps each session's standard error on a red case and names its last line |
| September 27, 2026 | [A grant restored after a cold resume](2026-09-27-grant-resume.md) | main's wrapper holds a grant while its task is open rather than for a fixed time, and confirms it again for the run that resumes the conversation, by main's decision; each journal entry names its conversation and main, and an ended run's copy stays while that main runs; a resumed run takes over the waits of tasks delivered into its conversation, and its report settles them; Claude Code is started with `--add-dir` for what main confirmed, Codex has the roots journaled again, a lost one added back and an unconfirmed one taken out; nothing is restored once main has ended; the workflow cases `claude-grant-resume` and `codex-grant-resume` with six mutants |
| September 28, 2026 | [The help told true](2026-09-28-cli-truth.md) | help, guide, refusals and briefings checked against the code: a grant's real end per harness and mode, its restore conditions and main's restart forfeiting them; `inbox --awaited` names "may still report"; `--grant-git` for a Codex write session only, a Claude Code recipient refused with exit 1; the nested launch rule on every launch page and in main's playbook; `send --wait` waits as long as asked; `-h` is help and other short flags are refused; a send to oneself refused; `rewake claude --worktree <word>` refused, and a typed `--worktree` replaces an alias's; after review, a standalone `--grant-git` named as never taken back, a long wait ended with its recipient, a leading `-h` read as `--help`, and `edit` refusing a self-send |
| September 28, 2026 | [A release gate for npm](2026-09-28-release-gate.md) | `go run ./tools/release <version>` checks a clean pushed tree, the source's `Version` set to the release by its commit, a version free in the registry by npm's own E404 only, the packages built by `pack.sh` against allowlists both ways, each binary's architecture, this machine's build and the shim installed with and without its platform package; publishes nothing without `--publish`, then the platforms before the entry, stopping at the first failure; `repository` in every package; the README installs from npm |
| September 28, 2026 | [Grants hardened after the reconnaissance](2026-09-28-grant-hardening.md) | an edit carries the grant main's wrapper holds, not its letter's; Windows short names read back to long ones; a path through `/tmp` refused and send prints the resolved directory; a shielded directory as the root is broad; the hard tier adds login and startup configuration, Go caches and toolchains; main is told the conversation the letter was pinned to; an unread task of an ended run, a failed or withdrawn one, is closed; adoption keeps the first read time; tests for the gaps the reconnaissance listed |
| September 28, 2026 | [Worktree removal, finish and land made whole](2026-09-28-worktree-lifecycle.md) | rm checks and removes under one hold of the repository's lock, finish holds it from its checks to the removal, with the record read again; a checkout whose launch is still starting its session is kept and shown as launching; `.worktreeinclude` copies recorded at once; finish and rm refuse a locked checkout or one with a submodule checked out before landing, `rm --force` passes git's double force; branch lines fit the state; a root inside the main checkout refused from a linked one; land refuses a source that switched branches mid-land, exits 1 for a source on the worktree's branch, 2 for `--into=`, and stops waiting for a hook after a minute without stopping git |
| September 28, 2026 | [The Codex grant path by conversation](2026-09-28-codex-grant-restore.md) | a settled grant is taken back only in the conversation it was granted in, and a root is rewake's only there; a resume takes every hinted root out before adding the confirmed back, so a refused copy cannot take a confirmed grant's root; the mains are asked before the reservation and the notice waits pending, as does one refused before sending |
| September 28, 2026 | [rewake 1.0.0 on npm](2026-09-28-release-1.0.0.md) | published on the owner's word as `@praline-labs/rewake` with its two platform packages from the release commit `6aacced`, tagged `v1.0.0`; the gate refused to call it done until the registry showed all three; installed from the registry and run; 1.0.1 the same day carries the new README and the tightened help |
| September 28, 2026 | [One npm package for rewake](2026-09-28-one-npm-package.md) | by the owner's decision, from 1.0.2 one name: the entry `<version>` under `latest`, each platform build the version `<version>-linux-x64` or `-linux-arm64` of the same package under its own dist-tag, reached through scoped `npm:` aliases in `optionalDependencies`; the shim looks for the alias; the gate asks for each version, checks the aliases, publishes with tags and checks them after, and has npm install the release from a loopback registry of its own; the old platform packages removed, 1.0.0 and 1.0.1 to be deprecated after 1.0.2 |

## Remaining work — owner decisions, September 19–21, 2026

What comes next, in the order the owner set on September 21, 2026, is in
[work-queue.md](../work-queue.md): the rest of the workflow suite, then pinning harness
versions in a disposable environment — its schema half closed on September 23, 2026
([entry](2026-09-23-harness-versions.md)), its behaviour half is still queued — then a
two-way channel for Claude Code, then the parity queue. Launch aliases, which stood second in that order, closed the same day
([entry](2026-09-21-launch-aliases.md)). This section keeps the decisions behind those items.

What the directory grants and the worktrees of September 27, 2026 left open — a grant
restored after a cold resume, a grant from a Codex main, a root swapped after delivery,
`land` and `finish` beside a foreign `git worktree add`, an owner file of grant rules —
is ordered in [work-queue.md](../work-queue.md#now-what-the-directory-grants-and-the-worktrees-left-open).

The native-notification priority is complete, and so is the Claude Code handoff it
pointed to: orchestration moved to Claude Code on September 21, 2026. The first three
entries of the [parity queue](../harness-features.md) closed the same day — HF-12, HF-07
with HF-20, and HF-21 — each on one observed run
([claude-parity-2026-09-21.md](../claude-parity-2026-09-21.md)). Queue entries are not
milestones; the remaining ones are listed in the feature map.

Check automation is the [workflow suite](../check-runner.md), recorded in
[the suite entry](2026-09-21-workflow-suite.md): the research answer,
[check-runner-proposal.md](../check-runner-proposal.md) with its selected scenarios in
[check-runner-scenarios.md](../check-runner-scenarios.md), was accepted on September 21,
2026 and the first scenario is implemented; short console output and a summary file,
and paid and manual tiers as opt-in, are still requirements rather than code.

A command that lists the models and effort levels a harness offers is **deferred**.
Owner decision, September 21, 2026, asked directly: not now. The reconnaissance that
would feed it is done and recorded in [research-launch.md](../research-launch.md) — what can be read
from each harness, how, and at what cost — so the work would start from facts rather
than from scratch. Roughly a day.

It would answer, per harness: which models are available to this account, which effort
levels each of them takes, and what the configured context window is. For Codex the
window comes from the catalogue and a configuration key, with arithmetic. For Claude
Code neither the subcommands that were read nor the catalogue cache carries one — but
the Agent SDK and the data handed to the status line do, and the reconnaissance did not
go there. So the open question is not
"is there a source" but "which of them is worth depending on", and nobody has yet found
a catalogue of windows for every model that does not require starting a session.

The sources are uneven: one protocol method, belonging to the Codex protocol, and three
unofficial ones — a debug subcommand group, an undocumented per-login cache, and the
text of a warning message. On the Claude Code side the documented route is the SDK.

The requirement that shapes it: **a source that disappears must turn its cell into
"unknown", never leave a stale truth standing.** Caches expire, debug commands are not
promises, and an account's list changes. A table that keeps showing yesterday's answer
because today's lookup failed is worse than one that admits it does not know — that is
the same mistake as a check reporting success it did not earn, one layer up.

A lead for the Claude Code telemetry collector, then missing (entry 5 of the
[parity queue](../harness-features.md)). Claude Code has a channel where the harness calls
a command **we** name and hands it a structure describing the session, the context
window size among its fields — so the source is the input to a program of ours, not
text meant for a person. rewake already works this way elsewhere: the wrapper sets a
command for the end of a turn, which calls `rewake turn-ended`
(`internal/harness/hooks.go`). A different integration point, the same shape — the
harness calls what we named and hands it data — and that shape looks like a fit for
the collector.

Not proposed, and not to be done: parsing the rendering of the status line. It is a
display for a person, its format is not promised, and a reader of the screen would be
fragile for no reason. What that structure carries was established on September 23,
2026, beside what the hooks carry, in [research.md](../research.md#telemetry-sources-the-status-line-and-hooks):
model, effort and context fill are there, activity is not, and compactions come from
hooks. The collector was built the same day on both, the hooks and a tap on the status
line that keeps the person's own ([entry](2026-09-23-claude-telemetry.md)).

Persistent Git permissions remain a separate future item: explicit orchestrator
event, scope to run and repository, preservation across later owner turns, and revocation. A running turn keeps its prior permission context.
Only validated Git metadata roots are in scope, not tools, credentials or arbitrary
folders. Do not implement that persistence in the current batch feature.
The main-readiness/missing-notice incident stays open separately.
