# A main that compacts and interrupts a Claude Code session — September 24, 2026

Stage 2 of rewake's function-hooks plugin, part A: `rewake compact` and
`rewake interrupt`, the request path they share, and the Claude Code side of it. Stage 1
only listened ([2026-09-23-claude-plugin.md](2026-09-23-claude-plugin.md)); this one acts
on the session. Part B, the Codex side, followed the same day
([2026-09-24-remote-control-codex.md](2026-09-24-remote-control-codex.md)). The design as built is in
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
- When main asked for a compaction itself with `rewake compact`, it gets no "context
  compacted" notice for it. The command's answer carries the tokens before and after
  and the session's compaction count. Compactions the worker makes itself, and ones
  anyone else asked for, keep their notice (September 24, after the live acceptance).

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
the fixture's host did not have, so every case that runs the module had it fail on its
first event and unloaded; the workflow suite was not run before that commit, and the
five checks do not run it. review-claude re-ran `43a1f62` afterwards:
`claude-interrupted` and `stopped-routing` on the Claude Code column were red, six times
`session.start failed … reading every`, while `claude-telemetry` stayed green.

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

**Accepted live.** On September 24, 2026, Claude Code 2.1.280, the installed rewake
built from `e65b865`, in the owner's sessions started through their launch aliases:

- main-claude ran `rewake compact review-claude "<focus>"` on the idle reviewer: exit 0,
  "compacted review-claude: 129381 tokens before, 5949 after".
- It then ran `rewake interrupt write-claude` 8 s into a task: exit 0. At 14:20:27 main
  received the stopped report "main-claude interrupted this turn with rewake interrupt".
- write-claude's next notice ended with "main-claude interrupted your previous turn with
  rewake interrupt."; the notice before it did not. The interrupted turn had reached only
  its first command.
- `rewake list` showed `/ 300K` for both workers after the restart: the context-window
  work of `87fcf54` seen live as well.

Two findings, fixed afterwards in one change:

- `rewake inbox --awaited` showed the interrupted task as `stopped by a person`. The
  listing now prints `stopped:` followed by the stop's own text, which names the main.
  The help, the stopped kind's description and main's playbook line now say that a stop
  comes from the person at the keyboard or from a main.
- main was sent the compaction notice for the compaction it had asked for; the owner's
  decision above followed. The module tells rewake who asked (`compact.asked`, with the
  request id), the collector gives that mark to the next `PostCompact`, main's wrapper
  skips a compaction that names it, and the command waits up to 3 s for the telemetry
  to count its request and prints "(compaction N)". `claude-steered` gained two checks:
  the count in the answer with no notice to main, and the awaited list naming main. It
  also gained four mutants: a wrapper announcing main's own compaction, an answer
  without the count, a module that does not tell who asked, and the old awaited
  wording.

**Review of that fix.** review-claude reviewed the uncommitted tree with a live run on
2.1.280 the same day: main's `rewake compact` answered "(compaction 1)" and main got no
notice; a worker's own `/compact` kept its notice; `--awaited` showed the right stopped
text after main's interrupt and after a keyboard Esc; `$.process.run` does wait for the
process to close, in the binary. Four findings, with main's decisions:

- The fix claimed to tie the compaction to the request by identity, not by timing, but
  the collector gives the mark to the first `PostCompact` it gets after it. A typed
  `/compact` leaves no turn for the module to see, so a request could start tens of ms
  after its end, and that compaction's late `PostCompact` would take the mark. Inferred
  from the source, not reproduced. Fixed: the mark is now sent from the module's
  `session.compact` handler for its own call, trigger `plugin`, which holds the
  compaction until it is sent. After the end of a compaction the module did not ask
  for, it waits a second before it asks the host — waiting, not refusing, since the
  host already refuses while the other runs. The claims now say what the code
  guarantees, and the remaining window is a known limit.
- The refusal was reported without waiting; it is now sent before the answer is
  written.
- The host's refusal while another compaction runs, "a turn is in flight; the
  conversation compacts between turns", seen live during a typed `/compact`, went out as
  `failed`; it is now `in a turn`.
- "Not enough messages" comes after `PreCompact` and no `PostCompact` follows, so the
  listing read `idle; compacting` until the next turn; the refusal now ends
  `compacting`. The fixture refuses a short conversation in that order too.

**Live re-review of `92e84a8`.** review-claude ran it on 2.1.280 the same day: the mark
never went out. The host skips a plugin's own handlers for an event its own code
raised — `hooks module rewake@inline session.compact skipped: re-entry (the plugin's
own code raised it; origin rewake)` — so main got the notice for its own compaction,
the answer came without the count after the 3 s wait, and a "Not enough messages"
refusal sent no `compact.refused`, leaving `compacting` set. Both hosts had run the
module's handler for its own compaction, softer than the harness. The wrapping itself
and the second waited out after another compaction's end were right. Fixed:

- The module sends the mark from the request again, before it asks the host and after
  the quiet second, while no turn runs; a refusal is sent before the answer and says
  whether the host had started. The collector ends `compacting` for a started one, and
  if its `PreCompact`, a background hook, comes after the refusal, that starts
  nothing. The handler serves only the quiet second.
- Both hosts skip the calling plugin's handlers for the compaction its own call
  raised, and a test holds each of them to it.

Seen live by write-claude after the fix, September 24, 2026, Claude Code 2.1.280, in a
private HOME against the stand-in API, a main and a worker started through rewake:

- Before the worker's first turn, `rewake compact` answered `nothing to compact`, exit
  1; the worker's debug log showed the mark sent, `session.compact skipped: re-entry`,
  `PreCompact`, then the refusal sent; `compactionInProgress` read false afterwards.
- After one turn: "compacted w1-claude: 60 tokens before, 1233 after (compaction 1).",
  exit 0, 0.2 s; no "context compacted" notice reached main.
- `/compact` typed in the worker's terminal, and `rewake compact` at once after it: the
  typed one ended at 12:45:39.035 (UTC, its `PostCompact`), the module sent the mark at
  12:45:40.041, a second later; the answer was "(compaction 3)", and main got "Rewake:
  context compacted (compaction 2)." for the typed one only.

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
- A typed `/compact` refused as "Not enough messages" leaves `compacting` set until the
  next turn: its `PreCompact` runs and nothing after it, and rewake hears no refusal of
  a compaction it did not ask for. Seen live on September 24, 2026.
- A `PostCompact` more than a second late after a compaction the module did not ask for
  is still taken for main's ([remote-control.md](../remote-control.md#known-limits)).
- A compaction whose `PostCompact` never reaches the collector leaves the mark for the
  next compaction, which then counts as main's
  ([remote-control.md](../remote-control.md#known-limits)).
