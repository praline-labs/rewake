# Stage 4: the first working space — tests, rules and build steps

The second half of [stage4-space.md](stage4-space.md), written October 13, 2026: what the
tests can hold and what only a live check shows, the rules each new behaviour needs before
code, the build steps in order with their acceptance, and the questions a step would
otherwise guess. Every step goes through the chain of `AGENTS.md`: the writer writes, a
review, fixes, and the commit; the steps that touch processes and transport — the terminal,
the hosted launch, the mod's endpoint, closing — also get the Codex side's acceptance.

## 10. Tests

**What the five checks hold, with no harness.**

- `internal/host/space`, the launch: the launcher started from the test binary, the worker's
  wrapper reparented — `proc.Descends` to the test refuses — with the slave side as its
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
  tests use, a request from another process refused, bounds on bodies and waits, a second
  hold answered `duplicate` without a second seat; a `turn/end` whose receipt was lost retried
  with the same end id and taken once; a failed one reported as failed.
- `internal/adapter/claude`: the embedded mod written into the run directory and named by
  `--plugin-dir`, the mod socket's path in the environment, `--session-id` from
  `ConversationNamer`.
- `tools/protogen`: the protocol's generated files equal to a fresh generation, and the
  generator refusing any word of the schema it does not read. The test lives beside the
  generator, since no layer of `internal/` may import a tool.
- The mod itself: a Go test runs `claude plugin validate` and `claude plugin test` on the
  written tree when a Claude Code of at least the minimum version is on `PATH`, and skips
  naming why when not; the mod's own tests (`*.test.ts`) drive the keeper's hold, the
  duplicate, the screen's redraw and the keys with the skill's test kit, as the probe's kit
  did.

**What the fixture column and the workflow suite can hold.** The space host is
harness-neutral, so the fixture's program can be the worker in the pseudo-terminal: a
fixture main runs `rewake space open --harness fixture`, and the scenario checks that the
worker starts in the given directory with a scrubbed environment, registers under its name
in main's room, carries the session id the record holds (the fixture adapter implements
`ConversationNamer`), exchanges a task and its report with main, and ends when main's run
ends, the record closed. The scenario runs under a private, non-default base and checks that
the default base was not touched. The fixture reaches the space host through the neutral seam
and imports nothing of the Claude Code adapter. What stands for the mod is a small program of the suite's,
`test/spacerig`, speaking the protocol over the mod socket as main's harness process would:
it spawns no keeper — the scenario hands it a directory as the keeper's checkout — but it
holds, polls the screen, sends keys and resizes. The fixture adapter offers spaces in the
fixture build, served through the rig; for the peer check the rig is started as the
fixture's harness process, so its requests pass for the right reason. Each scenario has its
control that fails: a worker started with main's variables, a hold seated twice, a worker
alive after main ends.

**The authority across the space** (the acceptance the design review named, October 13, 2026): a
command run by the hosted worker calls main's decision endpoint on a report of the worker's
and main's grant registration directly, once with the worker's environment and once with
main's variables copied in; both are refused. Main's own shell and a tool child of main's
still succeed against the same endpoints. **Main's wrapper killed:** a real-process worker
that ignores SIGHUP and SIGTERM, main's wrapper killed with SIGKILL; SIGKILL is needed and sent
to the harness's group only; within 5 + 2 s and a margin the worker's processes are gone, the
end is recorded by the surviving worker wrapper, and that wrapper is gone after; the next main
start closes the record.

**What only a live check shows.** That `$.http.fetch` connects from the harness process (P4);
the keeper's spawn, its checkout and its removal; the second call's timing; the base reset
and Claude Code's judgment of the checkout after it; the screen repaint by text and its cost
(P15c); keys and the prefix through the real key surface; what the keeper's release achieves
within `session.end`'s 1.5 s; a keeper across `/clear`; a worker's `turn.complete` carrying its
answer. Main runs them, as in the probe.

## Rules before code

A new group, `docs/rules/space.md`, numbered **SP1–SP9**, with `docs/rules_test.go` taught the
group in S4-2. Each rule names its tests as every group does, so a rule enters the group in the
step that builds the test holding it (decided by main, October 13, 2026): SP3, SP4 and SP9 at
S4-2, held in their existing part; SP2 and SP7 at S4-3, SP5 at S4-7, SP6 at S4-8, SP1 and SP8 at
S4-9. Until then a rule stands here.

- **SP1. A space lives no longer than its main.** When main's run ends — by `session.end`, by
  its harness ending, or by its wrapper being killed — every worker of its spaces is ended
  within a bound, by its own wrapper when main's is gone — a wrapper that no signal of that
  shutdown reaches, so it records the end it saw — and every held keeper released or
  gone with main. A conversation that ends while the process goes on (`clear`, `resume`)
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
- **SP5. One seat per space.** The first hold of a space seats its worker; another hold of it
  is answered at once and seats nothing.
