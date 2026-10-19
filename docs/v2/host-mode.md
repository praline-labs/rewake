# Design: the host mode of rewake

The owner's decision of October 18, 2026 ([project.md](../project.md#what-the-tool-does), "Main
starts its workers"): `rewake claude`, started in a host mode, holds the person's terminal and
shows main's Claude Code and its workers above a bottom tab bar. Since the owner's decision of
October 19, 2026, main is on the left and the chosen worker's live screen on the right; the bar
chooses the worker, and the sidebar of status cards that step 3 built is gone, a misreading of
the owner's wish. An ordinary `rewake claude` stays transparent. The bar replaced the pane:
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

- **Layout: the split** (the owner's decision, October 19, 2026). Above the bar's row, from 120
  columns on and once a worker exists, main takes the left part and the chosen worker the right,
  a dim divider column between; the worker's part is half of the rest, the narrower one when the
  width is odd, and a `[host]` configuration of the proportion comes later. Below 120 columns,
  with no worker, or zoomed, there is no split: the focused program takes the whole width, and
  the bar switches the window between main and the workers.
- **Focus.** One program takes the keys: main or the chosen worker. Choosing a worker on the bar
  focuses it; a click in a part, or Ctrl+\\ ← and →, moves the focus. A program that asked for
  focus reports (1004) hears that it lost or gained the focus. The keyboard protocols, the
  modes, the title and the cursor on the person's terminal are the focused program's.
- **Zoom.** Ctrl+\\ z gives the focused program the whole width and back; nothing else changes.
- **The bar.** It holds `main` and each worker's name, numbered from 0: the focused tab inverted,
  the other shown one underlined, an ended tab dim, a `…` on a worker in a turn (from the poll of
  the spaces once a second), and a `*` on a tab whose screen changed since it was last shown. A
  click on a tab chooses it. The hint at its right end names the arrows and `z` from 120 columns.
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
  colour stays the terminal's default. The bar and the divider use the default and palette
  colours only. A configuration section of the host for its layout and chrome colours comes
  later; nothing now. The focused program's cursor is placed at its position plus
  its part's column, and shown or hidden as the program has it.
- **Sizes.** Main's pty is kept at its part's size, every worker's at the workers' part, the
  chosen one or not. Choosing another worker is then only a redraw of its buffer: no program is
  told a new size and none has to repaint. A split appearing or going — the first worker, the
  last one gone, a zoom, a resize across 120 columns — resizes main and every worker.
  - On a SIGWINCH the host resizes the canvas and lays out again: main's pty and grid, and an
    `r` frame on every view whose size changed.
  - The last size from any client wins. Once the pane and attach are gone, the host is the
    only client that sizes a worker.
- **Not passed through.** Because nothing is passed through, a program's queries never reach the
  person's terminal, so the problem of an orphan reply after a switch does not exist here. A
  program's outputs that must still go out are caught by its emulator from the raw stream, before
  the library sees it (the library ends an OSC string at 0x9C, below), and ride in the screen
  frame — `Screen.Title` and `Screen.Events` — rather than a frame `e` of their own: one stream
  keeps their order with the rows, and main's grid gives them the same way.
  - Events: OSC 52 (a copy; its query `;?` is not forwarded), OSC 9 (but `9;4` progress), 99 and
    777 notifications, and the bell. They are written to the person's terminal from any tab, the
    hidden ones included, once each: a grid keeps the last 32 with their sequence, and a reader
    gets those after the sequence it had, so a view opened late never sets an old copy again.
  - The title (OSC 0 and 2) is a tab's state: the focused program's is set when it changes or the
    tab is switched. The host saves the person's title with `CSI 22;0t` on entering and gives it
    back with `CSI 23;0t` on leaving.

  OSC 8 hyperlinks are lost until the frames carry them. OSC 4 queries of the palette go
  unanswered (a probe of the grid, October 19, 2026).
- **Rejected: a scroll-region trick.** The idea is to tell the program a smaller size and place
  its raw output inside margins. It cannot give a split, because DECSLRM side margins are
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
  - The automaton matches the legacy byte `0x1c` and its CSI-u and modifyOtherKeys forms; the
    CSI-u form with the lock keys' bits and an explicit press or repeat counts too.
  - A key's release (the kitty protocol's event type 3) never cancels a pending prefix, and the
    release of the prefix and of the key taken after it are the host's, never forwarded.
  - It never matches inside a bracketed paste.
  - A pending prefix lapses after 2 s.
- **The commands after the prefix:**
  - `n`/`p`: the next or previous tab, from the focused one;
  - `1`–`9`: a worker, chosen and focused;
  - `0` or `m`: main, focused;
  - ← and →: the focus to main or to the chosen worker, the arrow in its legacy, application
    and CSI forms with no modifier; its release under event types is the host's until another
    key comes;
  - `z`: zoom the focused program or end the zoom;
  - the prefix again: sends Ctrl+\\ to the focused program;
  - any other key: cancels, which is a choice; §2 forwarded both keys.
