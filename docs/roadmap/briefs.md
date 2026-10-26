---
description: The design of briefs, not built yet — a markdown brief per task that main writes, bound to workers by send --brief, ticked and commented by them, read in the shell; the records, the commands, the tools and the shell's view.
---

# Briefs: the design

Decided by the owner, October 26, 2026: a **brief** is a markdown document for one task — a
title, a description, checkbox items, anything markdown holds — kept in the room, many per room.
Main alone writes and edits its text; a worker it was given to ticks items and writes comments of
at most 1000 characters; the person reads briefs in the shell and edits nothing there. A brief
outlives compactions of main and workers; a closed one goes to an archive. Main gives one with
`rewake send <worker> "..." --brief <id>`, and the worker gets its text with that task, and only
then. The finish report stays mail and is not copied into the brief. This page is the design for
the reviewer's acceptance; once built, what it settles moves into the living documents (below) and
the page leaves. The word is not the briefing, the launch text of `internal/core/brief`; the code
lives in `internal/core/taskbrief`, so neither is renamed.

## The records

Under the room, beside `inbox/` and `spaces/`:

```
briefs/.lock                 one flock for every write under briefs/
briefs/<id>/brief.json       {id, created, bound: [{name, epoch, since}]}, closed once closed
briefs/<id>/log/<n>.json     {n, at, from, epoch, kind: edit|tick|untick|comment, edition, item, itemText | text}
briefs/<id>/text/<e>.md      the text of edition e, as main wrote it
briefs/closed/<id>/          a closed brief, moved whole
```

- **A brief's id** is a positive integer, the highest in `briefs/` and `briefs/closed/` plus one,
  taken under the lock: short to type in `--brief 3`, and nothing for main to invent. A new brief —
  `brief.json`, its first text and the edit of edition 1 — is written into `briefs/.tmp-<random>/`
  and renamed to `briefs/<id>`, so a brief is whole or absent; a crash leaves a `.tmp-*` directory,
  which is never a record.
- **The log** numbers every event of the brief from one counter: entry `n` is the highest plus one,
  published with `state.PublishExclusive` under the lock, never edited or removed. Two numbers
  stay apart. The **edition** counts main's edits, 1 for a new brief, and only an edit raises it:
  the edit writes `text/<e>.md` first and its log entry, naming the edition, last, so the entry is
  the commit point — a crash between leaves a text no entry names, and nothing changed. The
  **current text** is the latest edition's. The **revision** is the log number of the latest
  edit, tick or untick — comments leave it —, what main's edit is checked against.
- **The title** is the current text's first `# ` heading, else its first line, cut to 60
  characters.
- **An item** is a task-list line — `- [ ]`, `* [x]`, `1. [ ]`, nested or not, outside a fenced
  block — numbered from 1 in the order of the current text; the scan is a line scanner of rewake's
  own, no markdown dependency in Go. A worker names an item by that number, and the answer echoes
  its text. A tick or untick names the edition it was read in, and is refused, exit 1, when main
  has edited since: the refusal gives the current edition and says to read the brief again, so a
  number read in an older text never ticks another item. A worker's own ticks leave the edition,
  so it ticks one item after another with no re-read. A tick is recorded with its edition and the
  item's number, and applies only within that edition: an item is done when the latest tick or
  untick of the current edition says so, else as main's own `[x]` in the text says. Nothing is
  matched by text.
- **An edit keeps the ticks by folding them into main's text.** Main edits the current text with
  the ticks applied as `[x]` (`brief show <id> --markdown`), and `brief edit` names the revision it
  started from: refused, exit 1, unless that is still the brief's revision, so neither a tick nor
  an edit made since is written over; the refusal names the revision now and says to read it again.
  After the edit the ticks live in main's text, where an item deleted, moved or reworded takes its
  own mark with it. Rejected: ticks keyed by an item's text and its occurrence — two items "Run
  tests", the first ticked and then deleted by main, and the second read done.
- **Who writes what.** Every write is a change form holding the room's build lease, then
  `briefs/.lock` (`state.WithBriefLock`, a new lock primitive beside `WithRoomLock`, in the seam's
  list of `rules/state.md`). Main writes `brief.json` (`WriteAtomic`), the texts and their edits,
  and moves a brief to `closed/` (`Rename`); a bound worker only adds ticks and comments to `log/`,
  after checking under the same lock that the brief is open, its run is bound and, for a tick, its
  edition is current. A view takes no lock: each file is whole by the atomic write, the set of them
  is no snapshot. The lock is taken after the lease and with no other lock of the room held, and
  nothing is taken inside it.
