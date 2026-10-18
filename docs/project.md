# rewake: the project

What rewake is, the principles it holds, and what the tool does, with a link to where
each part is specified. Read it to the end: every other document, rule and brief assumes
you know all of it, and the last section tells you which documents your work needs.
This is the project's goal: its substance changes only by the owner's decision, and its
links are kept current like any other.

## The essence

rewake lets interactive coding-agent sessions on one machine work together. A person
starts an agent through it — `rewake claude`, and next `rewake pi` — and gets the
ordinary program in their terminal; rewake registers the session under a name, and any other
session writes to it with `rewake send <name> "text"`. The recipient is told in one line
that a message is waiting, which wakes it when it is idle, and reads the text itself.

The problem it solves: several agent sessions doing one piece of work — one handing out
tasks, others doing them — have no shared channel of their own. Without one, the person
carries every message between terminals by hand, and nothing records who owes whom an
answer. rewake gives them that channel and that record, with no service to run beside
them — whichever harness each session runs on. The main session can also start its
workers itself, so the work begins without the person opening terminals.

Decided by the owner, October 18, 2026: the harnesses of 2.0 are Claude Code and next pi;
Codex does not return soon. A space is main starting its workers. Decided by the owner later
the same day: the workers are part of main's Claude Code — a panel lists them and shows the
chosen worker's screen, keys typed there reach it, and main raises them itself with the model
and effort it picks.

**Who calls it.** One person starts the sessions. The commands are called by the agents
themselves, from their own shell or through the tools their harness offers them. So the
caller of almost every command is a program reading the output, not a person reading a
screen, and that shapes the whole interface.

## The philosophy

Each principle names where it comes from.