- **The person's terminal mirrors the focused program's input modes.** At a switch, and whenever
  the focused program changes them, the host sets on the person's terminal the program's:
  - DECCKM and DECNKM, so Right arrives as `ESC O C` where the program asked for it;
  - bracketed paste 2004;
  - focus 1004;
  - the mouse modes (9, 1000, 1002, 1003, 1006).

  Keys then pass unchanged. At the exit every mode is reset. Step 1 is built this way.
- **Mouse, from step 2.** The host keeps SGR mouse (1002 with 1006) and bracketed paste on
  the terminal, and stops mirroring the mouse. It asks for 1003, motion without a button,
  only while the focused program asks for it: Claude Code does, for its hover.
  - A click on the bar chooses a tab.
  - Above the bar, mouse events go to the program under the pointer, focused or not, at the cell
    of its own part and in the form its modes ask for, but only when the program turned mouse
    on; otherwise, and on the divider, they are dropped. A left click also focuses the program.
    While a button is down, every event goes to the program it went down on, held to its part's
    edge, so a drag across the divider ends where it began.
  In step 1, while mirroring, a mouse event below the region (on the bar) is the host's and is
  never forwarded.
- **Paste.** A paste reaches the program framed by 200~/201~ only when the program enabled 2004;
  otherwise it goes bare.
- **Focus events.** These go through when the program asked for them (1004).
- **Keys.** All other input goes to the focused program unchanged.
- **The keyboard protocols, mirrored too** (found live by the owner, October 19, 2026: Shift+Enter
  in Claude Code arrived as a plain Enter and sent the message). Every emulator keeps the
  program's kitty keyboard flags — push `CSI > f u`, pop `CSI < n u`, set `CSI = f ; mode u` —
  and its modifyOtherKeys level (`CSI > 4 ; n m`, reset by `CSI > 4 m` or `CSI > 4 n`), and
  answers the query `CSI ? u` from them; the library knows neither. A program that hears no
  answer falls back to legacy keys, which is what Claude Code did behind the emulator.
  - The person's terminal gets the focused program's flags as one entry pushed over its own
    stack, replaced by a pop and a push at a change or a switch, and its modifyOtherKeys level;
    at the exit, and on every signal, the entry is popped only if pushed, so the person's shell
    keeps its own, and modifyOtherKeys is reset to the terminal's setting. Keys then pass
    unchanged, in the form the program asked for.
  - One stack serves both screens of an emulator, where the protocol keeps one each: a program
    that pushes before it takes the alternate screen keeps its keys.
  - What a terminal without the protocol sends stays legacy: the program was told the protocol
    is on, and Shift+Enter is Enter there, as before.
  - Seen on October 19, 2026: Claude Code 2.1.302 asks `CSI ? u` with DA1 and, answered, pushes
    5 (disambiguate, alternate keys) and sets modifyOtherKeys 2; unanswered it stays legacy. pi
    1.2.0 pushes 7 (with event types) before it asks, and sets modifyOtherKeys 2 only unanswered.
  - **Built and checked live, October 19, 2026, Claude Code 2.1.302 and pi 1.2.0**
    (`internal/host/terminal/keyboard.go`, `internal/host/hostmode/kitty.go`): a script acting as
    a kitty-capable terminal ran `rewake --host --main claude` on the cheapest model. The
    terminal got 5 and modifyOtherKeys 2; `CSI 13;2u` made a new line in main's prompt and in a
    Claude Code worker's; Enter ran main's `!rewake space open`; Ctrl+\\ n, 2 and m switched as
    `CSI 92;5u`, with its release under pi's flags; a switch to pi gave 7 and modifyOtherKeys
    reset, a new line in pi's editor too, and back to main 5 and 2 again. At the exit, on
    SIGTERM and on SIGHUP, the terminal was left with no flags and modifyOtherKeys reset.

## Scrollback, selection, copy

- **Scrollback.** The host runs on the alternate screen, so the person's terminal scrollback
  holds nothing of the programs. That is lost.
  - The host starts Claude Code with `CLAUDE_CODE_NO_FLICKER=1`, unless the person set it, so
    that each program scrolls its own history with the forwarded wheel. The claude adapter
    adds it for main when the launch is in host mode (`LaunchRequest.Host`); a worker inherits
    it through main's environment, which `space open` runs in.
  - A host-side history view is a later idea, not a step here.
- **Selection.** A drag in the region is the program's own selection, and its copy reaches the
  terminal clipboard through the forwarded OSC 52. Shift+drag gives the terminal's native
  selection over the visible composed screen, bar and divider included.
