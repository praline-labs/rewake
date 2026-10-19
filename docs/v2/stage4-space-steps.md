# Stage 4: the first working space — tests, rules and build steps

**Superseded in part, October 18, 2026**, by the owner's decision: a space is main starting its
workers, not shown inside Claude Code; the liveness pipe and the gate below are not built, and
one step replaces S4-7 to S4-10 ([the build steps](#the-build-steps)). What concerned the keeper
subagent, the screen drawn in main's session and its keys was removed on October 19, 2026.

The second half of [stage4-space.md](stage4-space.md), written October 13, 2026: what the
tests can hold and what only a live check shows, the rules each new behaviour needs before
code, the build steps in order with their acceptance, and the questions a step would
otherwise guess. Every step goes through the chain of `AGENTS.md`: the writer writes, a
review, fixes, and the commit.

## 10. Tests

**What the checks hold, with no harness.**

- `internal/host/space`, the launch: the launcher started from the test binary, the worker's
  wrapper reparented — shown outside the test's tree — with the slave side as its
  controlling terminal; the environment without the identity and transport variables and with
  the base of the room it was given, never the default; the liveness pipe's write end in no
  child; a test process that sets itself a subreaper sees the open refused; an unreadable
  ancestry refuses the open; with the launcher delayed on purpose, nothing of the harness runs
  before the gate's byte, and the pipe's end while waiting aborts the launch. The hosted
  wrapper's groups: the harness in a group of its own, the terminal's foreground group, the
  wrapper outside it; Ctrl+C and Ctrl+Z written into the terminal reach the harness and not the
  wrapper, and a stopped harness is continued; a handoff made from the already-background
  wrapper succeeds, its result and the foreground group checked; the harness's signal mask and
  dispositions hold no blocked or ignored job-control signal.
- `internal/host/terminal`: a real pseudo-terminal running the test binary itself as a
  small program (echo, a full-screen draw with colours and wide glyphs, the alternate
  screen, a resize report); bytes in, the screen read back as styled rows; a resize reaching
  the program as SIGWINCH and its new size; a hang-up ending the program's session.
- `internal/core/space`: records written only under the room's lease (`buildtest.Room`), the
  states and their order, a crash between steps leaving a record the next start can close,
  the kept-checkout listing.
- `internal/adapter/claude/modhost`: each endpoint against a peer seam as the endpoint's
  tests use, a request from another process refused, bounds on bodies and waits; a `turn/end`
  whose receipt was lost retried
  with the same end id and taken once; a failed one reported as failed.
- `internal/adapter/claude`: the embedded mod written into the run directory and named by
  `--plugin-dir`, the mod socket's path in the environment, `--session-id` from the offer's
  `ConversationOffer`.
- `tools/protogen`: the protocol's generated files equal to a fresh generation, and the
  generator refusing any word of the schema it does not read. The test lives beside the
  generator, since no layer of `internal/` may import a tool.
- The mod itself: a Go test runs `claude plugin validate` and `claude plugin test` on the
  written tree when a Claude Code of at least the minimum version is on `PATH`, and skips
  naming why when not; the mod's own tests (`*.test.ts`) run with the skill's test kit, as
  the probe's kit did.

**What the fixture column and the workflow suite can hold.** The space host is
harness-neutral, so the fixture's program can be the worker in the pseudo-terminal: a
fixture main runs `rewake space open --harness fixture`, and the scenario checks that the
worker starts in the given directory with a scrubbed environment, registers under its name
in main's room, carries the session id the record holds (the fixture adapter's `ConversationOffer` names a
flag by then; until S4-7 it answers that it takes none), exchanges a task and its report with main, and ends when main's run
ends, the record closed. The scenario runs under a private, non-default base and checks that
the default base was not touched. The fixture reaches the space host through the neutral seam
and imports nothing of the Claude Code adapter.

**Main's wrapper killed:** a real-process worker
that ignores SIGHUP and SIGTERM, main's wrapper killed with SIGKILL; SIGKILL is needed and sent
to the harness's group only; within 5 + 2 s and a margin the worker's processes are gone, the
end is recorded by the surviving worker wrapper, and that wrapper is gone after; the next main
start closes the record.

**What only a live check shows.** That `$.http.fetch` connects from the harness process (P4);
a worker's `turn.complete` carrying its answer. Main runs them, as in the probe.

## Rules before code

A new group, `docs/rules/space.md`, numbered **SP1–SP9**, made in S4-2. A rule entered the group
with the step that built what holds it (decided by main, October 13, 2026): SP3, SP4 and SP9 at
S4-2, SP2 and SP7 at S4-3. Since October 18, 2026 SP5 and SP6, the keeper's seat and its keys,
are not built (removed from this list on October 19, 2026), and SP1 entered with the space step
as the space host watching main's run; SP8
is not built by it. Until then a rule stands here.

- **SP1. A space lives no longer than its main.** When main's run ends — by `session.end`, by
  its harness ending, or by its wrapper being killed — every worker of its spaces is ended
  within a bound, by its own wrapper when main's is gone — a wrapper that no signal of that
  shutdown reaches, so it records the end it saw. A conversation that ends while the process
  goes on (`clear`, `resume`)
  closes nothing.
- **SP2. The record comes first.** A space's record — the worker's harness, name, role,
  session id where the harness takes one, the checkout's path and branch — is written before
  the worker starts, and every later state is written before the act it names is answered. A
  record is closed only from what was observed — the worker's end, the checkout's state — never
  from a release or a signal sent.
- **SP3. A hosted launch inherits nothing of a session.** Its environment carries no variable
  that names main's session or a transport of main's run, and none of the worker harness's
  `EnvNotInherited`; what it carries of rewake's identity and transport is set fresh by the
  host — the base of main's checked room, the room, the liveness descriptor — while the person's
  preferences pass as they are. It runs the wrapper's own image and joins main's room under
  main's base.
- **SP4. Only main's harness process reaches main's space host.** Every request of the mod's
  protocol is checked by its peer on its own, as T2's transport is; a descendant is refused.
- **SP7. A kept checkout is never forgotten and never removed by rewake.** A closed space
  whose checkout is still there is listed to main at its start and by `rewake space list`
  until the checkout is gone; rewake removes no checkout.
- **SP8. A record left open by a dead main is closed by the next start**, once that run is
  proven dead, with what was seen of its checkout; an unknown liveness leaves it open and
  says so.
- **SP9. A hosted worker is never main's descendant.** Its wrapper is reparented away from
  main's wrapper before it starts, checked by its whole chain of parents; an open that cannot
  prove it is refused, and nothing the worker controls runs before that proof.

L1 and L3 carry the mod's directory; T10 the worker's build; the existing O2 and the
reporting refusals carry the worker's reports.

## The build steps

Each small enough for one review; each closed by its acceptance check, not by its code.

| Step | What it builds | Acceptance |
|---|---|---|
| S4-1 | P4, live, from the probe's kit: the peer of `$.http.fetch` on every request and after a reload, and a shell child's request refused; a worker's `turn.complete` and its `answer` | the records in [research-claude-mod.md](../research-claude-mod.md); P4 positive, or the stage stops for a channel design ([design-claude.md](design-claude.md#the-socket-and-its-peer)) |
| S4-2 | the rules test's SP group with SP3, SP4 and SP9; the protocol schema, `tools/protogen` and the drift test | the drift test green; review of the rules |
| S4-3 | `internal/core/space` and `rewake space list`; built October 13, 2026 ([the record](stage4-space-record.md)) | the record tests under the lease; the listing on a hand-written record, closed and kept |
| S4-4 | `internal/host/terminal`, with `charmbracelet/x/vt` (the owner's exception, [section 1](stage4-space.md#the-terminal-library)); built October 13, 2026 ([the terminal](stage4-space-terminal.md)) | the pseudo-terminal tests above, with `-race`; the layout test with the library only in `host/terminal`; `go list -deps` of the binary before and after, recorded with the exception |
| S4-5 | the embedded mod's skeleton and `modhost`: written per run, `--plugin-dir`, the mod socket, `hello` with the peer check — part A, built October 13, 2026, its handlers counted among the run's workers after review ([loading the mod](design-claude.md#loading-the-mod-for-one-run)); `SpacesCapability` live on main; the spaces offer, its live and refused states and the conversation's name written into the API contract and the catalogue's completeness test, not as scattered optional assertions — part B, built October 13, 2026 ([spaces](design-api.md#spaces), [conversation names](design-api.md#conversation-names)) | live: the hello on the installed Claude Code, a shell child's request refused; the validator test |
| S4-6 A | built and accepted October 16, 2026 ([the record](../roadmap/2026-10-16-stage4-s4-6.md)): the worker's turns ([stage4-turns.md](stage4-turns.md)): `turn/start` and `turn/end` in `modhost`, each end's first capture reused by every retry, the dispatch split out of `server.go`; the mod's `turn.start` and `turn.complete`, its outbox in `$.store`, its retry chain and the cut of a long answer; the offer `TurnBoundary{Reports: true, HoldsEnd: false, AfterStart: true}`; the known causes refusing a reporting role; availability waiting for a boundary proven after the start | the tests of [part A](stage4-turns-build.md#the-build): the reasons mapped, a failed end, a lost receipt and a retry answering only its first capture, one end at once captured once, another digest refused, a request across `Close`, a run proven gone answered 410 with no receipt and an unreadable record retried, a reporting role refused on a known cause, a task refused before the hello; the mod's tests in `claude plugin test`, its outbox kept until a receipt or a run proven ended |
| S4-6 B | built and accepted October 16, 2026: the heartbeat: `mod/alive`, generations, the lapse and the withdrawal of TurnBoundary and main's Spaces, the restore by a new hello | the tests of [part B](stage4-turns-build.md#the-build): a lapse withdraws and a hello restores, a hello of an earlier or superseded load refused, an older generation or transition changes nothing, an end after a lapse is taken |
| S4-6 C | passed October 16, 2026 on Claude Code 2.1.296 ([the records](../research-claude-mod.md#s4-6-a-workers-turns-through-the-mod)): live, by main ([part C](stage4-turns-build.md#the-build)) | main sends a task to a worker started by hand with `rewake claude --write` and reads its report; the same through `--command` with and without `exec`; a mod that stops polling withdrawn within 30 s and restored; an interrupt reported as stopped; a subagent's turn not reported; the hooks' order at a turn's end and the heartbeat between turns and through a reload |
| S4-7 | built October 18, 2026 on branch `space`, waiting for the review and main's live acceptance ([the entry](../roadmap/2026-10-18-space.md); [launch.md](../launch.md#a-space-for-a-worker), [the rules](../rules/space.md)); the record lost `keeper` and gained `host`, the session is `<name>-<harness>`, `Ctrl-\` detaches. The space, by the owner's decision of October 18, 2026 in place of S4-7 to S4-10: `rewake space open <name> --harness claude\|pi [--role …] [--worktree[=…]] -- <harness args>`, a change for main only while its run has `Spaces` live; a short launcher re-executes rewake with `Setsid` and exits, so the space host is not main's descendant (SP9); the space host opens a pseudo-terminal (`host/terminal/pty.go` only), strips `REWAKE_SESSION`, `REWAKE_EPOCH` and the harness's own session markers, and runs an ordinary `rewake <harness> …` in it under main's base and room; it watches main's run once a second (`registry.ObserveRun`) and, when that run has ended, by SIGKILL too, sends SIGHUP, then SIGTERM to the worker's group and SIGKILL after about 5 s, closes the record from the observed end and exits; the record without the keeper's fields; `rewake attach <name>`, a raw relay of the master over a unix socket with raw termios on the client and SIGWINCH forwarded | the workflow scenario with the fixture worker under a private base, the default base untouched; live: from a Claude Code main a Claude Code worker with `--worktree`, and a pi worker once its adapter exists, a task and its report each, `attach` answering a prompt the worker waits on, main closed — `kill -9` included — and the workers gone within seconds |

The restore by session id is the step after S4-7 ([section 8](stage4-space.md#8-restore)).

**What the handlers owe the schema's choices** (the review of S4-2, October 13, 2026). The
generated types say which fields may be absent, not which combinations are valid, so each
handler validates its messages:

- S4-5, refusals: a status other than 200 with `Refusal` is an admission refusal; a lost answer or a server error after an effect proves no absence of the effect, and a retry keeps its identity.
- S4-5, `loadId`: the admitted load is bound to the run the socket and the peer check prove; it never stands in for the peer check of each request.
- S4-5, `role` and `reason` as strings: a value the host does not know never selects a more permissive action.
- S4-6, `Receipt`: the end handler takes a retried end only with the matching `endId`, as its first capture; a turn's start time the mod cannot give stays unknown (zero), never the arrival time, and a pending mark that needs it is recorded as a limit ([the start time](stage4-turns.md#the-start-time)).

## Open questions

Each one a step would otherwise guess; the step named decides it or brings it to main.

- **Q1. The terminal library** — decided by the owner, October 13, 2026: `charmbracelet/x/vt`,
  an exception to the dependency rule; its dependencies, measured at S4-4, are in
  [stage4-space-terminal.md](stage4-space-terminal.md#what-it-brings-in)
  ([the comparison](stage4-space.md#the-terminal-library)).
- **Q2. P4** (S4-1). If `$.http.fetch` does not connect from the harness process, no request
  of the mod can be trusted, and the space waits for another trusted channel. Answered
  positive on October 13, 2026, 2.1.296
  ([the record](../research-claude-mod.md#p4-the-peer-of-the-mods-requests)).
- **Q11. The default room** (the owner's direction, October 13, 2026; [answer
  8](design.md#the-owners-answers)). The worker joins main's room by `--room`; making the
  default room follow main's name without a flag is its own small step, outside this list.
- **Q12. pi's interface as the renderer** (the owner's idea, October 13, 2026): pi's terminal
  interface hosting the pseudo-terminal, with another harness spawned inside it. Recorded as an
  alternative only, while a separate reading of the harnesses runs; the design above does not
  depend on it.
- **Q14. The whole window** — decided by the owner, October 13, 2026: `rewake attach` in a
  second window now, a host mode later as a stage of its own; the external-editor hand-off P15d
  probed is not taken ([design.md](design.md#the-owners-answers), answer 11). The shell replaced
  both on October 19, 2026 ([shell.md](shell.md)).
