# A main that compacts and interrupts a Claude Code session — September 24, 2026

Stage 2 of rewake's function-hooks plugin, part A: `rewake compact` and
`rewake interrupt`, the request path they share, and the Claude Code side of it. Stage 1
only listened ([2026-09-23-claude-plugin.md](2026-09-23-claude-plugin.md)); this one acts
on the session. Part B, the Codex side, is next. The design as built is in
[remote-control.md](../remote-control.md).

**The owner's decisions**, recorded where they apply:

- Only a session of role main may call these commands (September 24).
- A compaction is refused at once while the worker is in a turn; it never waits for idle
  and never happens on its own (September 23).
- The behaviour is the same and predictable on both harnesses (September 24).
- A focus for a Codex compaction is refused for now: `thread/compact/start` has no field
  for it, and `compact_prompt` replaces the whole prompt for the whole conversation. A
  proper route and an emulation through the conversation's history are to be researched
  later (September 24; [work-queue.md](../work-queue.md)).
- After an interrupt from main, the Claude Code worker's next rewake notice carries a
  line saying so, once, so that its model sees roughly what Codex's does (September 24).

**The research.** review-claude probed Claude Code 2.1.280 live against a stand-in API
and read the binary and the Codex schemas the same morning; the facts are in
[research-claude-actions.md](../research-claude-actions.md#compaction-abort-polling) and
[research-protocol.md](../research-protocol.md#compaction-and-interrupt-on-request):

- `$.session.compact()` or `({ instructions })` compacts an idle session, runs the
  plugin event `session.compact` and the hooks `PreCompact`, `SessionStart` with source
  `compact` and `PostCompact`, and rewake's telemetry counts it; mid-turn the host
  refuses it at once and atomically, and the turn goes on.
- `$.turn.abort({ turnId })` ends the running turn with `turn.complete` reason `aborted`,
  leaves the prompt in the conversation and marks nothing for the model.
- A module polls a file with `$.clock.every` and `$.fs.exists`; a `$.fs.read` of a
  missing file is logged as an error every time.
- On Codex, core `compact()` aborts a running turn rather than refusing, so the wrapper
  must refuse itself; `turn/interrupt` answers only after the turn is aborted, and Codex
  records `<turn_aborted>` in the history.

**What was built.** Part A1 is `43a1f62`:

- The two commands in the command table under "STEER A SESSION", checked whole before
  anything is written: the caller must be main, the target a live session of the room
  whose harness takes control requests (`harness.Steerable`), a focus only where the
  harness can pass one (`CompactFocus`). Every wrong call is exit 2 and writes nothing.
- A control directory per run, made by the wrapper only for a harness that serves it,
  and the protocol in `internal/control`: a lock, a request put in place by rename, a
  mark when it is taken, a result written once, the asker owning every removal. Giving
  up withdraws the request before the last look for the mark, and the served side checks
  the request is still in place after marking it, so `not answering` is never reported
  for a request that was carried out. An asker cut short by a signal withdraws on the way
  out.
- rewake's module polls the directory four times a second, compacts or aborts, and
  writes back numbers and the host's text only. The host's refusals map to `in a turn`,
  `compaction switched off`, `nothing to compact` and `no turn running`.
- An aborted turn the request asked for is reported with who asked: main reads `stopped`
  with "`<main>` interrupted this turn with rewake interrupt", and the worker's next
  delivered notice ends with "`<main>` interrupted your previous turn with rewake
  interrupt.", once.
- Unit tests for the protocol in every order, the commands and their refusals, the
  wrapper's directory, and the module under node against a strict host.

Part A2 is the workflow case `claude-steered` with seven product mutants
([testing-plugin.md](../testing-plugin.md#steering-a-session)). For it the fixture's
plugin host now serves `$.clock.every`, `$.fs`, `$.session.compact` and `$.turn.abort`
as strictly as 2.1.280, the last two through a channel back to the session, which
compacts with the harness's hooks or ends a turn it holds open. The seven mutants
include a module that never asks the host to compact, one that drops the name of the
asker, and a command that lets any role steer. The mutant for the one-shot line had to
remove both of its guards. Told alone is never reached in the fixture: the next turn's
start lays the mark aside before a second notice is composed.

A2 also mended the suite. A1's module calls `$.clock.every` in `session.start`, which
the fixture's host did not have. Read from the code, not re-run on `43a1f62`: every case
that runs the module then had it fail on its first event and unloaded, and the workflow
suite was not run before that commit. The five checks do not run the suite.

**The live probes and reviews.**

- review-claude ran a snapshot of the product code taken at 11:48 on the real 2.1.280.
  All eight scenarios behaved as designed: an idle compaction with a focus in 0.24 s,
  counted; a busy one refused, the turn going on; an interrupt of a busy session giving
  main the stopped text and the next notice the line once; an idle interrupt refused; a
  concurrent request refused; a `--bare` session `not answering` at 5.06 s; a session on
  an older binary refused in 0.04 s; a hot reload not re-running a request. The role
  gate, the directory's permissions, the exit codes, `--json` and the absence of the
  summary held. Its findings, fixed in A1:
  - A pickup race: the module could read a request as the command gave up and carry it
    out after `not answering` was reported. It led to the withdraw-then-look protocol.
  - A session without a directory got the hint for a missing module. It now gets its own
    next step, a restart with the current rewake.
  - The help text said more of Codex than part A does.
  - "Not enough messages to compact" was a failure. It is now the refusal
    `nothing to compact`.
  - A hot reload mid-turn loses the tracked turn. It is documented as a limit.
- review-claude then reviewed A1 whole, live on 2.1.280 from the working tree, with no
  blockers. The protocol was honest in five forced orders:
  - the harness stopped past the deadline;
  - the mark written after the last look;
  - the mark written in the withdrawal window;
  - the mark written before the removal;
  - a new request in place of the old one.

  `not answering` never came on an executed request, and there was exactly one
  compaction where there should be one. "Not enough messages to compact." was seen live,
  with the plugin's name in front. Of 22 mutants, 19 were killed. The fixes, before the
  commit:
  - `lane.go` split by subject;
  - a test that a request stays in place while it is served, which kills the mutant that
    removed it at the mark;
  - the research's live tag and the refusal prefix;
  - a fact of `design.md` restored and the roles split out into `roles.md`, rather than
    paragraphs reflowed to fit;
  - the directory made only for a harness that serves it;
  - the command's wait cut short by SIGINT and SIGTERM;
  - `not answering` described as it is, and main's interrupt added to `flow.md`;
  - a test that the wrapper removes the directory.
- review-claude reviewed the snapshot of `43a1f62` and probed an Esc live. Six of the
  seven fixes were as claimed, and the mutants that had survived for the request left in
  place and the directory's removal were now killed. The seventh, the signal handler,
  had no test at the command's level: a mutant that waited on a plain context survived.
  The probe found what main's Esc does to a command it runs: SIGTERM to the command's
  whole tree on Esc and on Ctrl+C alike, never SIGINT, and SIGKILL 1.5 s later
  ([research-claude-control.md](../research-claude-control.md#a-second-probe-signals-and-hooks-around-an-interruption)).
  Fixed after A2, in a commit of its own:
  - a test that sends the command SIGTERM during the pickup and expects the request
    withdrawn;
  - a reason of its own, `cut short`, whose hint says to ask again, instead of a
    `not answering` that sent the caller looking for `--bare` on the target;
  - the refusal quotes with their prefix;
  - stale pointers from the code to the research;
  - the SIGSTOP entry: the missing mail inferred, not seen, and the wrapper's own
    `SIGCONT` as the likelier cause.

**What stays open.**

- Part B, the Codex side: the wrapper serves the same directory, refuses a compaction
  itself while a turn runs or a `turn/start` is in flight, and sends
  `thread/compact/start` and `turn/interrupt` ([work-queue.md](../work-queue.md#now-stage-2-of-the-plugin-the-rest)).
- A focus for a Codex compaction, to be researched as the owner decided.
- An asker killed outright with SIGKILL cannot withdraw its request: a target stalled at
  that moment carries it out when it resumes, telling nobody
  ([remote-control.md](../remote-control.md#known-limits)).
- Found by review-claude outside part A and not yet explained: after a SIGSTOP of the
  harness the wrapper stays stopped while the harness runs again, and by its state it
  would deliver no mail until someone continues it; the wrapper's own `SIGCONT` to the
  harness is the likelier cause
  ([intermittent-bugs.md](../intermittent-bugs.md)).
- The fixture's threshold for "Not enough messages to compact" is its own, and a focus
  reaching the summary request is shown only live; the workflow case says so.
