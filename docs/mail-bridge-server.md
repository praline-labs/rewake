# The server of the mail tool

The second of three stages of [mail-bridge.md](mail-bridge.md): the local stdio MCP
server the harness starts outside its sandbox, which runs the CLI of
[mail-bridge-cli.md](mail-bridge-cli.md) once per tool call, and the wrapper's context
endpoint that tells it who is calling. **Built and accepted October 1, 2026; the launch injection of stage 3
is built and its code accepted, live checks pending, and no harness starts it until that stage's gates G2 and G7 close**
([mail-bridge-launch.md](mail-bridge-launch.md#gates)). This
document holds the rules and one call's path; what happens after a call — the
acknowledgment of a read, the turn's end, a pending mark, the waits and their bounds — is
in [mail-bridge-turns.md](mail-bridge-turns.md). The rules come before the code, because
stage 1 converged only once its rules were agreed first and its completeness was checked
by construction, not by lists. The second pass of October 1 rewrote them after the first
review; what each finding met, the changes to stage 1, the tests and the live checks left
to stage 3 are in [mail-bridge-checks.md](mail-bridge-checks.md).

Two decisions of October 1, 2026 shape the stage. The endpoint, the tickets, the observer
and both harnesses' adapters belong to stage 2, since without them the server runs no
call; stage 3 injects the configuration at launch and runs the live checks. And the same
words from a later turn do not run beside an operation of theirs that is still unfinished
or whose effect is unknown: they stop at it as the shell's do.

## The rules

1. **The server is a transport.** It decides no mail: it checks the words on the
   tool's surface, gets a ticket, runs one child of its own image with those words, and
   returns that child's answer. It keeps nothing a later call needs. What any call did
   lives in the CLI's receipt journal and nowhere else, so a server that dies, restarts
   or runs twice loses nothing.
2. **A call runs only under a ticket the wrapper issued after it saw the same call
   natively.** The ticket names the native conversation, turn and call, the time, the
   deadline and the digest of the normalized words. Nothing the model supplies — a
   word, an argument, a `_meta` field it could shape — is authority on its own. A call
   the wrapper cannot match runs nothing, and one native call gets one ticket, whichever
   server connection asks for it.
3. **Every effect happens in the child, under its receipt; the server never repeats,
   finishes or undoes one.** It does not kill a child on a cancellation, does not retry
   a child that failed, and does not start a second child for a call whose first is
   still running.
4. **A call is bound to its operation before its first effect.** Every path of the CLI
   that holds a receipt — a new operation, the same words joining one, `retry`,
   `inbox --next` — writes the call's binding as soon as it holds the record, before any
   step: a file naming the record, keyed by the native call. So the binding's absence,
   read as "no such file", proves the call made no effect; its presence names the
   operation whose outcome is then unknown. "Nothing ran" is said only on that proof,
   or when no child ran, and only then may the answer send the same words to the shell.
5. **Everything read and written is bounded, in one place each.** A frame the server
   reads is bounded before it is parsed, and a request whose id the answer could not
   carry within the bound runs nothing. Every message written goes through one encoder,
   which checks the whole JSON-RPC message against `bridge.ResultCap`. The server never
   clips what a child printed: an answer that does not fit is replaced whole.
6. **A part is read only on its call's own answer.** A part counts as shown only when the
   result the harness recorded for that native call is exactly the answer the child
   recorded before printing it, and that result is direct, successful and inside the
   calibrated bound. Text that merely contains a letter's bytes proves nothing, and
   anything the server writes in place of a child's answer is an error that matches no
   record.
7. **A call's commits stop at its turn's end.** A pending mark checks, under the mailbox
   lock and before it marks, for an end of the run on record at or after its time, and
   is written only by an attempt that runs in its operation's own turn; an absence — of
   a later turn's start, of an end — never proves that turn open. A read's
   acknowledgment checks the same record and, in the wrapper that runs it, the ends the
   wrapper has captured and not yet recorded. Found, nothing is committed: the mark is
   finished as not made, and the letter shows again.
8. **An end's boundary is a cut between commits.** It is taken at the end's own event,
   as stage 1 takes it, from one of two moments: the clock as the end is noted, when no
   acknowledgment is between its check and its close; or the snapshot the one that is
   takes after its last write, still holding the mailbox lock. Never from a sample that
   could follow that lock's release. A budget bounds only what a commit waits for before
   its check, never a write it began. Each acknowledgment is applied once and never
   retried.
9. **Every wait has a bound, and nothing waits holding what it waits for.** A wait that
   runs out leaves a defined outcome — no ticket, no acknowledgment, a child killed —
   and never a later effect on something already decided. One exception, as in stage 1:
   a write already begun under a held lock is not timed, and an end captured while an
   acknowledgment writes waits for its close.
10. **One build per run.** The wrapper, the server and the CLI child of a run are the
    same build. The server execs its own image, never a path looked up again. A server
    or child of a different build is refused, before anything else happens.
11. **A failing tool never weakens the mail.** When the tool fails, the shell takes the
    same words under the same receipts. The surface, the read boundary and the refusals
    of the CLI stay as they are. The wrapper reports what it saw of either channel, each
    change once.

## Where the rules live

The server is `internal/bridge/server` (`bridge-serve`, an internal command of
`internal/cli/registry_internal.go`), the endpoint `internal/bridge/endpoint`, run by the
wrapper in `internal/wrap/mailtool.go`; the CLI side of rules 4, 6 and 7 is stage 1's code.

| Rule | Where |
|---|---|
| 1, a transport | `server.go`: the loop, four slots, nothing kept between calls |
| 2, a ticket per native call | `endpoint/input.go`: the neutral input every harness's observations reach — a turn started or ended, a call seen, a call's result, a startup failed — which the Codex parser and the Claude Code hook path (`endpoint/events.go`) call; `endpoint/calls.go`: `observe`, `issue` (one per call, kept in `spent` for the run; the turn open, its time from `endpoint/gate.go` `Stamp`), `confirm` (once, before the deadline) |
| 3, effects in the child | `child.go`: one child per call, never restarted or killed on a cancellation |
| 4, the binding first | `cli/inbox_parts.go` (`lockOperation`), `receipt/binding.go`; `child.go` `afterDeath` reads it |
| 5, bounds | `frames.go` (frames, ids), `encoder.go` (the one writer of stdout), `child.go` (`capped`) |
| 6, a part on its answer | `endpoint/events.go` (`resultOf`, each harness's result decoded), `endpoint/input.go` (`exposure`: the first text and the whole result's size; `complete`: the first result of a call only), `cli/read_ack.go` (`answeredBy`, digest) |
| 7, commits stop at the end | `cli/pending_turn.go` (`judgeMark`), `cli/read_ack.go` with `endpoint/gate.go` (`Enter`) |
| 8, the boundary a cut | `endpoint/gate.go` (`Capture`), taken by the Codex gateway (`event_stream.go`) and Claude Code's collector (`collector_turns.go`) |
| 9, bounded waits | `endpoint/calls.go` (the observation's wait), `cli/read_ack.go` (`ackBudget`), `child.go` (the hard bound, and `reapWait` past it once stdin ended), `server.go` (`outputWait`) |
| 10, one build | `endpoint/endpoint.go` (`admit`: uid, capability, descent, build); the child execs `/proc/self/exe` |
| 11, the shell stays | the CLI is unchanged under the shell; the channel record is stage 3 ([below](#fallback-and-what-main-sees)) |

The tests are listed in [mail-bridge-checks.md](mail-bridge-checks.md#how-the-checks-are-built).

## Who calls

### What the server knows at start

The harness starts `<rewake executable> bridge-serve`. That is an internal command,
outside the tool's surface since it is not in `toolFlags`. Stage 3 passes the server
these launch values: `REWAKE_DIR`, `REWAKE_ROOM`, `REWAKE_SESSION`, `REWAKE_EPOCH`, and
`REWAKE_BRIDGE_CAPABILITY`, the per-launch secret.

On Codex the values travel in the server table's `env`, since Codex hands the server
nothing else but `HOME LANG LOGNAME PATH SHELL TERM USER` (probe 1). On Claude Code they
travel in the injected `--mcp-config`'s `env`. There the server also inherits the
harness's whole environment, and reads none of it, so it never takes the harness's
conversation id from it: probe 2 found `CLAUDE_CODE_SESSION_ID` stale after a
conversation change.

At start the server connects to the endpoint, `sock/<name>.<epoch>.ctx` under the state
directory, with the digest fallback past 103 bytes, and says hello with the capability,
its build stamp, its pid and its boot-clock start. The wrapper accepts the hello only
when the peer's uid is its own (`SO_PEERCRED`), the capability and the build match, and
the peer descends from the harness process this wrapper started. The connection stays
open for the server's life, so its close is how the wrapper learns that the server died.
These checks keep two runs from crossing wires. They do not stop someone running as the
same user, who can read the capability from a command line; mail through the shell has
the same reach, and the specification draws the boundary there.

### One call, step by step

1. **The call arrives.** A `tools/call` for `rewake` comes in with `{"words": [...]}`
   (bounds below). The server takes the boot-clock time it arrived and checks the
   arguments' shape: a single key, an array of strings. It then runs `cli.ToolWords`,
   which returns the normalized words or the CLI's own refusal. A refusal ends the call
   here, with nothing run.
2. **The server asks for a ticket.** Over its endpoint connection it sends the transport
   (`codex-mcp`, `claude-mcp`); the native ids from `_meta` — on Codex `threadId`,
   `callId` and `x-codex-turn-metadata.turn_id`, on Claude Code `claudecode/toolUseId`;
   the normalized words and their digest; the raw arguments; and the arrival time.
3. **The wrapper matches the request with its own observation of the same call**
   (below). It waits up to two seconds for that observation, because neither harness
   orders the two. On Codex, probe 2 saw the server's `tools/call` land one millisecond
   before the gateway saw `item/started`, in one of eight calls. A match issues a ticket
   and consumes the observation, so a second request for the same call, on this
   connection or a restarted server's, is refused. A request with no observation by then
   is refused.
4. **The server starts the child** with the ticket on an inherited pipe (fd 3,
   `REWAKE_BRIDGE_TICKET_FD`), stdin from `/dev/null`, in its own process group. Its
   environment is built only from the launch values, plus `PATH`, `HOME` and `LANG`.
5. **The child authorizes the call** as stage 1 built it: it checks the words again,
   checks the ticket's digest, and has the wrapper confirm the ticket. The confirmation
   is one-time, by the ticket's nonce; the wrapper records which process used it (pid and
   start, through `SO_PEERCRED`) and refuses it again.
6. **The child runs the command.** Every path that holds a receipt takes the record's
   lock through one function (`lockOperation`), and that function writes the call's
   binding right after the lock is held (rule 4, below). Then the command runs exactly
   as stage 1 built it.
7. **The server encodes the answer** (stdout, stderr, exit code) through the one
   encoder, and writes it.
8. **The observer settles the call** when the harness records the result
   ([mail-bridge-turns.md](mail-bridge-turns.md)).

### What the wrapper matches

| Field | Codex: gateway sees `item/started`, `mcpToolCall` | Claude Code: PreToolUse hook |
|---|---|---|
| conversation | `threadId`, equal to the run's primary bound thread | `session_id`, equal to the conversation the collector holds |
| turn | `turnId`, an interval the gateway saw start and not end | the hook's own `prompt_id`, with the latest end on record ([turn identity](mail-bridge-turns.md#the-turn-a-call-belongs-to)) |
| call | `item.id` = `_meta.callId` | `tool_use_id` = `_meta["claudecode/toolUseId"]` |
| tool | `server` = `rewake`, `tool` = `rewake` | `tool_name` = `mcp__rewake__rewake`, `mcp_server` = `rewake` |
| words | `arguments` normalized, digest equal | `tool_input` normalized, digest equal |

On Codex the request's `threadId` and `turn_id` from `_meta` must equal the
observation's; a request whose `threadId` is absent or not the primary thread is refused
before any wait, since the gateway forwards no other thread's items and the wait could
only time out (revised after the live checks of October 4, 2026, and built the same day:
the primary comes from the backend's `Thread()`, so while a selection is pending every
Codex call is refused this way). Missing, conflicting or duplicate correlation refuses: a second
observation with the same call id; a call this run already gave a ticket, however long
ago; a thread other than the primary one, which covers a sub-agent's thread; a Codex
turn already completed, or one whose end was captured since the call was heard or the
turn started; on Claude Code, a call heard before an end the wrapper captured, and a
hook input that names an agent (the companion says what stays unavailable there until
stage 3).

The table of calls is bounded — 512 calls, each forgotten two minutes after it was first
heard — but the calls given a ticket are kept apart for the whole run, so a call heard
again after the table forgot it is never taken for a new one. A run keeps at most 65,536
of them; past that every new call is refused, its words named for the shell, rather than
one forgotten and served twice.

A Claude Code PreToolUse hook is a synchronous command (`rewake bridge-hook`), never
`async`: the harness runs the call only after the hook exits, and the hook exits once the
wrapper recorded the observation, so the observation is in place before the call reaches
the server (6 ms in probe 2). A hook that cannot reach the wrapper within its bound exits
0 and prints nothing. It changes no permission decision; the call then finds no
observation and runs nothing.

### The ticket

The wrapper fills `bridge.Ticket` with the capability; the conversation, turn and call as
matched; `CalledBoot`, the time it issues the ticket on the boot clock, inside the call
and so inside the turn; `DeadlineBoot`; the digest; the transport; the transport's
declaration, as its request made it, that its turn ids are never reused
(`TurnsNeverReused`, which the receipt of an operation the call begins keeps too —
[who may mark](mail-bridge-turns.md#a-pending-mark-at-its-turns-end)); and a one-time
nonce, a field stage 1 does not have yet. The ticket validates once, for one process, and never
after its deadline.

`CalledBoot` is what an acknowledgment's check compares with a noted end, so the wrapper
takes it under the gate's mutex, the one a capture notes its end under, and only while
no end was noted since the call was heard — on Codex, since its turn started. A call
heard before an end and asked for after it gets no ticket: the gateway captures an end
before `turn/completed` reaches the table, and a capture on `thread/status/changed` may
have no `turn/completed` at all, so the open turn alone cannot say the turn is over. A
capture after the ticket notes a later moment, and the call's acknowledgment then writes
nothing ([the end meets its calls](mail-bridge-turns.md#a-turns-end-meets-its-calls)). Tickets live in the wrapper's memory: the wrapper lives exactly as
long as the run, and a ticket means nothing to another run.

The deadline is `CalledBoot` plus 25 seconds. On Codex, stage 3 sets the transport's
own `tool_timeout_sec` to 30, so the deadline falls inside it. On Claude Code the
transport's timeout is the person's `MCP_TOOL_TIMEOUT`, read from the wrapper's own
environment at launch — the adapter names the variable (`ToolTimeoutVariable`), and
`DeadlineFor` reads it; when that is shorter, the deadline is that timeout less two
seconds. A deadline under five seconds means the tool reads nothing: the call is
refused, and the reason names the setting. That is stage 2 as built; stage 3 replaces
the launch-time reading with the value each call's hook observes and our server's own
`timeout`, as [mail-bridge-launch.md](mail-bridge-launch.md#claude-code) lays out, and
the wrapper's inherited environment then decides nothing. Stage 1 already cuts `send --wait` to the
deadline less half a second, and the heads-up's lock wait to the deadline.

## Running the child

- **The image.** The server execs `/proc/self/exe`, its own image, even when the file at
  the launch path was replaced since. So the server and its child are one build by
  construction, and the hello carries that build to the wrapper.
- **Concurrency.** At most four children run at once. A fifth call is refused with
  "busy", nothing run.
- **The binding.** `receipts/<epoch>/calls/<call>`, where `<call>` is the hex SHA-256 of
  the transport, the conversation and the native call id, holds the token of the record the call holds.
  It is published exclusively — a native call has one ticket and so one binding — right
  after the record's lock is taken and before the first step, by every path: a new
  operation (after `receipt.Begin`), the same words joining one, `retry` of a heads-up,
  a pending mark or a read, and `inbox --next` of a read. A replay writes it too, though
  it changes nothing. A path that holds no record — `whoami`, `--peek`, `--owed`,
  `--awaited`, help, a kept output's next part — makes no effect, and has none. The
  binding goes with its record at the sweep. One binding has two readers: the server
  after a child's death, and the observer, which resolves a call to its read from it.
- **The outcome after a death.** A child that dies without an answer is looked up by its
  binding, which the server can name since it holds the ticket:
  - **The binding:** the call held that operation, its outcome is unknown, and the answer
    names its token.
  - **"No such file":** this call made no effect. A record its `receipt.Begin` opened
    before the binding is no effect: the same words in this turn join it, and the
    shell's same words stop at it (exit 3) and name the `retry` that finishes it.
  - **Anything else:** unknown, with no token. The answer names the same words in this
    turn, which join whatever this call began.
- **The hard bound.** A child still alive five seconds past its deadline is killed with
  SIGKILL, and its process group with it. Its locks are `flock`s, so they die with it
  (rule 2 of stage 1 stays: no lock file is removed by anyone else). A child that does
  not die — or something of it that holds its output past the kill — keeps its slot while
  the server serves, so at worst the tool answers "busy". Nothing else waits for a
  child: neither the observer nor a turn's end does (rule 7).
- **Cancellation.** `notifications/cancelled` drops the response and leaves the child
  alone. What the child commits after its turn's end is on record is refused by that
  record (rule 7), and its receipt holds the outcome for the retry.
- **The harness goes away.** On EOF from stdin, the server accepts no new call, waits for
  its children up to their hard bounds, kills what is left, and exits. A killed child
  not reaped one second past its kill, or past the EOF when the kill came first, is left
  behind: its call answers that the command outlived its kill and its outcome is
  unknown, naming its token when the binding has one and the same words in this turn
  otherwise — never that nothing ran, since a child still running may yet bind. The
  journal decides it, as after any death. A server killed
  outright leaves its children running; each ends by its own deadline check before
  commit, and its final write to a dead pipe fails after the effect is journaled.
- **The harness stops reading.** The server reads stdin apart from writing stdout, so a
  full stdout never hides the end of stdin, and up to 16 frames are read ahead of the
  dispatch. While stdin is open and its input does not back up, an answer that waits on
  a full stdout only waits — a client paused for a while gets it whole. Once stdin
  ended, or while a frame waits behind output that has not moved, output stuck for 2 s
  ends the server as one killed outright: its calls stay open, their effects in their
  receipts, and an answer that never left is no read — no completion follows it, and
  the letter shows again. A call dispatched after the server began to end runs nothing.
  A message is at most the result cap, 4 KiB, so a client that reads at all moves the
  output well within the 2 s.

## The answer

A result is `{"content":[{"type":"text","text":<stdout>}], "isError":<bool>}`. Two more
text items are added when needed: the stderr, when there is any, and `exit status N`,
when N is not 0. `isError` is true when no child ran, when the child exited non-zero, and
whenever the server writes anything in place of a child's answer. The child's stdout
goes out byte for byte or not at all.

**Bounds on what comes in.** A frame is one line of at most 256 KiB — the words' own
32 KiB, escaped, with room to spare; a longer line is read to its end and discarded. A
request id must be an integer or a string of at most 128 bytes encoded. Either failure
answers a JSON-RPC error with id `null`, which the harness cannot match to its call, and
runs nothing: the harness then times the call out, and the model sees a transport error,
never an effect. Both harnesses number their requests with small integers (probes 1
and 2).

**The bound on what goes out.** `bridge.EncodedSize` counts stdout and stderr as JSON
strings plus 256 bytes of framing, and the id's 128 bytes fit inside the framing. The
encoder checks the whole message — envelope, id and items — against `ResultCap`. A
message over the bound can come only from a child that broke its own bound; it is
replaced by one line naming the shell, with `isError`, and matches no record (rule 6).

Errors split in two:

- **JSON-RPC errors**: an unknown method, a tool other than `rewake`, arguments of the
  wrong shape, a call before `initialize`, a frame or id over its bound.
- **Tool results with `isError`**: everything else, including every refusal on the
  surface. They read like the CLI's refusals, cut to 1 KiB by the same `cutDiagnostic`.

## Failure points of a call

What each point proves, leaves unknown, and what the call answers. "Same words" means the
same words in the same turn, which join the same receipt (stage 1). The points after the
answer — completions, acknowledgments, ends — are in
[mail-bridge-turns.md](mail-bridge-turns.md#failure-points-after-the-answer).

| Point | Proven | Unknown | The call answers | Then |
|---|---|---|---|---|
| arguments of the wrong shape, an unknown tool | nothing ran | — | JSON-RPC error | the model fixes the call |
| a frame or id over its bound | nothing ran | — | JSON-RPC error, id `null` | the harness times the call out |
| words off the surface | nothing ran | — | the CLI's refusal, exit 2 | not a channel failure: the shell gets no route around it |
| no endpoint, no hello, a build mismatch | nothing ran | — | "nothing ran; same words in the shell" | channel: tool failing |
| no observation within 2 s, conflicting ids, a completed Codex turn, an agent's call, busy | nothing ran | — | the same, naming the reason | an observation that comes later expires unused |
| the child cannot start | nothing ran | — | the same | channel: tool failing |
| the child's ticket refused (used, expired, unknown) | nothing ran: no binding | — | the child's own refusal | — |
| the child exits with an answer | as the CLI says | as the CLI says | the answer | parts wait for their evidence |
| the child dies, its binding found | the call held that operation | its effect | "outcome unknown; `rewake retry <token>`" | the receipt decides |
| the child dies, its binding proven absent | this call made no effect | — | "nothing ran"; the shell may take the same words | a record it opened stops the shell's same words (exit 3) |
| the child dies, its binding unreadable | — | whether it began | "outcome unknown; same words in this turn" | the same words join it |
| a deadline passes, then the hard bound | — | the effect, until the child is gone | as the death above | the CLI checked the deadline before commit |
| stdin ended, the killed child not reaped a second past its kill or the EOF | — | the effect, and whether the child still runs | "outlived its kill, outcome unknown"; its token when bound, else the same words | the server exits without it; the receipt decides |
| the harness times out first | — | the outcome, at the model | the harness's own timeout | the same words join; a joined read replays its batch |
| a cancellation | — | the outcome | nothing (dropped) | the same |
| the server dies mid-call | — | the outcome | the harness's "transport closed" | the child finishes; same words or `retry` find it |
| stdout stuck 2 s once stdin ended or input backs up behind it | — | the outcome | nothing: the server exits | as the server dying mid-call; the unwritten answer reads nothing |
| the server dies between calls | — | — | the next call: Claude Code restarts it; Codex says closed for the run | channel: tool unavailable |
| the wrapper dies | new calls cannot be authorized | the effect of a child already authorized | refusals: no endpoint | that child runs on to its deadline; its outcome is in its receipt, or unknown |
| the wrapper and the server die together | the same | the same | "transport closed", or nothing | the same |

A command that holds no record makes no effect, so its death leaves nothing unknown. The
fault test proves the row "binding proven absent" by construction: in every path, no
write that is an effect comes before the binding.

## Fallback and what main sees

**Moved to stage 3, decided October 1, 2026:** the channel record, its notices to the
worker and main, and its display in `whoami`, `rewake list` and main's header are built
with the injection, since until then no harness starts the server. They are designed in
[mail-bridge-channel.md](mail-bridge-channel.md). Stage 2 keeps the server's own
outcomes — a tool call that ran, one refused, a transport that failed — as defined
results that record will read.

## What the server refuses

- Every method but `initialize`, `notifications/initialized`, `ping`, `tools/list`,
  `tools/call` and `notifications/cancelled` (no resources, prompts or sampling); a
  tool other than `rewake`; arguments other than one `words` array of strings; a frame
  or id over its bound.
- Words off the surface: stage 1's `cli.ToolWords`, run again by the child.
- A call with no native ids, or ids the wrapper cannot match.
- A call while the endpoint is unreachable or the hello was refused; a fifth
  concurrent call; new calls after stdin closed.

Every refusal before a child starts says that nothing ran. Only the transport refusals
name the shell.
