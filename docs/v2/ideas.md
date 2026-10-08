# Ideas recorded, not scheduled

Ideas the owner recorded on October 8, 2026, during stage 3, and the recon of Claude Code
mods that bears on them. **None of this is scheduled work.** An idea becomes work only
when a stage takes it up, through the chain of `AGENTS.md`: rules, a review, then code.
Each idea says what it is, what the recon found, what blocks it, the stage it belongs
to, and the probe that would settle it.

**How the facts were obtained.** Read, not verified live, on October 8, 2026:

- the mod API types of Claude Code 2.1.289;
- the documentation at code.claude.com;
- the source of published mods.

The local Claude Code is 2.1.294 since that day, and the mod API is early access and
changes between releases. Re-read every fact here against the installed version before a
stage relies on it, and refresh the types before the stage 4 probe
([design-claude.md](design-claude.md#the-stage-4-probe)). Each fact is marked:

- *(types)* — the 2.1.289 types;
- *(docs)* — the documentation;
- *(source)* — a mod's code.

## What the recon found

**No published analogue of rewake.** The closest mods:

- **claude-message-mod and claude-cockpit-mod**
  ([devohmycode/claude-mods](https://github.com/devohmycode/claude-mods)). A tabbed pane
  beside the transcript. Letters are caught by `session.receive`, which returns
  `{consumed}`, and kept in an on-disk mailbox. Answers go out through the native
  messaging without a model turn. Claude Code only: no roles, rooms or owed reports.
  *(source)*
- **session-fleet**
  ([bennewton999/claude-code-mods](https://github.com/bennewton999/claude-code-mods)).
  A board of live sessions with status, branch and worktree, and a beacon for a session
  that needs the person. An overview, not task handout. *(source)*

**Native cross-session messaging** (Claude Code 2.1.224 and later,
[documentation](https://code.claude.com/docs/en/cross-session-messaging)):

- the tools `ListAgents` and `SendMessage`, addressing a session by name;
- one Unix socket per session, which a script or hook may write to;
- a message lands between tool calls and wakes an idle session;
- `notify_when_idle` gives one notice when another session idles or exits;
- incoming messages can be accepted, held or refused.

It covers delivery and wake between Claude Code sessions only. There are no rooms, roles,
owed reports, pending, questions, remote compact or interrupt, and no other harness.
*(docs)*

**Native teammates** ([agent teams](https://code.claude.com/docs/en/agent-teams)):

- named agents of a session's team, idle between turns, woken by a message, addressed
  `<name>@<team>`;
- in-process by default, or each a separate full `claude` process in a tmux or iTerm2
  split (`teammateMode`), coordinated through files under `~/.claude/teams/`;
- a split-pane teammate runs no loop in the lead, and its status is the last word it
  wrote to its team's roster *(types)*;
- a mod cannot create one through `agent.spawn`: the hook only chooses the model or
  denies *(types)*.

Split panes depend on the terminal multiplexer and do not draw inside the lead's screen.
*(docs)*

**What a mod can do that bears on rewake:**

- `session.receive` and `session.send`;
- `$.session.compact` and `$.turn.abort`, on the mod's own session only;
- `$.store`, a key-value store shared by every session of the machine;
- `$.prompt.submit`, which wakes the model from a background job;
- `$.process.spawn` and `$.http.fetch`, including to a Unix socket path.

*(types, docs)*

## 1. Sessions as tabs inside Claude Code

**What.** A band in the main's Claude Code, drawn by rewake's mod, with a "+" button.
Pressing it opens a tab titled, say, "claude agent". Inside it runs a full interactive
Claude Code session, started through rewake (`rewake claude`) and connected to the room
like any other.

The owner rejected two alternatives:

- tabs as tmux or other multiplexer panes, because they depend on the terminal;
- `claude -p`, because it is not the wanted launch.

The direction is rewake as a bridge:

- rewake starts the full session in a pseudo-terminal it owns;
- it keeps the screen as a cell grid;
- it serves that grid and takes input over a socket;
- the mod draws the grid in the tab and forwards the person's keys.

This works in any terminal, inside a multiplexer or not. The boundary this needs was
approved on October 8, 2026 and is written in
[project.md](../project.md#the-philosophy): rewake never intervenes in a session the
person started themselves, and provides the terminal bridge only for a session the
person asks it to host in a tab.

**What was found.** The pattern exists in a published mod:
[zenbu-labs terminal-browser](https://github.com/zenbu-labs/terminal-browser) shows an
outside program's screen in a pane and forwards keys, pointer, scroll and resize.
*(source)*

- **The process.** An outside process renders. The mod starts or finds it with
  `$.process.run` and long-polls its state over local HTTP. *(source)*
- **Drawing.** Frames go to a `Raster` with `$.ui.blit`. A `Raster` is up to 512×256
  cells of 24-bit colour, and `blit` takes up to 120 frames a second, of which about 60
  are shown. *(types)*
- **Input.** A transparent `Client` over the `Raster` captures input. Keys are queued
  and posted in batches, because only one `post` per frame is delivered and a later post
  replaces an undelivered one. *(source, types)*

For rewake the same shape would be:

- rewake owns the pseudo-terminal and a VT parser;
- it serves the cell grid over a Unix socket;
- the mod fetches it with `$.http.fetch` and converts it to `Raster` cells;
- keys go back through the `Client`'s queue.

**What blocks it.**

- **Escape never reaches a `Client`**: it returns the focus to the prompt *(types)*.
  Claude Code needs Escape to interrupt and to close dialogs. A remap or a button would
  be needed; an interrupt can go through `rewake interrupt`.
- **Other keys are uncertain.**
  - Ctrl+X is the engine's chord prefix *(docs)*.
  - What happens to Ctrl+C, Ctrl+D, other Ctrl combinations and paste while a `Client`
    has focus is not described.
  - Key events carry no Alt.
- **Focus.** Focus comes only from a click or `open({focus})` *(types, docs)*.
- **Rendering fidelity.** `Raster` cells have no bold, italic, underline or dim, and no
  wide or non-BMP glyphs *(types)*. Claude Code's own screen degrades.
- **No pseudo-terminal from the mod.** `$.process.spawn` has none, and its standard input
  is one-shot *(types)*. The pseudo-terminal and the VT parsing must live in rewake's
  process. A mod cannot import libraries.
- **Pane size.** A pane is a side region or a region above the prompt, not the full
  screen *(docs)*. The hosted session sees a small window and is resized to it.
- **A launch inside a session.** rewake refuses to start a harness inside a session
  today (`AGENTS.md`, "Checks"). A hosted launch needs its own path.

Realistic today: a view with limited input. A faithful full interactive tab is weak on
Escape, Ctrl combinations and text attributes.

**Stage.** PersonUI and Launch in stage 6 ([design-api.md](design-api.md#personui)),
after the mod adapter of stage 4. A live feasibility probe can come earlier.

**The probe that would settle it.** A `Client` that logs every key event it receives:

- Escape through a remap;
- Ctrl+C and Ctrl+X;
- Alt;
- paste;
- function keys.

Alongside it:

- the frame rate `blit` reaches on a 200×50 screen;
- `$.http.fetch` with a socket path to a Unix socket rewake serves.

### Later: workers launched only from a main's tabs

A later direction from the owner, not scheduled: once tabs exist, Claude Code workers are
launched only from inside a running main, in its tabs, not standalone. Each worker is then
bound to its main from the start. Open questions for that design:

- a worker's lifetime when its main ends or restarts;
- the room as the main's set of tabs, against today's room by directory and name;
- Codex workers, which have no mod and keep launching through the command line.

## 2. A room panel in the main's screen

**What.** A panel in the main's Claude Code with:

- the room's sessions, with status and context;
- who owes whom a report;
- buttons to compact, interrupt and send.

Tabs could have digit hotkeys, as the cockpit mod's tab bar does with `Button` hotkeys
*(source)*.

**What was found.** Panes, tabs and buttons are what mods draw today *(docs, source)*.
session-fleet shows that a board of live sessions is feasible *(source)*. The room's
state comes from rewake (`rewake list --json`, `rewake inbox --owed`), not from the
harness.

**What blocks it.** Nothing found. A pane opened by a timer rather than by the person
appears only on a wide terminal *(docs)*, so the panel opens on the person's action.

**Stage.** PersonUI, stage 6. The person's own status line stays untouched
([design.md](design.md#the-stage-plan-from-here)).

**The probe.** None needed before stage 6 beyond the stage 4 mod.

## 3. Checks for the stage 4 probe

Three mechanisms the stage 4 probe could look at beside its own points:

- **`session.receive` returning `{consumed}`.** It keeps a message from the model *(docs)*.
  rewake could use it to mark or account for a letter without a model turn.
- **`$.store`.** A key-value store shared by every session of the machine *(docs)*; a
  candidate for shared room state.
  - It would not replace the state directory, which holds evidence across runs.
  - It is per machine, not per room or build.
- **Native `notify_when_idle`.** One notice when another session idles or exits *(docs)*,
  beside rewake's own notices.

**Stage.** 4, as checks added to the probe
([design-claude.md](design-claude.md#the-stage-4-probe)). Not commitments.

## 4. Headless workers

**What.** A worker run as `claude -p` with stream-json, rewake keeping its standard
input open.

**Status.** The owner prefers full sessions. This stays only as a fallback, should full
sessions prove unworkable for some use. No stage holds it.