- **Default colours.** The host asks the person's terminal for its default colours at start —
  OSC 10 and 11, then DA1 as the sentinel every terminal answers, waiting at most 300 ms — before
  the harness starts, and seeds main's emulator with them (`SetDefaultForegroundColor`/
  `Background`), so a program that asks for the background sees the real one. Workers get them
  through the environment rather than a view frame: the host puts `REWAKE_HOST_COLORS=fg;bg`
  (XParseColor forms) into main's harness, `space open` inherits it, and the space host seeds its
  grid from it. A reply that comes after the wait is dropped from the input, never typed into the
  program. A terminal that answers neither leaves the library's defaults.

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
   - **Built and checked live, October 18, 2026, Claude Code 2.1.302** (`internal/host/hostmode`,
     the canvas in `internal/host/terminal/canvas.go`): main and a worker on the cheapest model, the worker opened by
     `space open`; typed into main, switched with Ctrl+\\ n, a line answered by the worker,
     Ctrl+\\ m back with main's prompt kept, a resize to 100×30 reaching both, Ctrl+C twice
     ending main with every mode reset and the space closed. A key typed into main shows in about
     33 ms: main's screen is taken 16 ms after a change, for the same reason a view waits 30 ms.
   - **Found live:** the emulator library ends an OSC string at the byte 0x9C even inside a
     UTF-8 character (x/ansi v0.11.7 and v0.11.8), so Claude Code's title `✳ Claude Code` was
     drawn over its logo, in every grid. The grid drops the non-ASCII bytes of an OSC string
     before the emulator; step 4's title forwarding must take that into account.
   - **Left for later steps:** main's last words after it exits — Claude Code's resume line —
     are drawn on the alternate screen and lost with it.
2. **Mouse, paste and focus.** The forwarding rules above. Live check: Claude Code's fullscreen
   scroll and its click selection in a worker.
   - **Built and checked live, October 18, 2026, Claude Code 2.1.302**
     (`internal/host/hostmode/mouse.go`): main on the cheapest model opened a worker by its own
     `space open`, and both ran fullscreen, the worker by inheritance. A click on the bar showed
     the worker; the wheel scrolled its answer of 120 lines back to its top; a drag over six
     lines made it say `copied 17 chars to clipboard`; a paste of two lines stayed in its prompt,
     framed, its line break not sending it. At the exit every mode was reset.
   - The copy reaches the person's clipboard only with step 4's OSC 52.
3. **The sidebar.** The width threshold, `s`, and clicks on it. Replaced by the split (step 5)
   on October 19, 2026, by the owner's decision; its code is gone.
   - **Built and checked live, October 19, 2026, Claude Code 2.1.302**
     (`internal/host/hostmode/sidebar.go`): on a 130-column terminal main opened a worker by
     `!rewake space open`; the sidebar listed main and `w1-claude` with its harness, model and
     `idle`, main's screen drawn 102 columns wide; Ctrl+\\ s hid it and main repainted at 130,
     Ctrl+\\ s showed it again; a click on the worker's name in the sidebar showed the worker.
4. **Events and colours.** OSC 52, title, bell, default colours; the theme's indices checked
   live. Live check: a copy from a worker lands in the clipboard.
   - **Built and checked live, October 19, 2026, Claude Code 2.1.302, pi 1.2.0**
     (`internal/host/terminal/osc.go`, `internal/host/hostmode/colors.go`): under an outer
     terminal answering OSC 10/11 with a light theme, `echo $REWAKE_HOST_COLORS` printed both
     colours in main and in a Claude Code worker; a drag over four lines of the worker's output
     sent `OSC 52;c` with exactly those lines to the outer terminal; the titles `✳ Claude Code`
     (with its spinner frames) and pi's `π - <cwd>` reached it, the latter on a switch to pi; on
     exit the title was given back. Indexed colours were already written as indices from step 1
     (`colorFor` in `canvas.go`); pi draws its own theme in RGB.
   - Not the design's `e` frame: the events and the title ride in the screen frame, and the
     colours reach workers through the environment (above).

5. **The split** (the owner's decision, October 19, 2026, in place of the sidebar). Main left, the
   chosen worker right, focus by click or Ctrl+\\ arrows, Ctrl+\\ z, no split below 120 columns,
   a worker's turn as a mark on its tab.
   - **Built and checked live, October 19, 2026, Claude Code 2.1.302**
     (`internal/host/hostmode/split.go`, the panes of `internal/host/terminal/canvas.go`): a
     program acting as a 130×30 terminal ran `rewake --host --main claude` on the cheapest model;
     main opened a worker by `!rewake space open`, and the screen split as main's 65 columns,
     the divider and the worker's 64, each drawn for its width. Text typed went to main;
     after Ctrl+\\ → to the worker; a click in main's part gave main the keys again; Ctrl+\\ z
     gave the worker the whole width and back; a click on `0 main` on the bar focused main; at
     100 columns main alone, Ctrl+\\ 1 the worker alone, and back at 130 the split; `/exit` gave
     the terminal back and the space closed.
   - Seen there: under the kitty protocol a terminal sends Ctrl+C as `CSI 99;5u`, and Claude
     Code 2.1.302 does not take a bare 0x03 as one.

The tests are the subset of §2's checklist that "Which tests to write" keeps: the automaton, a
restore on every signal, and the routing of mouse and paste by mode.
