# Stage 3: the steps S7–S11

The steps of [stage3.md](stage3.md#the-build-order) from the tool path to the host on the
live set: the neutral tool input and rig, the removals of Codex and of Claude Code's
hooks, the adapter API, the host's decisions by liveness. S1–S6 are in
[stage3-steps.md](stage3-steps.md), the moves in [stage3-moves.md](stage3-moves.md).
Citations are to `b6ed4ed`.

## S7. The tool path on the fixture [codex]

**Written.**

- **The neutral endpoint input**: the five inputs the endpoint's state takes today from
  two parsers (`bridge/endpoint/events.go:55,114`) — a turn started and a turn ended
  (`calls.go:330-347`), a call seen (`:135-157`), a call result (to `exposureOf`,
  `events.go:163`, and the asynchronous acknowledgment), a tool startup failed. The
  Codex event parser and the Claude Code hook path become callers of them and leave
  with their adapters in S8 and S9.
- **A transport's turn ids never reused — the one contract.** `inOwnTurn` trusts a
  turn id only on `CodexTransport` (`cli/pending_turn.go:21-31`). From S7 a transport
  may declare, in its binding, that its turn ids are never reused, and `inOwnTurn` reads
  that declaration on the ticket and on the record instead of the constant: declared,
  a later attempt in the same conversation and turn marks; absent, only the first attempt
  marks — T7's own wording. The Codex transport declares it; Claude Code's hook path
  does not; the fixture declares it by default and a switch withholds it. The tests that
  used the constant (`endpoint/helpers_test.go:25,69`, `cli/bridge_tool_test.go:73`,
  `cli/journal_test.go:74`, and through `inOwnTurn` `cli/pending_test.go:60,106,132,198`)
  run with a neutral test transport twice: declared, keeping their 1.x meaning, and
  absent, where a retry in the same turn does not mark and says why.
- **The turn's latest start, in the core.** `judgeMark` takes it as an argument since
  S4; its only source is the Claude hook's telemetry file
  (`harness/claude/telemetry/turnstart.go`). The host now records the neutral input's
  "turn started" in the mailbox (`inbox`, beside the read clock), and the CLI takes the
  later of that record and the telemetry file until S9 removes the file. The tests that
  write a start through telemetry (`cli/pending_test.go:51,269-275`,
  `cli/turn_hold_test.go:63`) write it through the core record.
- **The fixture's ToolTransport** ([stage3-fixture.md](stage3-fixture.md#the-tool-transport)),
  the descriptors built in `cli` from the command table and the allowlist
  (`cli/bridge_surface.go:21-37`), the binding from the harness's own ids, and the
  per-request peer check on the exact harness process
  ([design-claude.md](design-claude.md#how-a-tool-call-runs)).
- **The neutral rig**, `test/toolrig`, outside the layers
  ([stage3-tests-tcl.md](stage3-tests-tcl.md#the-neutral-rig)), beside `bridge/server`'s
  rig; both green.

**Tests.** Every oracle of `bridge/server` rebuilt on the neutral rig, each shown to fail
under the mutation its old one failed under; `endpoint/events_test.go:135` (ticket use
and binding) driven through the neutral input, and the neutral half of `:191` (the whole
result and its size), whose decoding of Codex result shapes stays with Codex until S8;
the gaps of E4, T1, T2, T4, T5, T6, T7 and T11 closed. **Review checks** no test of the
old rig is deleted in this step.

## S8. Codex, the MCP injection and `bridge/server` leave [codex]

**Deleted.** `internal/harness/codex` with `gateway`; in `harness`: `check*.go`,
`gates*.go`, `mailtool.go`; in `harness/claude`: `mailtool*.go`, `managed.go`; in `wrap`:
`mailtool*.go`, the server lifecycle of `wrap/channel.go`; `internal/bridge/server` and
`cli/bridge_serve.go`; in `bridge/endpoint`: the Codex parser (`events.go`, less the
neutral helpers) and `CodexTransport`; in `cli`: `accept.go`, the `bridge-hook` command,
`--no-mail-tool`, the Codex notify branch of the payload decoder (`cli/turn_payload.go`);
`registry.CodexHome`; `state.ToolConfigPath` (`state/paths.go:70-73`); in `channel`:
`selection.go` and the Codex and Claude MCP letters of `events.go`; Codex's marks of
`docs/legacy.md`. In the suite: the Codex fixture and column, its scenarios, the
`selection` capability and the gate's exception. `tools/harnesscache`,
`REWAKE_CODEX_VERSION` and their `AGENTS.md` commands. The Codex and MCP documents and
`mail-bridge-server.md` to the archive.

**Tests.** The old rig's tests go — their oracles ran on the neutral rig in S7. The
`cli` tests on the gateway, by what each holds: `gateway_wire_test.go` and
`gateway_side_test.go` (its fixtures) go; `gateway_intent_test.go:25` (a resume refused
until accepted) and `gap_advisory_test.go:18,74` (the gateway's advisories for a run
without proof of work) are Codex later; `gateway_integration_test.go:50` leaves with its
neutral half rebuilt in S5 and its selection half Codex later;
`review_receipt_identity_test.go:50,82` leave with their neutral oracle rebuilt in S5,
and `:109` stays, renamed by subject; `error_report_test.go:77,126` leave, rebuilt in S5.
The channel space is cut to the neutral alphabet and its tests rebuilt in this commit
([stage3-tests-tcl.md](stage3-tests-tcl.md#c1c8-the-channel-record)); C3 and C8 get
their own tests.

**Review checks** no file names Codex outside the archive and research; the channel
space still walks every start at depth 4; the suite is green on the fixture gate.

## S9. Claude Code's hook machinery leaves

**Deleted.** In `harness/claude`: `settings.go`, `statusline.go`, `plugin.go`,
`plugin.js`, `permission.go`, `telemetry/`; `grantauth/keeper.go` and
`state.KeeperAddress` (`state/paths.go:104`); `cli/granthook.go`, `cli/telemetry.go`,
`cli/turn_payload.go` and the hook command of `turnended.go` (`handleTurnEnded`,
`readPayload`, `isTerminal`, `:35-97,188-210`) — its turn-end functions stay for S14; the
hidden commands `turn-ended`, `observe`, `status-tap`, `grant-hook`; the Claude Code
branch of `cli/pending.go:65`; the hook path into the endpoint
(`bridge/endpoint/client.go:144`, `limits.go`); the hook constants of `harness/hooks.go`
with their users. The Claude Code column of the suite retires until stage 4. The hook,
plugin and telemetry documents to the archive.

**Kept.** Claude Code's Launch (flags, resume, worktree, protected directories, the
version read before the claim) and Wake (the 1.x messaging line: `socket.go`,
`lane*.go`, `receipt.go`, `replysweep.go`, `notice.go`).

**Refused — decided by the owner on October 5, 2026** (answer 1 of
[stage3.md](stage3.md#the-owners-answers)): stage 3's Claude Code offers Launch and Wake
only, with no interim turn boundary; this paragraph is the one rule that follows, and
S11 derives its own from it. A launch of a reporting role on a harness whose contract
reports no turns — neither `Backend` nor `TurnReporter` (`harness/backend.go:97,139`)
nor, from S10, a TurnBoundary — exits 1, naming the missing turn boundary and that the
role is available on that harness from stage 4; a task or question to such a session
exits 1 naming why.

**Tests.** The Claude Code launch tests that do not touch hooks carry
(`claude_test.go:44,101,118`). The hook cases go, each with its oracle already on the
fixture since S5: `TestPendingConfirm`'s Claude Code case and its controls on that
column, `cli/turn_hold_test.go:80,122,174,191,209`, `turn_test.go:109`, the hook leg of
`error_report_test.go:46` and `:99`. The tests that take a text from telemetry
(`cli/inbox_awaited_test.go:153,157`, `cli/turn_hold_test.go:152,161`) take it as a
literal: their oracle is the routing of a stopped turn, not the words. The grants step 3
gap closes. **Before the commit** the messaging line is checked live to wake a Claude
Code main without the hooks.

## S10. The adapter API and the catalogue as a value

**Written.** `internal/adapter`: the capability interfaces and the values of
[design-api.md](design-api.md); `adapter/catalog` with one constructor returning the
catalogue; `cmd/rewake` builds it and hands it to `cli`. `harness` becomes `adapter`:
the fixture and Claude Code implement capabilities, and the wrapper calls them through
the interfaces. Every TurnBoundary names its event (E7), carries the end's reason (O2),
and asks before an end closes — S5's confirmation, re-expressed. The fixture's readiness
exchange is re-expressed as the capabilities it makes live. The refusals stay those of
S9.

**The launch helpers, placed by the import rule** (an adapter imports no host):

- `Flag` and its argv helpers (`harness/flags.go`), the settings and defaults
  (`settings.go`, `defaults.go`) — standard library only — go to `adapter`, as functions
  over the values the interfaces carry, so the Claude Code adapter, `alias` (host) and
  `cli/launch_worktree.go` reach them by allowed edges.
- The environment: the host builds `LaunchRequest.Env` (`SessionEnv`,
  `harness/environment.go`), and an adapter declares as data the variables its children
  must not inherit; `CheckEnv` goes to the host.
- `notice.go` and `preview.go` go to `inbox` (core); `NoticeID` and `ShellQuote`, used
  only by Claude Code, to its adapter; `WorktreeHarness`, `ThreadTracker`, `ThreadSource`
  to the API; the `cli` assertions on `GitGrantHarness`, `DirGrantHarness`,
  `GrantIssuer` and `LaunchRefuser` (`cli/send_*.go`, `cli/launch.go`) become
  capability queries.

**What S8 left for S10** (S8's review, October 8, 2026). Three things only tests reach
after S8, each closed here, not carried to stage 5:

- `LaunchRefuser` (`harness/backend.go`, asserted in `cli/launch.go`): no adapter offers
  the refusal; the assertion becomes a capability query, and the empty interface and
  its assertion go if no adapter offers it then.
- The endpoint's server hello and client (`bridge/endpoint`, `roleServer`, `Dial`):
  no adapter starts a server since S8; the channel's hello and close are mapped onto
  the adapter capability's lifecycle, and the secret-based server ticket protocol is
  retired if no adapter consumes it.
- The channel's inputs: S10 defines the neutral channel and capability inputs the
  keeper takes; S11 wires the keeper.

**Tests.** `wrap/thread_test.go` tests the wrapper's use of `ThreadTracker` and
`ThreadSource` through its own fakes (`threadedHarness`, `threadObserver`); it is kept
and recast on the API's interfaces, whether or not a real adapter implements them in
stage 3.

**Review checks** no interface carries `registry.Session` whole; `adapter` imports only
`core` and `infra`; rule 3's exception retires; the suite is unchanged.

## S11. The host on the live set [codex]

**Changed.** The host records in the session record which capabilities are live — set
by the readiness exchange, withdrawn at once on disconnect — and decides by them: `send`
exits 3 without a live Wake; a task or question exits 1 without a live TurnBoundary;
`compact`, `interrupt` and `clear` exit 1 without a live Control; a reporting role's
launch waits for its TurnBoundary to become live up to the readiness bound, then refuses
([design-api.md](design-api.md#the-capabilities)) — on Claude Code at once, by S9's
rule. Liveness never changes the run: a capability withdrawn and made live again
leaves the session's epoch, its name and its authority as they were.

**Tests.** Each absence on the fixture column through its switch; a boundary lost
mid-run withdrawn at once; the offered set never read where the live one is meant; a
capability dropped and reconnected leaves the run and its grants' authority unchanged.
The existing assertions this changes — a send, task or question to a session that is
up but not yet live — are listed by a search of the tests that send to a session started
in the same test, and each is assigned to this commit.

**What S8 left for S11** (S8's review, October 8, 2026), each closed here:

- The channel keeper (`wrap/tools.go`, `channelKeeper`): nothing makes one after S8, so
  every session shows its mail channel as unknown. S11 wires it to the live set and the
  shell's evidence, and covers on the fixture column a channel offered and absent, live
  and lost, denied, the shell's evidence, and the record frozen at the run's exit. Until
  then C1–C8 prove the record and the keeper under synthetic inputs only, not end to end.
- `control.Serve`: S11 connects the fixture's live Control to its request and answer
  path; the package moves to the core in S13.
- `brief.Context.Tool`: no launch sets it after S8; S11 derives it from the live tool
  capability when it builds the briefing. It is the briefing's flag, not a field of the
  tool input.
