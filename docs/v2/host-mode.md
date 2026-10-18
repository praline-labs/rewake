# Design: the host mode of rewake

The owner's decision of October 18, 2026 ([project.md](../project.md#what-the-tool-does), "Main
starts its workers"): `rewake claude`, started in a host mode, holds the person's terminal and
shows main's Claude Code and each worker above a bottom tab bar, with a sidebar of workers on the
right. An ordinary `rewake claude` stays transparent. The bar and the sidebar replace the pane:
the pane, `/workers`, the `workers/*` endpoints and `rewake attach` were removed outright before
step 1, so nothing trails behind (the owner's decision, October 18, 2026; the pane's design is
archived, [workers-panel.md](../archive/v2/workers-panel.md)). The techniques come from
[agent-deck-ideas](../archive/v2/agent-deck-ideas.md) §2–3. The mod side is the `mods` skill's,
taken on Claude Code 2.1.302.

## Topology

```
person's terminal ⇄ host (main's wrapper) ⇄ pty ⇄ main's harness      local Grid
                         │  ⇄ space socket "v" ⇄ space host ⇄ pty ⇄ worker   (one per space)
```

- **Main's wrapper becomes the host.** It does not start a second process. Today the wrapper
  gives the harness its own stdin and stdout (`internal/host/wrap.go`). In host mode it opens a
  pty (`terminal.OpenPlain`) and starts the harness in it instead. It feeds the harness's output
  to a local `terminal.Grid` and writes the grid's answers back to that pty. The mailbox, the
  signals and the run's end stay as they are.
- **Workers stay in their space hosts** ([space.md](../rules/space.md), SP1 unchanged). The host
  connects to each space through the **view** stream `v`, not the attach stream:
  - view takes several clients;
  - view carries the modes;
  - the space host's emulator keeps answering the worker's queries, so nothing reaches the
    person's terminal unasked.

  The host finds spaces the way `spacehost.Panel.Workers()` does, polling once a second, and
  opens a view for each.
- **One kind of tab.** Main's local grid and a worker's view both give `terminal.Screen` frames,
  so the renderer never knows which program it is drawing. The host keeps each tab whole from
  the raw frames, modes included (`terminal.Picture`). `spacehost.View` is not reused here,
  because it converts to `adapter.Screen` and drops the modes.
- **Exits.** The host ends when main's harness exits: the host restores the terminal and the run
  ends. The space hosts see the run end and end their workers (SP1), which is already how it works
  today. If the host itself is killed, its run ends with it, and the space hosts follow the run's
  identity, so the workers end whatever main does. The master side of main's pty closes and main
  gets a hangup, which a program may handle or ignore. Today a killed wrapper leaves the harness
  running on the terminal; here it is left without one. A SIGKILL also cannot restore the
  person's terminal from raw mode and the alternate screen, so `reset` repairs it. Both are known
  limits, not a reason for another supervisor. A worker that ends keeps its tab, marked as
  ended, until its space record goes.
- **Job control.** The harness runs in a session of its own on its pty, so the person's
  terminal no longer stops it or the host. A harness that stops itself is continued at once,
  since nobody else could bring it back.

## Rendering: from the emulators

The host draws a composed screen on the person's terminal from the emulators' cells. It does not
pass the programs' bytes through.

- **Layout.** The region is the person's terminal size, minus one row for the bar, minus the
  sidebar's width (about 28 columns) when the sidebar is shown. The sidebar is shown only at 120
  columns and wider, and the prefix can hide or show it.
- **The bar.** It holds `main` and each worker's name, with the current tab inverted, an ended
  tab dim, and a mark on a tab whose screen changed since it was last shown.
- **The sidebar.** It holds what `Panel.Workers()` already joins: name, harness, model, effort,
  context and state.
- **One renderer.** Each frame becomes a cell buffer drawn by `ultraviolet`'s `TerminalRenderer`,
  which diffs against what is on the terminal and degrades colour to the terminal's profile.
  `ultraviolet` is already a direct dependency (`internal/host/terminal/vt.go`), so this adds none.
- **What reaches the terminal.** Full colour (RGB, indexed, default) and all six attributes
  arrive as the emulator holds them.
- **The person's theme shows through** (the owner's wish, October 18, 2026). A program's
  indexed colour is written to the person's terminal as an index, never converted to RGB, so the
  terminal's own palette paints it: 0–15 as `ansi.BasicColor` (SGR 30–37, 90–97), 16–255 as
  `ansi.IndexedColor` (SGR 38;5;n). `ultraviolet` hands a colour to `colorprofile.Convert`,
  which (v0.4.2, read in the module source) passes a basic colour through on every profile and
  an indexed one on every profile but ANSI, where it maps it to the nearest of the 16. RGB stays
  RGB on a true-colour terminal and becomes the nearest index on a 256-colour one. A default
  colour stays the terminal's default. The bar and the sidebar use the default and palette
  colours only. A configuration section of the host for its layout and chrome colours comes
  later; nothing now. The shown program's cursor is placed at its position plus
  the region's offset, and shown or hidden as the program has it.
- **Sizes.** Every program's pty, main's and each worker's, is kept at the region's size. A tab
  switch is then only a full redraw of the other program's buffer: no program is told a new size
  and none has to repaint.
  - On a SIGWINCH the host resizes main's pty and grid, and sends an `r` frame on every view.
  - The last size from any client wins. Once the pane and attach are gone, the host is the
    only client that sizes a worker.