- **The letter.** A task or question sent with `--brief` carries `brief: "<id>"`, an additive
  field of the message record (`mailbox-records.md`); the brief's text is not copied into it.

## The commands

The `brief` group of the command table, with `--json` on each, examples parsed as every other:

| Form | Who | Class | What |
|---|---|---|---|
| `brief new <text>` or `--file <path>` | main | change | a new brief; answers its id and title. Text at most 64 KiB, not empty |
| `brief edit <id> --rev <n> <text>` or `--file <path>` | main | change | replaces the text whole, refused unless `n` is still the brief's revision; the ticks since the last edit end there |
| `brief close <id>` | main | change | moves it to `closed/`; every binding ends |
| `brief list [--closed]` | main | view | open briefs, or closed ones too: id, title, done of items, comments, who is bound and whether that run lives |
| `brief show <id> [--markdown]` | main, a bound run | view | a header with the edition and the revision, the text with ticks applied and each item's number, the items, the comments, the bound runs; `--markdown` prints only the text with ticks applied, no numbers, for main to edit |
| `brief tick <id> <n> --edition <e> [--undo]` | a bound run | change | ticks or unticks item `n` of edition `e`, refused once main edited since; answers the item's text |
| `brief comment <id> -- <text>` | a bound run | change | at most 1000 characters (code points), not empty |
| `send <worker> <text> --brief <id>` | main | change | a task or `--question` that binds the brief to the recipient's run and carries the brief; refused beside `--notify` |

- **Who.** "main" is the verified run of the room's main (`ownRunIn`, its record's role `main`, as
  `compact` checks); "a bound run" is a run whose name and epoch `brief.json` lists. Anyone else is
  refused, exit 1, naming who may. A shell outside a session reads no brief: the person reads them
  in the shell, which asks as main's run.
- **Binding.** `send --brief` runs every check of `send` first, then binds under the brief lock —
  adding the recipient's run, or keeping it — then writes the letter. Binding is by run, so a later
  worker of the same name gets no right to an earlier brief. It ends when the brief closes or the
  run ends; an ended run stays listed, marked ended. There is no bind or unbind of its own: a
  binding without a task would give a worker a brief it was never told of.
- **What the worker gets.** `rewake inbox` prints a letter that carries a brief and then, read at
  that moment, the brief: a header `brief 3 · <title> · edition 2 · 2 of 5 done · 4 comments`, a
  line naming `rewake brief tick`, `comment` and `show` and their tools, and the text with each
  item's number (`- [ ] #3 run the tests`); under `--json` the message gains a `brief` object.
  `inbox --owed` prints it again with the task, the current text, so a compacted worker re-reads
  it. A brief closed meanwhile prints as closed; one that cannot be read prints the letter whole
  and says the brief could not be read and how to read it again (E5, E6). A notice never carries
  it.
- **An edit tells nobody.** "Only then" is the owner's: main that wants a worker to see a new
  text sends a letter, with `--brief` again.
- `rewake edit` of a task with a brief keeps the brief in its replacement; `withdraw` leaves the
  binding.

## The tools

The same three in Claude Code's mod and rewake's pi extension, offered to every run whose
`REWAKE_ROLE` is not `main` — a choice of what is shown, as for the worktree tools; main uses the
commands. They are thin front ends of the CLI, as `worktree_status` is
([worktree-tools.md](../worktree-tools.md)): the command by argv through `REWAKE_EXECUTABLE`, no
shell, the run's `REWAKE_DIR`, `REWAKE_ROOM`, `REWAKE_SESSION`, `REWAKE_EPOCH` set over the
harness's environment, bound 60 seconds; a nonzero exit, a cut answer or a command that did not run
is an error result.

| Tool (Claude Code / pi) | Input | Runs | Answers |
|---|---|---|---|
| `mcp__rewake__brief_read` / `rewake_brief_read` | `id` | `brief show <id>` | the line form, its header naming the edition a tick takes: markdown reads as written, not escaped in JSON |
| `mcp__rewake__brief_tick` / `rewake_brief_tick` | `id`, `item`, `edition`, `undo?` | `brief tick <id> <item> --edition=<edition> [--undo]` | the item's number, text and state |
| `mcp__rewake__brief_comment` / `rewake_brief_comment` | `id`, `text` | `brief comment <id> -- <text>` | the comment's number |

