# A worker's permission requests and questions answered by main

A worker that asks for a permission, or whose model asks a question, at a screen nobody watches
stops its task, and main is not told. Since October 18, 2026 such a request of a Claude Code
worker goes to main as an ordinary notify, main answers with `rewake answer`, and the worker goes
on; a person at the worker's screen — a tab of main's host mode — may answer first.
The facts the build rests on, and where they were seen, are in
[permission-requests-sources.md](permission-requests-sources.md).

## What was decided

The owner, September 28, 2026, and still standing: only a worker's requests go to main — a
session of the roles `general` and `write`, never main's own; what the session's rules and mode
already allow or refuse passes as it would; a request waits within rewake's own limit, and past
it the worker is refused with the next step, `rewake pending "needs <what>"`; a person at the
worker's screen may answer first. Main allows only what it may do itself, and a call it would
not run itself — deleting, a force push, another tool's settings — it refuses and takes to the
owner; rewake marks nothing as the owner's in code.

Main's decisions of October 18, 2026, which replaced the September design's `permission` kind,
grants and journal: the choices are **Allow once**, **Allow for this session** — this exact call,
the tool and its whole input in the same directory, kept in the mod's memory for its load — and
**Refuse** with an optional reason the model reads; the model's own `AskUserQuestion` keeps its
real choices. Main hears of a request through an ordinary `notify` letter, answers with
`rewake answer <worker> <request> <choice…>`, and only a session of role main in the room
answers, checked as `rewake compact` checks it: a process that takes on main's name and run in
its environment could answer too, a limit accepted. No authority server, no journal, nothing on
disk: a request lives in the worker's wrapper's memory, and one lost to a restart is asked
again. Main's own permission dialogs are not caught. pi has no permission gate or question tool
of its own (`question.ts` and `permission-gate.ts` are examples, not defaults), so its adapter
carries nothing.

## The path

On Claude Code 2.1.302, through the mod (`internal/adapter/claude/mod/hooks/asks.ts`):

1. **A permission.** `tool.check` awaits the engine's verdict. An `ask` of a real call is
   carried when the run is a worker, the mode the harness last reported in its classic events
   is one where the person would be asked (`default`, `manual`, `acceptEdits`), no
   administrator's ceiling is set, and the tool is not `AskUserQuestion` or `ExitPlanMode`.
   An `ask` in `auto` precedes the classifier, and one in `plan`, `dontAsk` or
   `bypassPermissions` is not the person's: those stay with the engine. A call allowed for the
   session passes at once; any other becomes `$.ui.ask` of the engine's own dialog with the three
   choices, and its answer becomes the check's verdict. A failure of the hook refuses the call:
   nothing has run.
2. **Every question dialog of a worker** — that one and the model's `AskUserQuestion` — reaches
   `tool.call` of `AskUserQuestion`. The mod opens a request on its wrapper (`ask/open`); the
   wrapper keeps it in the run's broker (`internal/core/ask`), leaves main the letter, and
   answers its id. The mod starts the dialog (`next`), which the person may answer, and polls the
   wrapper (`ask/poll`), each poll held up to 15 seconds, so the hook spends none of its own
   budget and no fetch is cut. The first answer wins at one latch in the mod: the person's
   through the dialog, main's through a poll — returned as the tool's result, keyed by the
   question, which closes the dialog — or the deadline's refusal. The mod settles the request
   with its winner (`ask/settle`), which wakes a held poll and tells main's waiting answer. A
   wrapper that will not open a request — no main in the room, a run main does not answer for —
   leaves the dialog the person's alone, and so does a wrapper that stops answering polls.
3. **The deadline** is 120 seconds from the opening. Past it a permission is refused, and the
   model's question refused, with the text the model reads: no answer came; do not retry
   blindly; end the turn with `rewake pending "needs <what>"` if the work needs it. An interrupt
   cancels the dialog and the request.
4. **Main's answer.** `rewake answer` goes through the worker's control directory as `compact`
   does (`internal/core/control`, action `answer`); the wrapper reads the words against the
   request's offer, gives the open request main's answer, and waits up to 15 seconds for the mod
   to settle it. The command's success is the mod's acceptance, not its queueing; an answer the
   mod did not confirm is reported as not confirmed, never repeated by itself.

## What main sees

A notify from the worker, which the ordinary notice announces and `rewake inbox` reads:

```
Rewake: api asks permission for Bash (request 3f9a); answer by 14:05:12.
Input: {"command":"make deploy"}
In: /home/me/code/app
Choices: once allows this call; session allows this exact call for the rest of the worker's run; refuse [reason] refuses it, and the reason goes to its model.
Answer: rewake answer api 3f9a once
A person at the worker's screen may answer first; the first answer counts, and with none by the deadline the worker is refused.
```

A question lists its options numbered, says when several may be chosen, and names the command
with one answer per question. The input shown is cut at 2,000 characters, the mod's copy at
16,000. No report is owed for the letter, and none comes for the answer.

## The command

`rewake answer <worker> <request> <choice…>`, main only:

- a permission takes `once`, `session`, or `refuse` with the rest of the words as the reason;
- a question takes the numbers of its options, several as `1,3` where it allows several, or any
  other text, which is the answer typed under Other; with several questions, one quoted word each.

Exit 0 when the worker took the answer; 1 when it was refused — no such request (settled,
expired or never opened), not a choice, answered first at the screen, past its deadline, not
answering, another request in flight — or not confirmed; 2 for a wrong call. Its help is the
command table's.

## Known limits

- A request lives in the wrapper's memory: a wrapper or mod restart loses it, and the dialog
  stays the person's at the screen; the model asks again on its next call.
- Session allowances live in the mod's memory and go with a reload of the mod.
- The mode comes from the harness's classic events; a mode never reported carries nothing to
  main, and the dialog stays the person's.
- A request settled while main's answer is on its way is reported to main as answered first or
  expired; one whose settling never reaches the wrapper ends at its deadline there.