- **Not passed through.** Because nothing is passed through, a program's queries never reach the
  person's terminal, so the problem of an orphan reply after a switch does not exist here. Three
  outputs must still go out, and they become a new view frame `e` (an event) from the space host,
  which main's grid gives locally:
  - OSC 52 (copy). It is dropped today in `vt.go`; it will be forwarded instead.
  - The title.
  - The bell and notifications.

  OSC 8 hyperlinks are lost until the frames carry them.
- **Rejected: a scroll-region trick.** The idea is to tell the program a smaller size and place
  its raw output inside margins. It cannot give a right sidebar, because DECSLRM side margins are
  rarely supported. Programs reset margins themselves. A switch needs the program to repaint, with
  the orphan replies and the size nudge §2 had to build. The terminal's scrollback would mix
  programs. Its only gain is lower latency. A view frame comes 30 ms after a change, which is
  measured live in step 1, and a host-only view mode with a shorter delay is the remedy if it is
  felt.

## Input

- **One reader.** A single stdin reader feeds a prefix automaton over the byte stream, as in §2.
  Its state survives a split at any offset.
- **Whole keys.** An incomplete escape sequence at the end of a read (`ESC`, `ESC O`, a CSI of
  only parameter bytes) is held until it completes or 50 ms pass. So a lone Escape still goes
  out, and a key is never sent in two writes.
- **The prefix is Ctrl+\\.** No harness binds it, and it was `rewake attach`'s detach key.
  - The automaton matches the legacy byte `0x1c` and its CSI-u and modifyOtherKeys forms.
  - It never matches inside a bracketed paste.
  - A pending prefix lapses after 2 s.
- **The commands after the prefix:**
  - `n`/`p`: the next or previous tab;
  - `1`–`9`: a worker;
  - `0` or `m`: main;
  - `s`: hide or show the sidebar;
  - the prefix again: sends Ctrl+\\ to the shown program;
  - any other key: cancels, which is a choice; §2 forwarded both keys.
- **The person's terminal mirrors the shown program's input modes.** At a switch, and whenever
  the shown program changes them, the host sets on the person's terminal the program's:
  - DECCKM and DECNKM, so Right arrives as `ESC O C` where the program asked for it;
  - bracketed paste 2004;
  - focus 1004;
  - the mouse modes (9, 1000, 1002, 1003, 1006).

  Keys then pass unchanged. At the exit every mode is reset. Step 1 is built this way.
- **Mouse, from step 2.** The host keeps SGR mouse (1000/1002 with 1006) and bracketed paste on
  the terminal, and stops mirroring the mouse.
  - A click on the bar or the sidebar switches tabs.
  - In the region, mouse events go to the shown program, with the coordinates offset and in the
    form its modes ask for, but only when the program turned mouse on. Otherwise they are
    dropped.
  In step 1, while mirroring, a mouse event below the region (on the bar) is the host's and is
  never forwarded.
- **Paste.** A paste reaches the program framed by 200~/201~ only when the program enabled 2004;
  otherwise it goes bare.
- **Focus events.** These go through when the program asked for them (1004).
- **Keys.** All other input goes to the shown program unchanged.
- **The kitty keyboard protocol.** The host does not push it on the person's terminal. Keys
  arrive in the legacy form, which Claude Code handles today behind the space host's emulator.
  Translating the kitty forms is a later step, done only if a harness needs it.

## Scrollback, selection, copy

- **Scrollback.** The host runs on the alternate screen, so the person's terminal scrollback
  holds nothing of the programs. That is lost.
  - The host starts Claude Code with `CLAUDE_CODE_NO_FLICKER=1`, unless the person set it, so
    that each program scrolls its own history with the forwarded wheel.
  - A host-side history view is a later idea, not a step here.
- **Selection.** A drag in the region is the program's own selection, and its copy reaches the
  terminal clipboard through the forwarded OSC 52. Shift+drag gives the terminal's native
  selection over the visible composed screen, bar and sidebar included.
- **Default colours.** The host reads the terminal's default colours with OSC 10/11 at start and
  seeds every emulator with them (`SetDefaultForegroundColor`/`Background`), so a program that
  asks for the background sees the real one. A worker's emulator gets them in a frame on the view.

## The mode switch

- **The flag.** Host mode is a launch flag `--host`, placed before the harness word. It is
  refused for a role that does not host spaces, and refused without a terminal on stdin and
  stdout.
  - An alias turns it on through its existing `rewake = ["--host"]` field
    ([launch-defaults.md](../launch-defaults.md)), so no new alias field is needed.
  - A flag rather than a separate command: host mode is a way of running main, not another
    harness.
- **Nothing to gate.** Since the pane and its endpoints are removed rather than kept for host
  mode, the mod needs no switch: no environment variable, no hello field. Ordinary main keeps
  opening spaces as now; `Role.Spaces` and the live `SpacesCapability` are untouched. Without
  `--host`, a worker's screen is seen only from a host.

## Steps, each checked live

1. **Main and one worker, the bar and switching.**
   - Pieces: `--host`; main in the host's pty and grid; one view per space; the bar; Ctrl+\\
     n/p/digit/m.
   - Keys go to the shown program; resize and exit.
   - Live check: type into main, start a worker, switch to it, type a line, switch back.
   - Tests: the prefix automaton, the layout arithmetic, and one workflow scenario over the
     fixture.
2. **Mouse, paste and focus.** The forwarding rules above. Live check: Claude Code's fullscreen
   scroll and its click selection in a worker.
3. **The sidebar.** The width threshold, `s`, and clicks on it.
4. **Events and colours.** The `e` frame: OSC 52, title, bell, default colours; the theme's
   indices checked live. Live check: a copy from a
   worker lands in the clipboard.

The tests are the subset of §2's checklist that "Which tests to write" keeps: the automaton, a
restore on every signal, and the routing of mouse and paste by mode.