- **SP6. Keys reach only a hosted worker.** The space host writes keys into the
  pseudo-terminal it owns and nowhere else; nothing types into a session rewake does not host
  (the project's boundary).
- **SP7. A kept checkout is never forgotten and never removed by rewake.** A closed space
  whose checkout is still there is listed to main at its start and by `rewake space list`
  until the checkout is gone; rewake removes no checkout.
- **SP8. A record left open by a dead main is closed by the next start**, once that run is
  proven dead, with what was seen of its checkout; an unknown liveness leaves it open and
  says so.
- **SP9. A hosted worker is never main's descendant.** Its wrapper is reparented away from
  main's wrapper before it is seated, checked by the same ancestry test the authority makes; an
  open that cannot prove it is refused, and nothing the worker controls runs before that proof.
  So main's decision and grant endpoints refuse the worker's commands by the existing rule, and
  no authority rule changes.

L1 and L3 carry the mod's directory; T10 the worker's build; the existing O2 and the
reporting refusals carry the worker's reports.

## The build steps

Each small enough for one review; each closed by its acceptance check, not by its code.

| Step | What it builds | Acceptance |
|---|---|---|
| S4-1 | P4 and P15c, live, from the probe's kit: the peer of `$.http.fetch` on every request, after a reload and after a keeper's respawn — the kit's existing coverage, not P15b's fetches, which say nothing of their peer — and a shell child's request refused; the screen drawn as styled text in the key surface at 160 by 30, its repaint rate and CPU against P15b's cell grid; a progress signal on the held call, if the skill shows one (Q4); a worker's `turn.complete` and its `answer` | the records in [research-claude-mod.md](../research-claude-mod.md); P4 positive, or the stage stops for a channel design ([design-claude.md](design-claude.md#how-a-tool-call-runs)); P15c picks text or the fallback; P15c also carries the keeper's view cleaned of Claude Code's other rows and the prompt box made useful, since no render site hides it (the owner's questions of October 13, 2026) |
| S4-2 | the rules test's SP group with SP3, SP4 and SP9; the protocol schema, `tools/protogen` and the drift test | `go test ./docs/` and the drift test green; review of the rules |
| S4-3 | `internal/core/space` and `rewake space list`; built October 13, 2026 ([the record](stage4-space-record.md)) | the record tests under the lease; the listing on a hand-written record, closed and kept |
| S4-4 | `internal/host/terminal`, with `charmbracelet/x/vt` (the owner's exception, [section 1](stage4-space.md#the-terminal-library)); built October 13, 2026 ([the terminal](stage4-space-terminal.md)) | the pseudo-terminal tests above, with `-race`; the layout test with the library only in `host/terminal`; `go list -deps` of the binary before and after, recorded with the exception |
| S4-5 | the embedded mod's skeleton and `modhost`: written per run, `--plugin-dir`, the mod socket, `hello` with the peer check — part A, built October 13, 2026 ([loading the mod](design-claude.md#loading-the-mod-for-one-run)); `SpacesCapability` live on main; the spaces offer, its live and refused states and `ConversationNamer` written into the API contract ([design-api.md](design-api.md)) and the catalogue's completeness test, not as scattered optional assertions | live: the hello on the installed Claude Code, a shell child's request refused; the validator test |
| S4-6 | the worker's turn boundary: `turn/start` and `turn/end`, `TurnBoundary{Reports: true}`, the write role admitted on Claude Code when live; the cost of not holding written down ([section 5](stage4-space.md#5-reports)); the receipt's retry with the same end id; the capability withdrawn when the mod is lost | the tests of a failed end and a lost receipt; live: main sends a task to a worker started by hand with `rewake claude --write` and reads its report; a run without its mod refuses the task as today |
| S4-7 | opening a space: `rewake space open`, `events`, the keeper's type, its spawn, its hold and the duplicate, the checkout found and proven the keeper's and empty of work, the base moved, the detached launch with its checks and its gate, the hosted wrapper's process groups, its environment and base, its session id, the record | the workflow scenario with the fixture worker and `test/spacerig` under a private base, the default base untouched; the authority across the space (above); live: the keeper spawned, the worker registered in main's room, its session id in the record, its wrapper outside main's tree; the native cleanup compared with the keeper's original base for an untouched reset checkout and for one with the worker's changes and commits, not inferred from Git status |
| S4-8 | the view: the screen as styled text in the keeper's row, `space/screen`, keys, the prefix and its hint, resize | live: the worker's screen in the keeper's view, a prompt typed there answered, Alt+a c twice ending the worker |
| S4-9 | closing: `session/closing`, the liveness pipe and the worker wrapper's bounded shutdown, a keeper released after its worker's observed end, `rewake space close`, the conversation's end that closes nothing, the next start's closing of a dead main's records, the reminder | the workflow scenario: main's run ends and its worker is gone, the record closed from the observed end; main's wrapper killed with a worker that ignores SIGHUP and SIGTERM (above); Ctrl+C and Ctrl+Z through the terminal; live: `/exit` with a worker seated — the worker ended, what the release achieved within 1.5 s (Q7), an unchanged checkout removed or kept and reminded, a changed one kept and reminded; `/clear` with a space open (Q13) |
| S4-10 | the end to end: main opens a space, sends the worker a task, reads its report, closes main | live, written into the roadmap: the whole target above in one run on the installed Claude Code |

The restore by session id is the step after S4-10 ([section 8](stage4-space.md#8-restore)).

**What the handlers owe the schema's choices** (the review of S4-2, October 13, 2026). The
generated types say which fields may be absent, not which combinations are valid, so each
handler validates its messages:

- S4-5, refusals: a status other than 200 with `Refusal` is an admission refusal; a lost answer or a server error after an effect proves no absence of the effect, and a retry keeps its identity.
- S4-5, `loadId`: the admitted load is bound to the run the socket and the peer check prove; it never stands in for the peer check of each request.
- S4-5 and S4-9, `role` and `reason` as strings: a value the host does not know never selects a more permissive action.
- S4-6, `Receipt`: the end handler takes a retried end only with the matching `endId`; a turn's start time the mod cannot give stays unknown (zero), never the arrival time, and a pending mark that needs it is either served by a contract extended with a reliable start or recorded as a limit.
- S4-7, `Spawn`: exactly one of `agentId` and `error`; `events` acknowledges only what the mod processed, a spawn's acknowledgement is retried idempotently, and the sequence is scoped to the run.
- S4-8, `space/screen`: `after` 0 asks for the whole screen, the first answer is a complete baseline, rows and columns stay in range, and a dead worker carries exactly one of `code` and `signal`.
- S4-8, `space/keys`: the keys are UTF-8 text and control bytes, which is what the key surface sends; raw octets would need another encoding.

## Open questions

Each one a step would otherwise guess; the step named decides it or brings it to main.

- **Q1. The terminal library** — decided by the owner, October 13, 2026: `charmbracelet/x/vt`,
  an exception to the dependency rule; its dependencies, measured at S4-4, are in
  [stage4-space-terminal.md](stage4-space-terminal.md#what-it-brings-in)
  ([the comparison](stage4-space.md#the-terminal-library)).
- **Q2. P4** (S4-1). If `$.http.fetch` does not connect from the harness process, no request
  of the mod can be trusted, and the space waits for another trusted channel.
- **Q3. Text or the cell grid** (S4-1, P15c). The repaint rate and CPU of styled text through
  props at 160 by 30; the cell grid is the fallback.
- **Q4. The keeper's second call** (S4-1). The skill says a stalled agent is retried; whether
  a held call can show progress so that none starts. The `duplicate` answer is built either
  way.
- **Q5. Finding the checkout** (S4-7). That the id in `.claude/worktrees/agent-<id>` and in
  the lock reason is the spawn's `agentId`.
- **Q6. The base** (S4-7). Whether Claude Code judges a checkout whose branch was moved to
  main's commit as changed, and keeps it at the end.
- **Q7. What a release achieves at exit** (S4-9). `session.end` gives its whole chain 1.5 s, so
  the shutdown is the wrappers' and the keeper's release is best effort: whether a worker that
  ends on its hang-up is released early enough for Claude Code to remove an unchanged checkout
  before the process exits, or the checkout stays locked and is reminded. Either way is correct;
  the answer only says which one the person will usually see.
- **Q8. The `events` loop** (S4-7). A loop of polls the mod starts at `session.start` and does
  not await: that it keeps running between hooks, that a reload's new `session.start` starts it
  once again, and that after a `/clear`, which fires no `session.start`, the classic session
  start restores it.
- **Q9. The keeper's model** (S4-7). The cheapest one the person's Claude Code offers, named by
  a launch option rather than in code; or the session's own.
- **Q10. Paste into a space** (stage 6 unless the skill shows a way).
- **Q11. The default room** (the owner's direction, October 13, 2026; [answer
  8](design.md#the-owners-answers)). The worker joins main's room by `--room`; making the
  default room follow main's name without a flag is its own small step, outside this list.
- **Q12. pi's interface as the renderer** (the owner's idea, October 13, 2026): pi's terminal
  interface hosting the pseudo-terminal, with another harness spawned inside it. Recorded as an
  alternative only, while a separate reading of the harnesses runs; the design above does not
  depend on it.
- **Q13. A keeper across `/clear`** (S4-9). Whether Claude Code keeps a subagent's held call
  through a `/clear` or a resume; if not, the space stays open without a view, and whether
  Claude Code then removes an unchanged checkout under a running worker is the failure to look
  for. It guards the build: if the check sees that removal, an open space without a view is not
  enough, and the result goes to main before the keeping of spaces across a `/clear` is landed
  or claimed.
- **Q14. The whole window** — decided by the owner, October 13, 2026: `rewake attach` in a
  second window now, a host mode later as a stage of its own; the external-editor hand-off P15d
  probed is not taken ([design.md](design.md#the-owners-answers), answer 11).
