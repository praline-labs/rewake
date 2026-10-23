# A worker's permission requests and questions answered by main

A worker that asks for a permission, or whose model asks a question, at a screen nobody watches
stops its task, and main is not told. Since October 18, 2026 such a request of a Claude Code
worker goes to main as an ordinary notify, main answers with `rewake answer`, and the worker goes
on; a person at the worker's screen — its pane in main's shell — may answer first.
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

Main's decision of October 23, 2026, after a worker stood on a dialog in its pane with main told
nothing: Claude Code 2.1.302 runs a launch with no mode in auto mode, its default, so workers run in
it, and there the check carries nothing; any permission dialog the engine draws itself is carried to
main as well (step 2 below), and main is told of any other dialog a worker waits at
([below](#a-dialog-nobody-is-asked-about)).

## The path

On Claude Code 2.1.302, through the mod (`internal/adapter/claude/mod/hooks/asks.ts`, its hooks in `dialogs.ts`):

1. **A permission.** `tool.check` awaits the engine's verdict. An `ask` of a real call is
   carried when the run is a worker, the mode the harness last reported in its classic events
   is one where the person would be asked (`default`, `manual`, `acceptEdits`), no
   administrator's ceiling is set, and the tool is not `AskUserQuestion` or `ExitPlanMode`.
   An `ask` in `auto` precedes the classifier, and one in `plan`, `dontAsk` or
   `bypassPermissions` is not the person's: those stay with the engine, and a dialog it then
   draws is carried by step 2. A call allowed for the
   session passes at once; any other becomes `$.ui.ask` of the engine's own dialog with the three
   choices, and its answer becomes the check's verdict. A failure of the hook refuses the call:
   nothing has run.
2. **A permission dialog the engine draws.** Where the check carried nothing — auto mode, or a
   mode it does not carry — an ask of a worker's call is recorded at the check with the call's
   id, and the engine draws its own dialog, firing `classic.PermissionRequest`, which carries no
   id; the call is found by its tool and input. Unless an administrator's ceiling made the ask, the
   mod opens a permission request on its wrapper with the same three choices and holds the event
   by the same polls, the dialog staying on the screen. Main's answer returns as the event's
   decision, which closes the dialog; a call allowed for the session passes at once. The person
   answering first ends the call — `classic.PostToolUse`, `PostToolUseFailure` or
   `PermissionDenied` with its id — or, refused, the next step of the main loop or the turn's end
   comes; either settles the request as the person's, and the event is left undecided
   ([the probe](research-claude-mod.md#a-workers-dialogs-in-auto-mode)).
3. **Every question dialog of a worker** — that one and the model's `AskUserQuestion` — reaches
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
4. **The deadline** is 120 seconds from the opening. Past it a permission is refused, and the
   model's question refused, with the text the model reads: no answer came; do not retry
   blindly; end the turn with `rewake pending "needs <what>"` if the work needs it. An interrupt
   cancels the dialog and the request.
5. **Main's answer.** `rewake answer` goes through the worker's control directory as `compact`
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
Choices: once allows this call; session allows this exact call until the worker ends or its mod reloads; refuse [reason] refuses it, and the reason goes to its model.
Answer: rewake answer api 3f9a once
A person at the worker's screen may answer first; the first answer counts, and with none by the deadline the worker is refused.
```

A question lists its options numbered, says when several may be chosen, and names the command
with one answer per question. The input shown is cut at 2,000 characters, the mod's copy at
16,000. No report is owed for the letter, and none comes for the answer.

## A dialog nobody is asked about

Main's decision of October 23, 2026: a worker waiting at a dialog rewake does not carry — one main
cannot answer through the mod, or one whose request could not be opened — must not look like work.
Claude Code sends `classic.Notification` while a session waits at a dialog (2.1.302: a permission
dialog's `permission_prompt`, about six seconds after it is drawn,
[the probe](research-claude-mod.md#a-workers-dialogs-in-auto-mode)); the engine draws trust,
onboarding and login before the mod, and they have no such event.

- **The mod** (`dialogs.ts`) of a worker's admitted load takes any notification but
  `idle_prompt` and `auth_success` while no request of steps 2 and 3 is held — the notification of
  a carried dialog is not another dialog — and puts its message, cut at 300 characters, in the
  state it samples (`StateRequest.dialog`, optional, the protocol's one addition). The next tool
  result (`PostToolUse`, `PostToolUseFailure`, `PermissionDenied`), main-loop step or turn end
  clears it.
- **The wrapper** keeps it in the snapshot (`session.Snapshot.dialog`, optional) and, the first
  time a sample names a dialog, leaves main one notify through `Config.Dialog`
  (`ask.TellMainDialog`): `Rewake: api waits at a dialog in its pane: <message>`, then that rewake
  cannot answer it, so main answers it in the pane or asks the person. A letter that could not be
  left is tried with the next sample, which the heartbeat brings; a dialog cleared and another
  drawn is told again.
- **`rewake list` and the shell's strip** read `waiting at a dialog` for it, in place of `idle`,
  `working` or `waiting for`.

Not covered: a dialog of the engine that sends no notification, and one drawn while a carried
request is held, which by the screen's order is the carried one.

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
- Session allowances live in the mod's memory and go with a reload of the mod. The request bridge is Claude Code's; pi has none built in (October 19, 2026).
- The mode comes from the harness's classic events; a mode never reported carries nothing at
  the check, and the engine's own dialog is then carried as in step 2.
- A person who refuses an engine-drawn dialog is seen only at the next step or the turn's end;
  main's answer before that is reported as taken, and changes nothing.
- A request settled while main's answer is on its way is reported to main as answered first or
  expired; one whose settling never reaches the wrapper ends at its deadline there.