The tool refuses before any process starts an `id`, `item` or `edition` that is not a positive
integer and an empty `text`; the `--` keeps a comment that begins with a dash from being read as a
flag. The right is the CLI's check, so the tool and a command the model types from its shell answer
alike.

- **Claude Code**: registered at `session.start` beside the worktree tools, served from a
  `tool.call` hook with `$.process.run` and a `.catch` that turns a failed hook into an error
  result; a new module `hooks/briefs.ts`, named in `mod.go`'s `embed` line. Mod API of 2.1.310,
  from the `mods` skill; the installed `claude --version` is 2.1.310.
- **pi**: `pi.registerTool` and `pi.exec`, `killed` read first, as in `worktree.ts`.
- **No protocol change**: the mod↔host schema and pi's wire stay as they are. Rejected: `brief/*`
  endpoints on the mod socket with the peer check — a second path beside the CLI, and pi would need
  wire operations of its own for the same thing.

## The shell

- **The button.** While a worker is up the bar is drawn, and it ends with a `briefs` button after
  the workers', two columns apart, in the unfocused grey with the terminal's foreground — it names
  no program, so no program's colour. A click or the prefix's `b` opens the modal; `b` again or Esc
  closes it. Main has no button still; the hints gain the `b` row while a worker exists.
- **The modal** is the layer `shell.md` leaves room for: a `<box position="absolute">` over the row
  of panes, the whole width on a narrow screen; the panes keep drawing under it, and the focus comes
  back to the pane after it closes. The keys go to the modal while it is open.
- **The list**: one row per brief, newest first, open ones then closed ones in the faint colour —
  `3 · Auth refactor · 2/5 · write-claude, review-claude`, an ended run faint. ↑ ↓ and Enter, or a
  click, opens one; an empty list says main makes briefs with `rewake brief new`.
- **One brief**: a `<scrollbox>` holding the text, rendered by OpenTUI's `<markdown>` (0.6.6, on
  `marked`, already in the shell's tree), then a rule and the comments, each
  `<name> · <time>` over its text. The wheel, ↑ ↓, PgUp PgDn, Home End scroll; Esc or Backspace goes
  back to the list. Given no `treeSitterClient`, `<markdown>` starts OpenTUI's default one; a
  probe of a compiled shell (review, October 26, 2026) drew headings, a checkbox and fenced code.
  Rejected: plain text — a table or nested list main writes would read badly, and rendering adds
  no dependency.
- **How it reads and learns of a change**: by rewake's commands, as main's run, like the strip —
  `rewake brief list --closed --json` once a second while the list shows,
  `rewake brief show <id> --json` once a second while a brief shows, nothing while the modal is closed. A new answer that
  differs from the last is drawn, the scroll position kept. Rejected: watching `briefs/` from the
  shell — it would build state paths itself and read records past the CLI's checks.

## Built with it

The main role's playbook names `rewake brief` and `send --brief` (the golden copies in
`internal/core/brief/testdata/`); `GLOSSARY.md` gains the article Brief, apart from the briefing;
`flow.md` notes the brief in Act 2 and Act 4; `design.md`'s tree and `mailbox-records.md` take the
records; `rules/state.md` the lock and the forms; `shell.md` the button and the modal; and this
page becomes `docs/briefs.md`, the subject's living document, in the map. Tests, by
`AGENTS.md`: the item scan and tick resolution (pure logic), the records' format and the rights
(state and a contract), the `--json` the shell reads, the tools' argv and refused input in the
mod's and pi's tests, and one workflow scenario — main makes a brief, sends it, the fixture worker
reads it with its task, ticks and comments, main's `show` sees both.

## Known limits

- The line scanner and `marked` may disagree on an odd checkbox (inside HTML, a lazy continuation):
  one may not be tickable, or drawn as text.
- The right to a brief is the run's identity from its environment, as a letter's sender is: a guard
  against mistakes, not against a session forging another's variables.
- A send that fails after binding leaves the worker bound to a brief it never got; it may read,
  tick and comment there, as any bound run.
- Briefs live in the state directory: a reboot that clears the temporary directory takes them, as
  it takes mail. Closed ones are not swept.
- The shell shows a change within a second and the command's time.