- **The caller is an agent.** The machine-readable form matters more than the pretty
  one; a refusal names the next action to take; nothing ever waits for input, because a
  command that asks a question hangs an agent forever. ([design.md](design.md#goal),
  [design.md](design.md#failures-and-exit-codes).)
- **One command table, and everything derived from it.** The parser, the guide, every
  help page and the hint on a refusal come from one description of the commands, so none
  of them can drift from what the commands do. A table that has drifted from the behaviour
  is worse than none: it teaches a wrong call with confidence.
  ([design.md](design.md#output), [design.md](design.md#commands).)
- **No arguments prints the guide**, not an error: the first call of a session should
  teach how the tool behaves. ([design.md](design.md#output).)
- **The model of an answer is separate from its printing.** A command builds one answer
  and prints it either as lines or, under `--json`, as that same answer, so a command
  without a machine-readable form cannot exist. ([design.md](design.md#output).)
- **An unknown flag or a stray argument is always a refusal.** A silently ignored flag
  returns an unfiltered answer with success, and the caller believes it.
  ([v2/design.md](v2/design.md#what-does-not-change).)
- **A refusal teaches.** It carries the reason, the syntax, the examples, the flags and
  where the full help is. ([design.md](design.md#failures-and-exit-codes).)
- **Examples are real invocations**: they are copied verbatim, so each one is checked to
  parse. ([design.md](design.md#output).)
- **The output is the same everywhere**: no colour, no detection of a terminal.
  ([design.md](design.md#output).)
- **Exit codes are a contract**, printed in the guide: 0 done; 1 the target refused or
  could not be reached; 2 the call was wrong; 3 the message was accepted but is not
  delivered yet. ([design.md](design.md#failures-and-exit-codes),
  [v2/design.md](v2/design.md#what-does-not-change).)
- **The command line is the interface**: every mail operation is a command, and a session
  works fully through its shell and the guide. Since October 18, 2026, by the owner's
  decision, there are no mail tools beside it. (The owner's decision, recorded in
  [v2/design.md](v2/design.md#what-does-not-change).)
- **The core knows no harness.** What rewake does with messages is written once, without
  naming any harness; each harness joins through an adapter that declares what it can
  do, and a new harness is added without touching the core. (The owner's decisions,
  recorded in [v2/design-api.md](v2/design-api.md#why-capabilities) and
  [v2/design.md](v2/design.md#the-import-rule).)
- **Delivery only through what a harness offers itself.** rewake never intervenes in a
  session the person started themselves — it does not stand between them and its
  terminal — and never types into anyone's screen; it reaches a session only through the
  ways that harness provides. A worker main starts in a space runs in a pseudo-terminal
  rewake holds, and rewake shows it in main's Claude Code panel and to a person who runs
  `rewake attach`; keys reach it only from there. ([design.md](design.md#scope-of-the-first-version),
  [v2/design.md](v2/design.md#what-does-not-change).)
- **The person's configuration is never edited.** Whatever a session needs is passed as
  options of that one launch. ([v2/design.md](v2/design.md#what-does-not-change),
  [v2/design-rules.md](v2/design-rules.md#what-a-launch-adds-l1l3).)
- **Transcript contents are not read.** ([design.md](design.md#scope-of-the-first-version),
  [v2/design.md](v2/design.md#what-does-not-change).)
- **No service runs beside the sessions.** What serves a session lives exactly as long
  as that session and serves only it: there is nothing to start, stop or
  recover, and no state where the sessions exist but a service has died.
  ([design.md](design.md#processes), [v2/design.md](v2/design.md#what-does-not-change).)
- **Simple steps, retried from the disk.** A turn's end writes its reports, then clears the
  waits they answer, then records itself done, all under the sender's mailbox lock; a retry runs
  the same steps from what is on disk. A record that cannot be read fails the operation that met
  it, which is retried, and stops nothing else. Decided by the owner, October 18, 2026: the
  mechanics of unknown outcomes — `decide`, stops, journals before effects — is replaced by
  this; no reliability mechanism is added without a live failure that needs it, and a report
  lost when a process is killed between two steps is asked for again by main.
  ([delivery-turn-end.md](delivery-turn-end.md#the-end-of-a-turn).)
- **A working product over guarantees and process.** Decided by the owner, October 17, 2026:
  a step is done when it works live and breaks no existing test. A test is written where a
  breakage would not be seen live or would be seen too late, where it already happened, for a
  contract between processes, or for pure logic cheaper to test than to try by hand; a rule is
  written down where behaviour is not obvious, as a reference, without naming its tests.
  (`AGENTS.md`, "Which tests to write"; the analysis the owner accepted that day replaced the
  earlier principle "rules before code; every rule names its tests".)
- **Complete by construction, not by care — where it pays.** Where something must be whole — a
  harness fully described or absent, every command with a machine-readable form, every layer
  importing only what it may — the structure makes the gap impossible or a test makes it fail,
  rather than a reviewer having to notice. Since October 17, 2026, by the owner's decision, it
  no longer covers rules and documents. (`AGENTS.md`, "Adding a harness";
  [design.md](design.md#output).)
- **The documentation is the memory of the project, in proportion.** What is not written down
  has to be recovered from the code next time, slower and less reliably, so a change of
  behaviour an agent or the owner sees lands with its documents; the depth matches the change,
  and stale text is corrected when met. (`AGENTS.md`, "Keeping the documentation true".)
- **Provider-agnostic.** No model or provider is named in code, configuration or text
  that explains how the system works. (`AGENTS.md`, "Keeping the documentation true".)

## What the tool does

**Sessions, rooms, names and roles.** A session started through rewake is registered
under a name unique in its room; rooms keep separate groups of sessions apart, with no
address across them. Each session has a role: the main session hands out work and reads
what comes back; the others take work and answer it. See [design.md](design.md#rooms)
and [roles.md](roles.md).

**Handing out work and reading what comes back.** The main session sends tasks to the
others, follows what they still owe, and reads their reports as they arrive; it is the
only one that steers other sessions. See [flow.md](flow.md) and
[roles.md](roles.md#what-a-session-is-told).

**Different harnesses in one room, both ways.** Sessions on different harnesses work
together in one room: a main session on Claude Code hands out work to sessions on pi and
reads their reports, and the other way round, through the same mail and the same rules as
between two sessions of one harness. See
[v2/design.md](v2/design.md#the-stage-plan-from-here).

**Main starts its workers.** The main session opens a space: rewake starts a worker on the
harness main names, in a checkout of its own if asked, outside main's process tree, in a
pseudo-terminal rewake holds; the worker joins main's room as any session, and ends when
main's run ends. Main's Claude Code shows the workers in a panel of its own — each with its
model, effort, context and state, and the chosen one's screen, which takes keys — and main
picks the model and effort when it raises one. A person can also reach a worker's terminal
with `rewake attach`. (The owner's decisions, October 18, 2026.)

**Messages of several kinds.** A task asks for work and owes a report; a question is a
task whose sender waits for the answer; a note informs and owes nothing. A sender can
take back a message nobody has read yet, or replace its text. See
[delivery.md](delivery.md#kinds), [flow-endings.md](flow-endings.md) and
[delivery-sent.md](delivery-sent.md).

**One report per task.** In ordinary work every task gets exactly one report — never none,
never two; only a process killed between two steps of a turn's end can lose one, and main asks
again. A session that stops before its work is done says what it still waits for, and the
report follows when the work is done. Who owes whom a report, and who waits on whom, is
always on record. See
[delivery-owed.md](delivery-owed.md) and [turn-outcomes.md](turn-outcomes.md).

**Waking an idle session.** A recipient is told in one line, with a preview, that mail is
waiting. An idle session wakes for it; a busy one is steered where its harness allows.
Where no notice can reach a session, the message is still kept, and the session reads it
when it next looks. See [delivery.md](delivery.md#the-notice) and
[v2/design-api.md](v2/design-api.md#wake).

**Reading mail by command.** A session reads its mail with `rewake inbox`; a read counts
only once the reader has received the text. See [delivery.md](delivery.md).

**The command line contract.** One command table, a guide printed with no arguments,
help for every command, a machine-readable form of every answer, exit codes as a
contract, and never a prompt. See [design.md](design.md#interface); the tool prints its
own command list with `rewake guide`.

**Harnesses join through adapters.** Each harness is reached through an adapter that
offers what that harness can do — start a session, wake it, report its turns, report its
context and limits, compact or interrupt it —
and the core does without whatever is missing, saying so rather than pretending. What a
run actually has is checked when it starts, not assumed from what the adapter offers.
See [v2/design-api.md](v2/design-api.md#the-capabilities).

**Compaction and interrupt.** The main session can compact another session's
conversation, optionally saying what the summary should keep, and can stop the turn
another session is working on; it sees how full each session's context is before it hands
out work. On 2.0 these come through the Claude Code mod and natively on pi. See
[remote-control.md](remote-control.md).

**A checkout of its own.** A session can start in a separate git worktree, so
sessions working on one repository do not step on each other. See
[worktree.md](worktree.md).

**One build per room.** Every session of a room, and every rewake process acting for
them, is one build of rewake; another build is refused, not mixed in. See [v2/design-state.md](v2/design-state.md#one-build-per-room).

**What it never does.** It never intervenes in a session the person started themselves
— it does not stand between them and its terminal — and never types into anyone's
screen; a worker main starts runs in a terminal rewake holds and shows only in main's panel
and through `rewake attach`. It does not edit the person's harness configuration; it
does not read transcripts; it runs no service of its own beside the sessions.

**How the project is checked.** A change lands only when the project's checks pass —
formatting, static analysis and the tests. An end-to-end suite runs rewake against a
stand-in harness, and live checks run against the real harnesses. See [testing.md](testing.md) and
`AGENTS.md`, "Checks".

## What to read next

By the kind of work. Every document is on [the map](README.md); the current plan is
[v2/README.md](v2/README.md).

- **Picking up the current work**: [v2/README.md](v2/README.md), then
  [roadmap/README.md](roadmap/README.md) — what is done, what comes next, the open
  reviews.
- **Changing the core** — mail, reports, turn ends: [flow.md](flow.md), then
  [v2/design-rules.md](v2/design-rules.md), [delivery.md](delivery.md) and the
  documents it links to, [mailbox-records.md](mailbox-records.md).
- **Changing an adapter or adding a harness**: [v2/design-api.md](v2/design-api.md),
  the harness's own design ([v2/design-claude.md](v2/design-claude.md),
  [v2/stage4-reading-pi.md](v2/stage4-reading-pi.md)), [launch.md](launch.md), and its
  facts in research below.
- **Changing the command line**: [design.md](design.md#interface),
  [roles.md](roles.md), and `AGENTS.md`, "How the CLI is organised".
- **Changing the layout or the state directory**: [v2/design.md](v2/design.md),
  [v2/design-state.md](v2/design-state.md), [code.md](code.md).
- **Changing the tests**: [testing.md](testing.md), `AGENTS.md`, "Checks".
- **Changing the documents**: [README.md](README.md) and `AGENTS.md`, "Keeping the
  documentation true".
- **Looking up a fact about a harness**: [research.md](research.md) and its companions
  [research-launch.md](research-launch.md) and
  [research-claude-mod.md](research-claude-mod.md); each fact says where and on which
  version it was verified.
- **Something behaves unexpectedly**: [traps.md](traps.md).
