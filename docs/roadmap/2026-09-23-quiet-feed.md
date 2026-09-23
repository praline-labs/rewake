# A quieter feed — September 23, 2026

The owner asked, on September 23, 2026, for as little debug as possible in what rewake
puts in front of an agent and a person. The machine form, `--json`, stays as it was:
scripts depend on it, so no field was added, removed or reworded, including the
`detail` of a pending question. Two stored texts did change: the compaction note and
the undelivered note are written with their new wording, so `rewake inbox --json`
returns the new values in `text` for messages written from now on; messages already
stored keep theirs. The rule for the text form, now in the guide and in
[design.md](../design.md#output): a line rewake says itself starts with `Rewake:` and
states the outcome, except the lines the guide names — a peer's headers, main's state
line and the availability and departure notices; how it works lives in `--help` and
the guide. A refusal keeps its
full form — reason, syntax, examples, full help — and only its first line may be
shorter.

**Kept by owner decision, the same day.** Two lines the first inventory proposed to
cut stay exactly as they were, and the decisions are recorded where they apply, in
[session-state.md](../session-state.md): the telemetry header above every message main
reads, so main always knows each session's state; and the full identity block of an
availability notice, so the reader never has to ask for it again. The departure notice
has the same block and keeps it for the same reason.

## The inventory

Every text line of the ordinary flow, with what became of it. Unchanged lines are
listed too, so the next pass starts from the whole set.

| where | old | new | why |
| --- | --- | --- | --- |
| `harness.Notice` | `Rewake: <session> <kind>, N new message(s)` and `  ↳ <preview>`; `Rewake: N new messages` for a group | unchanged | already short, and the preview is the author's own line |
| Codex `mailboxNotice` | the notice above, with a colored mark, plus member ids | unchanged | the ids are what the briefing tells Codex to read by |
| `inboxLines` | `No new messages.` | `Rewake: no new messages.` | rewake speaking |
| `inboxLines` | `from <session> · <kind> · <time>` then the text | unchanged | the peer's header, not rewake's own line |
| `stateLine` | `<session>: <activity> \| context … \| compactions …` above each message main reads | unchanged | owner decision |
| `ThreadChangedWarning` | `the reader's thread changed after delivery; the report may not answer it, resend the message` | `Rewake: the reader's thread changed after delivery; this may not answer it, resend the message` | rewake speaking; the action stays |
| `writeInboxPeek` | `N unread messages (overview; none marked read):` | `Rewake: 1 unread message:` / `Rewake: N unread messages:` | what a peek does is in `--help`; the count agrees in number |
| `writeInboxPeek` | `<id> · <from> · <kind> · <date time> · <preview>` | unchanged | the ids are the point of a peek |
| `writeInboxPeek` | `Read one: rewake inbox --message <id>; read all: rewake inbox` | dropped | the same words are in `--help` and the TALK summary of the guide |
| `owedLines` | `N read and still owed a report — shown again, nothing marked; ending your turn reports on them:` | `Rewake: owed a report for 1 message:` / `… for N messages:` | the one line the owner named |
| `owedLines` | `from <session> · <kind> · <time> · <id>` | `from <session> · <kind> · <time>` | the header of an ordinary read; the id stays in `--json` |
| `owedLines` | `from <session> · <id> · the text is no longer kept` | `from <session> · <id> · text no longer kept` | shorter; the id is all that is left to name it |
| `owedLines` | `Nothing read is owed a report to another session. A task sent from a plain shell owes no report and is not listed, so it cannot be shown again this way.` | `Rewake: nothing owed a report.` | the plain-shell rule was already in `--help` |
| `sendLine` | `delivered to <name> via <path>[; <detail>]` | `Rewake: delivered to <name> via <path>[; <detail>]` | rewake speaking |
| `sendLine` | `pending for <name>: <detail>` | `Rewake: pending for <name>: <detail>` | rewake speaking |
| `sendLine` | `held for <name>: <detail>` | `Rewake: held for <name>: <detail>` | rewake speaking |
| `sendLine` | `failed for <name>: <detail>` | `Rewake: failed for <name>: <detail>` | rewake speaking |
| `answerQuestion` | `answer from <name>:` / `error from <name>:` / `stopped from <name>:` then the text | unchanged | the peer's header |
| `answerQuestion` | `asked <name>: no answer from <name> yet; it will arrive as a "Rewake: <name> finished" line or a grouped notice, to be read with rewake inbox` | `Rewake: no answer from <name> yet; it arrives later as its report` | how a report is announced is in the guide; `--json` keeps the old detail |
| `handlePending` | `pending: when this turn ends, <senders> will read that the work is still going: <text>` and `The report follows at the next turn end. Better still: wait inside this turn.` | `Rewake: marked pending; at this turn's end <senders> will read that the work goes on.` | the text was the caller's own; the advice is in `--help` |
| `tellUndelivered` | `Your <kind> to <name> was not delivered: <why>. The agent never saw it, it is not owed a report, and nothing sends it again; send it anew if it still matters.` then what was sent | `Rewake: your <kind> to <name> was not delivered: <why>. Send it again if it still matters.` then what was sent | the consequences are in the guide |
| `compactionMessage` | `Primary compaction completed (observed count N).` | `Rewake: context compacted (compaction N).` | shorter, and says it is rewake's |
| `availabilityMessage` | `Session available.` / `Session already available in this room.` and name, role, harness, room, cwd | unchanged | owner decision |
| `departureMessage` | `Session is no longer available in this room (<reason>).` and the same block | unchanged | the same decision, for the same reason |
| wrapper stderr at launch | `rewake: room <room>, role <role>: <reason>` and other `rewake: …` notes | unchanged | a person's terminal before the harness starts, in the usual `program: message` form, not the feed |
| refusal, `inbox` outside a session | `This shell is not part of a rewake session, so it has no inbox. Start an agent with rewake to give it one.` | `This shell is not part of a rewake session, so it has no inbox.` | the hint below the refusal already shows the calls |
| refusal, `inbox --owed` in main | `<name> owes no reports: a main session's reads record no obligation, so --owed has nothing to show. What you wait on from others comes back as reports; rewake list shows who is still working.` | `<name> owes no reports: a main session's reads record no obligation, so --owed has nothing to show.` | the rest was advice, not the reason |
| refusal, `pending` with nothing owed | `nothing is owed: no task or question read in this run waits for a report, so there is nothing to keep open; end the turn as usual.` | `nothing is owed a report, so there is nothing to keep open; end the turn as usual.` | the same reason, once |
| other refusals | their reason, syntax, examples and full help | unchanged | already one statement each |

## Tests

Every test that pinned an old line pins the new one, whole where the line is fixed:
the empty inbox, the empty and filled `--owed`, the pending question, the held and
pending send, the `pending` answer, the first lines of the `pending` and main
`--owed` refusals, the undelivered note, the thread warning, a new
test for the peek overview's two lines, and the compaction notice in the Claude Code
telemetry scenario. The workflow case `owed-reread` now checks the id in the machine
form and the new header in the text, and that the text leaves the id out.

**Open.** The briefing at launch is instructions rather than feed, and was not part of
this pass.
