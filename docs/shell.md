---
description: rewake's shell — its layout and panes, who owns each pseudo-terminal, closing, the raw stream a worker's pane is fed, the space host's emulator, keys, colours, the bar, the strip, the briefs view, and how it ships.
---

# The rewake shell

The shell is an application in TypeScript on OpenTUI (`@opentui/core` and `@opentui/solid`,
MIT), running inside the person's terminal with a terminal pane per program; Solid is the
framework. There is no web UI. From 120 columns, once a worker exists, main is on the left and
the chosen worker, or the briefs ([below](#the-briefs)), on the right, half of the width less a divider column, the narrower part when
the width is odd; below 120 columns, with no worker, or zoomed, the focused program takes the
whole width and the bar switches the window between the workers. Each pane keeps a
column of padding on both sides; the program's size is the width less two. A click on a button
or in a pane focuses it. The prefix is Ctrl+\\, which no harness binds. The sources live in
`shell/` at the repository's root, outside the Go module. The mod knows nothing of who draws the
screen.

## Who owns which pty

- **Main's pty is the shell's.** The shell starts main's ordinary wrapper (`rewake claude
  [args]`, the launch's options kept) in a pty of its own with `Bun.spawn` and `terminal`, so
  main's run is that wrapper's and the shell is its terminal. Main's pane gets the widget's
  answers to main's queries (source `response`), since no other emulator stands between them.
- **A worker's pty stays its space host's** ([space.md](space.md)); its pane is fed from the
  host's socket ([below](#how-a-pane-is-fed-the-raw-stream)).
- **Closing ends everything**: sessions must not live on unseen, so there is no detach and no
  re-attach. The shell ends when main ends, and when its own terminal goes — a hangup, or its
  input at an end. Ending, it closes main's pty, so main hears a hangup, and waits a bound for
  main's group before SIGTERM and SIGKILL; main's run ends, and each space host, which reads that
  run, ends its worker. A shell killed outright closes main's pty the same way, by the kernel.
  OpenTUI's own handlers of SIGHUP and the other ending signals only destroy the renderer, and the
  panes' ptys would keep the process alive with both sessions; so the shell makes its renderer with
  no exit signals and follows its terminal itself — SIGHUP, SIGTERM, SIGINT, SIGQUIT, SIGPIPE, its
  input at an end or failing, and its parent gone, since a closed window takes the shell's parent
  with it.

### The closing screen

From the moment main's end begins until the shell exits, the whole screen is one spinner and
"closing rewake…" in the middle (`shell/src/closing.tsx`); no pane or bar shows, and no key
reaches a program. It is a layer over the work, and the work underneath is hidden as one, so a
terminal background the shell does not know never lets a pane show through.

What begins it:

- **Main leaving its screen.** Claude Code runs fullscreen and leaves the alternate screen at once
  at every end of its process, before `session.end`, so the shell starts the layer on the
  `?1049l` (or 47, 1047) the scanner reads in main's stream (`shell/src/scan.ts`), and the frame
  that would show main's left screen shows the layer instead. A leave may be no end, so under a
  layer begun by a leave alone main keeps its focus and keys, and the layer is lifted when main
  comes back to the alternate screen — an external editor's end — or when main still runs 5 s
  after the last sign. Text main writes after a leave is no sign of a pause: an exit prints
  "Resume this session with: claude --resume …" on the screen it went to. A leave within a second
  after the shell sent main a Ctrl+Z (0x1a, or its kitty or modifyOtherKeys form, `isSuspend` in
  `shell/src/keys.ts`) begins nothing.
- **Main's closing mark.** Main's mod tells its wrapper `session/closing` at `session.end`, for
  `/exit`, Ctrl+C, Ctrl+D and `/logout` alike
  ([the record](research-claude-mod.md#the-end-of-the-process-as-the-mod-hears-it)); a pi main's
  extension tells it `closing` at a quit's `session_shutdown`
  ([research-pi.md](research-pi.md#the-adapters-wire-contract)). The wrapper of a main in the shell
  then writes `ESC ] 6262 ; rewake-closing BEL` on main's pane (`internal/host/start.go`), which
  the scanner reads (the widget ignores an OSC it does not know). It makes the layer an end, the
  keys held, where no leave began it, and restarts the 5 s, since the session has ended and only
  its process is left to exit. The mark is not sent at `/exit`'s `command.run`: `/exit` with work
  running opens Claude Code's dialog "Background work is running" after that hook, and a layer
  then would hide the dialog.
- **The fallbacks**: the room's listing reading main's run ended for a space the shell shows,
  main's process ending, and the shell ending itself on SIGTERM, SIGINT or SIGQUIT; not when the
  terminal is lost, with nowhere to draw.

Once main has ended, the layer stays until every worker the shell showed reads closed in the
room's listing (polled every 200 ms, at most 10 s, the space host's own bounds for ending a
worker), and at least 600 ms. Without it the shell exited before a frame of the layer was drawn
and the worker ended unseen. Main's process exits a few hundred milliseconds after it leaves its
screen; its workers' close starts at the closing mark ([space.md](space.md#closing)).

**Ctrl+Z** suspends and resumes at once. Claude Code leaves the screen, writes "has been
suspended", waits for `SIGCONT` and sends `SIGTSTP` to its group, which on the shell's pane is
orphaned, so the kernel drops the stop; main's wrapper catches that `SIGTSTP` and sends the
`SIGCONT` ([launch.md](launch.md#launching-a-harness)). Claude Code's screen is back about 20 ms
after the key, the normal screen with "has been suspended" showing for one frame between.

## How a pane is fed: the raw stream

The space socket serves one mode, `b` (bytes):

- On connect, the host writes a **preamble** built from its emulator, then every byte the program
  writes after it. The cut is made in `Grid.commit`, under the grid's lock: the snapshot and the
  client's subscription are taken there, between two committed pieces, so the client gets exactly
  the bytes the emulator took after the snapshot. Not in `relay.pump`: `Grid.Write` queues, and
  the holder delays a suffix, so a read of the pty is not the emulator's boundary.
- **The preamble:** the alternate screen if on, the screen as ANSI (each run placed, indexed
  colours kept as indices), the pen text is written in now (a cut between an SGR and its text is
  common; the emulator keeps the pen private, so rewake follows SGR itself, a pen restored by
  DECRC excepted), the cursor's place and visibility, the title, and the input modes — DECCKM,
  DECNKM (the application keypad), bracketed paste 2004, focus 1004, mouse 9, 1000, 1002, 1003
  and 1006, modifyOtherKeys, and the kitty stack as one push per entry in order, so a later pop
  finds what the program pushed before. A client that attached late without the modes would lose
  Shift+Enter.
- **The cut is safe inside a sequence.** A pty read has no sequence boundary, so a commit can end
  inside one: after `ESC [ ? 2004` the emulator shows paste still off, and the `h` comes later.
  The grid keeps the bytes committed since its parser last stood in the ground state, and the
  preamble ends with them, so the widget's parser finishes the sequence from the live bytes as the
  host's did. Without it a late widget kept paste off and sent a paste of two lines bare.
- The client writes two frames: `i` keys, `r` a size; the last size wins. A client more than 4096
  pieces behind is dropped, its stream closed rather than given a gap; the shell attaches again a
  second later, on its widget reset and seeded anew.
- **The host keeps answering the program's queries**, as it must while nobody is attached; the
  pane's widget answers too, so the shell drops what the widget sends with source `response` for a
  host-fed pane — never for main's. A host-fed program hears the host's answer: DA1, `CSI ? u`,
  OSC 10/11, XTVERSION as `libghostty`, DECRQM 2026 as known (reset, set inside a frame); kitty
  graphics and OSC 4 go unanswered, and the widget is not made a second responder. XTVERSION
  matters: Claude Code sends its mode queries, DECRQM 2026 among them, only after it hears a
  terminal's name, and without synchronized output the widget drew each piece of a frame, the
  cursor flickering.
- Scrollback before the attach is not replayed: Claude Code runs fullscreen
  (`CLAUDE_CODE_NO_FLICKER=1`, which the claude adapter adds for a shell's main unless the person
  set it, `LaunchRequest.Shell`; a worker inherits it) and scrolls its own history.

## The space host's emulator

`internal/host/terminal` holds the pseudo-terminal, opened with the standard library (Linux
`/dev/ptmx` and its ioctls, `Setsid` and `Setctty`), and the emulator that keeps each worker's
screen for the preamble, behind one small interface — write bytes, resize, read the screen as
styled rows with the cursor — that no other package sees.

The emulator is `github.com/charmbracelet/x/vt`, pinned at v0.1.0, an exception to the dependency
rule decided by the owner: a nested harness's screen needs CSI, SGR in all its colour forms,
scroll regions, the alternate screen, modes and the widths of wide glyphs, and it is the only
maintained Go emulator that tracks widths; the st ports (`hinshun/vt10x`, `micro-editor/terminal`)
would shift every column after a wide glyph. It brings `charmbracelet/ultraviolet` and
`charmbracelet/x/ansi` as direct requirements, with about fourteen indirect ones, and the layout
test holds the exception to its place: `github.com/charmbracelet/` is imported by
`internal/host/terminal` alone ([rules/layout.md](rules/layout.md#the-import-rule)). Its
pseudo-version that agent-deck pins loses a combining mark after an ASCII base within one write;
v0.1.0 keeps it.

What the library does not do, and what the package does about it:

- **It ends a glyph cluster at the end of every write.** A mark, a variation selector or a joiner
  that arrives in the next read becomes a cell of its own (both versions). The feeder holds back
  the last cluster of a read, at most 64 bytes, and writes it with the next read or alone after
  5 ms (`hold.go`); a test fails when the library starts joining across writes, so the hold can go.
- **Its reply pipe is unbounded and blocking**, so a goroutine drains it into a queue of 64 and
  counts what does not fit: a program that floods queries and never reads its input loses answers
  and never stops the emulator in the middle of a write.
- **Closing the emulator races its reply reader**, so the package closes the pipe's write end and
  never the emulator.
- **Its damage tracking is never cleared**, so the package diffs the rows itself.
- **Rows are read by walking cells**, not by parsing `Render()`: measured on a 200x50 screen, a
  repaint of about 35 KB through `Write` took 1.5 ms, `Render()` 0.9 ms, and the walk of all
  10 000 cells 0.16–0.19 ms; the walk is also the only route to styled runs.
- **It ends an OSC string at the byte 0x9C even inside a UTF-8 character** (x/ansi v0.11.7 and
  v0.11.8), which drew Claude Code's `✳ Claude Code` title over its logo; the grid drops the
  non-ASCII bytes of an OSC string before the emulator and keeps the title whole beside it for the
  preamble.
- **What a run cannot carry is dropped**: concealed text is drawn as blanks; blink, the
  underline's style and colour, and hyperlinks are left out; a wide glyph cut at the right edge is
  drawn as blanks.

A post of keys is written with exactly one `write(2)` on the master's raw descriptor, at most
4096 bytes, under one mutex: a key split across two writes reaches the program as two keys. A
canary (`testdata/canary.golden`) holds what the library does as wanted, so an update that changes
it fails that one test.

The locale: a program in the C locale draws a wide glyph as question marks, so a space host gives
its worker `LC_CTYPE=C.UTF-8` when the locale names no UTF-8 (`terminal.Environ`), and main's
wrapper in the shell does the same for main (`terminal.UTF8Locale`), whose workers inherit it.

## What the widget lacks and what the shell does about it

- **OSC 52, title.** The widget swallows them. The shell scans the bytes before `write()` — a
  stateful scanner, since a sequence can split across reads — and calls
  `renderer.copyToClipboardOSC52` and `setTerminalTitle` (main's pane's only: a worker's title stays on its pane, so the outer terminal's title, which a host such as herdr shows, never reads as a worker's whichever pane has the focus). A program that
  copies through the machine's clipboard sends no OSC 52, and inside the shell such a copy never
  reaches the person's terminal: pi 1.3.0 in fullscreen on WSL without `WT_SESSION` copies with
  PowerShell. The shell starts main with `PI_CLIPBOARD_OSC52=1` unless the person set it, pi's own
  switch for a multiplexer, so pi sends OSC 52 after its own write; main's workers inherit it.
- **Bell, OSC 9/99/777 notifications** are not delivered to the person's terminal, by the owner's
  decision. OpenTUI offers no way to write them anyway.
- **OSC 8** is lost.
- **Indexed colours** are resolved to RGB by the widget. The shell asks the person's terminal for
  its 16 colours and default colours at start (`askColors` in `shell/src/colors.ts`: OSC 4/10/11
  written once, replies read through `renderer.subscribeOsc` for up to 2.5 s) and writes them into
  every widget as OSC 4/10/11; a theme changed while the shell runs is not followed.
  `renderer.getPalette` is not used: it gives up after a fixed 300 ms of silence and returns
  nothing without failing. Without a background reply the panes are `transparentBackground`, so
  the terminal's own shows. Without a foreground reply the widget would write white where the
  program asked for none; the shell seeds a stand-in grey no palette names, `#c0c1bf`, and turns
  cells in that colour into OpenTUI's default-foreground intent, written as SGR 39
  (`keepDefaultForeground`). A part answer is common inside a multiplexer: herdr 0.9.3 answers
  OSC 4 from its own palette and OSC 10/11 only when it learned its host's colours, so there an
  empty `REWAKE_HOST_COLORS` and a transparent background are not a fault.
- **The hosts are seeded too**, since a host-fed program hears the host's answer: the person's
  default colours reach every host before its program starts — `REWAKE_HOST_COLORS=fg;bg`
  (XParseColor forms) in main's environment, which `space open` inherits and the space host reads
  to seed its grid. A colour the terminal did not give is left empty. Unseeded, a worker on a light
  terminal would hear a black background while its pane shows white.
- **Mouse.** The widget forwards the wheel, clicks and drags in the program's mode, and scrolls its
  own scrollback when the program takes no mouse. OpenTUI's own selection takes the drag, so the
  shell keeps a pane `selectable` only while its program has no mouse tracking on (9/1000/1002/1003,
  read in the same scanner) and copies a finished selection through OSC 52. Over rewake's face the
  face takes the mouse itself and none of it reaches the pane's widget ([face.md](face.md#copying)).
- **Keys.** OpenTUI pushes kitty flags 5 on the person's terminal, parses every key and the widget
  re-encodes it in the focused program's current mode. Key releases reach a program only if the
  terminal sends them. A space host answers a worker's `CSI ? u` from the flags it follows:
  unanswered, Claude Code stays on legacy keys, where Shift+Enter is Enter; answered, it pushes 5
  and sets modifyOtherKeys 2, and pi pushes 7. Under the kitty protocol Ctrl+C arrives as
  `CSI 99;5u`, and Claude Code does not take a bare 0x03 as one.

## Solid: registering the widget, room for a modal

- `extend({ pane_terminal: PaneTerminal })`. Solid constructs with `{ id }` only, so a subclass
  fixes in its constructor what the widget reads only there: `maxScrollback`, and the mouse-down
  hook — assigning `onMouseDown` later **replaces** the widget's own mouse forwarding. `onData`,
  `onTerminalResize` and `selectable` are ordinary props.
- **A modal fits without touching the layout**: a `<box position="absolute" zIndex>` centred over
  the row of panes, mounted under `<Show>`. Opening it blurs the focused pane and focuses the
  modal's input; the panes keep reading their programs and drawing underneath. Closing restores
  the pane's focus on the next tick, after the modal unmounts — restored at once, the first key
  after closing was lost. The closing screen is such a layer, full-size, with every pane blurred
  under it.

## The prefix and the room

- **Ctrl+\\** is read in `prependInputHandlers` on raw sequences, before OpenTUI parses them: the
  byte 0x1c, its CSI-u forms with event types and lock bits, its modifyOtherKeys form; it waits two
  seconds for its command, and the releases of the prefix and of the key taken are swallowed. The
  commands: `n`/`p` the next or previous of main, the workers and, while a worker is up, the
  briefs at the bar's end; `1`–`9` a worker, `0` or `m` main, each focused;
  ← → the focus to main or to the chosen worker; `z` zooms the focused program or ends the zoom;
  `b` shows the briefs in the worker's place ([below](#the-briefs)), and while they have the keys
  goes back to what was chosen and focused before; `v` flips a dev worker's pane between rewake's
  face and the native one ([face.md](face.md#the-face)); the prefix again sends Ctrl+\\ to the focused
  program, but not to the briefs; any other key cancels.
- **The room** comes from one call of rewake's own, polled once a second (`shell/src/room.ts`):
  the hidden `rewake room --json`, whose shape is the schema `internal/cli/room_view.schema.json`.
  `tools/protogen` writes the shell's types (`shell/src/room_view.gen.ts`) and the command's Go
  types from it, and its test fails when a checked-in file differs, so the shell maps nothing by
  hand and the command's answer cannot drift from what the shell reads. The view carries the
  spaces not over — each with the socket path of its host, so the shell never builds one, and its
  `mainRun`, whose reading `ended` begins the closing screen — and the sessions for the strip
  (below); while the briefs show, also the briefs (below). The command is hidden like `statusline`:
  the shell is its only caller, and a documented command would promise agents a shape. The
  existing `list`, `space list` and `brief` `--json` forms stay as they are.

## The bar

The bar and the hints are the shell's two lines of navigation, laid out as the owner drew it on
October 26, 2026: every pane has its status line, the strip, under it; the hints sit under main's,
and the bar under the worker's.

```
   main's pane    |   worker's pane
   main's strip   |   worker's strip
   the hints      |   the bar
```

- **Both panes** (from 120 columns, once a worker exists, no zoom): main's column is its pane, its
  strip and the hints; the worker's column is its pane, its strip and the bar. Both panes end two
  rows above the bottom, so the columns end together.
- **One pane** (narrow, a zoom, or no worker): the shown pane, its strip under it, then the hints
  and, while a worker exists, the bar, both across the whole width. The pane is three rows short of
  the screen, two with no worker.
- **The buttons** (`shell/src/bar.tsx`): each worker by its name, in the order the spaces opened,
  with no number — the prefix's digits still choose by place. Main has no button, by the owner's
  decision: the prefix's `m` (or `0`, or ←) reaches it in every layout, a click on its pane or its
  strip while both show, and the hints say so while a worker has the keys. With no worker the bar
  is not drawn. The focused button is filled in the workers' colour, bold, with near-black or
  near-white text, whichever reads; the others sit on a grey with that colour as the text: the
  person's palette entry 2, and 8 for the grey, each with a fallback of its own
  (`shell/src/theme.ts`). In the wide layout, while main has the focus, the worker shown beside it
  is underlined: filled is where the keys go, underlined is shown beside it without them. The
  `briefs` button stands at the bar's right end (the owner's word, October 30, 2026), the workers'
  at its left, and a zoom shows `zoomed` just before it. It is chosen like a worker's: filled in the
  terminal's foreground, bold, while the briefs have the keys, underlined while they show beside
  main, otherwise in the same grey with the terminal's foreground, since it names no program; a
  click on it shows them. Where the buttons do not all fit, the workers' row is cut a cell before
  `zoomed` and `briefs`, which stay whole: no button lies over another, and the prefix's digits
  still reach a worker the cut hid.
- **The activity edge.** The owner asked on October 27, 2026 to see in the navigation which
  workers work, without the row moving when a mark goes, on the edge of the navigation's button,
  and the same day, having seen it live, set what it says: «если мигает, значит воркер встал по
  какой-то причине; если просто горит, значит работает; если не горит, значит в idle». The first
  cell of each worker's button, its left padding, is that edge, painted by the mark of the worker's
  activity verdict, which rewake judges and the shell only shows
  ([session-state.md](session-state.md#activity)): **steady** while it is `working` or
  `compacting`; **blinking**, lit and dark half a second each, when it has stopped for a reason —
  `waiting for a permission`, `waiting for an answer`, `waiting at a dialog`, or a turn whose
  snapshot went stale; **not lit**, the button's own background, when `idle`, `unknown` or ended.
  Both lit states are one colour, so the meaning is in the motion: of the palette's bright yellow
  (11), its yellow (3) and the terminal's foreground, the one that stands out most from both the
  focused button and the others (`shell/src/theme.ts`). The cell is always there, so a mark going
  on or off moves nothing; one clock drives every blinking edge, a redraw per phase, and stops
  while none blinks. The briefs button has none.
- **The hints** (`shell/src/hints.ts`): what the keys do now — where typing goes, the prefix
  commands that apply, and after a Ctrl+\\ the commands themselves until one is taken or the two
  seconds pass. A hint is a row of one table keyed by the context (main focused, a worker focused,
  the briefs focused, the prefix waiting), kept by a `when` over the focus, the zoom and the number of workers; a new
  hint or a new context is a row there. "keys go to <name>" comes first, the rest after it in the
  table's order. In main's column the line has half the width, so a hint that does not fit is left
  out whole, with those after it; only the first is cut at the width.

**One look for the shell's own rows**: the strip, the bar and the hints are each a `ShellRow`
(`shell/src/bar.tsx`), and they share these rules.

- **Inset and alignment.** A row keeps the panes' column of padding on both sides, and its content
  starts at the left of what is inside, in the column of the programs' own text. Nothing is
  centred; only the bar's `zoomed` and `briefs` keep the right end.
- **Air.** A row is drawn on a band, the terminal's background moved 6% towards the ink that reads
  on it (`#282828` gives `#343434`), across its whole width, padding included: wide, each column's
  strip and the row under it read as one band of two rows; narrow, the strip, the hints and the bar
  as one of three. A terminal that does not say its background gets no band.
- **Colours and weights.** Words are in the terminal's own foreground; what names a program is bold
  in its colour — the focused button filled, the strip's name, the hints' keys in the focused
  program's colour; main's colour, the palette's entry 4, is apart from the workers'. Parts are separated by ` · ` in a faint colour, halfway from the foreground to
  the background (palette 8 where the terminal did not say both): OpenTUI 0.6.6 writes no dim over
  the terminal's own foreground.
- **Selection.** A selection over these rows is one colour, a pane's (`selectionBg`/`selectionFg`,
  the stand-in grey where the terminal said no foreground), not each part in its own colour.

## The pane

Each fact around a pane is in one place, alike for Claude Code and pi (the owner, October 31, 2026,
brief 60): where the session works on the row above the pane, who it is and how it stands on the
strip under it, and its room work at its input. The program has two rows less.

```
~/.rewake2-worktrees/rewake-44a66b/wt-5928f6 · ⎇ brief60-pane-facts         the top row
  … the session's own screen, native or drawn by rewake's face …
  ── ⏵⏵ auto mode ──────────────── brief 57 3/6 ─                             the face's input rule
  ⏵⏵ auto mode on (shift+tab to cycle) · brief 57 3/6                         or Claude Code's hint line
write-claude (code) · working · <model> low · 90% 42m                  230k/1M   the strip
```

- **The top row** (`shell/src/path.tsx`): the room view's `path`, the directory the session works in
  now, with the person's home as `~`, then ` · ⎇ ` and the branch checked out there. Claude Code's
  mod sends `$.session.cwd()` with every sample ([claude.md](claude.md#telemetry)), which a worktree
  entry moves and a `cd` in Bash does not; pi's directory is the one it started in, its record's
  `cwd`. A worker whose row is not read yet shows its space's checkout. Too long, the path loses whole
  directories from its start, `…/worktrees/wt-3836d0`, then letters of its last; too narrow for both,
  the branch stays, cut at its end. A press on it chooses the pane. The briefs, shown in a worker's
  place, take the row into their own height, so their strip stands where a pane's does.
- **At the input**: the run's room work, the room view's `work` (`internal/core/work`): its open
  briefs with their ticks, `brief 57 3/6`, and the reports it waits on, `⧗ 3`; nothing when there is
  none. On the face's input rule ([face.md](face.md#the-look)) and at the end of Claude Code's hint
  line ([claude.md](claude.md#the-band)), the same text. A narrow row cuts it first, so it is never
  the only place: the briefs view, `rewake brief show` and `rewake inbox --awaited` say the same.
- **The person's own status line** is off in every Claude Code run rewake starts
  ([claude.md](claude.md#the-cache-ttl)): the strip says what it said.

Seen live on October 31, 2026 (Claude Code 2.1.310, pi 1.4.0, a private room): a Claude Code
worker's top row went from the project to `/tmp/rw60/worktrees/repo-6a8dae/wt-bfe5d6 · ⎇ wt-bfe5d6`
on its `worktree_enter`, and to `…/repo-6a8dae/wt-bfe5d6 · ⎇ wt-bfe5d6` at 46 columns; main's hint
line ended `· ⧗ 1`, the worker's `· brief 1 2/3`, a pi worker's rule `brief 2 1/2`. At 46 columns
the strip had dropped the model and the limits and kept the context and the cache, the pi rule
kept its brief, and Claude Code's hint line was cut to `· bri`, our tail first.

## The strip

The row under each pane, main's and every worker's, a shell's row, is one line drawn by the shell
(`shell/src/strip.tsx`), the same for every harness. Main's strip,
asked for by the owner on October 26, 2026, reads main's own row of the same listing; main has no
type, so its name stands alone. The context stands at the strip's right edge, the rest from the
left, and the account's limits are on main's strip alone (the owner, October 31, 2026).

```
write-claude (code) · working · <model> low thinking · 90% 42m                          230k/1M
main · working · <model> high · 90% 42m · 5h 37% · 7d 9%                               674k/1M
```

- **What it says**: the session and its type, on a pane drawn by rewake's face as on a native one,
  the activity as the label of its verdict, which
  `rewake list` shows too, after `dev rewake` or `dev native` on a dev worker's strip (`working`, `waiting for a permission`, `waiting at a dialog`, `idle`,
  `compacting`, with ` (stale)` once it is not current; nothing while it is `unknown`;
  [session-state.md](session-state.md#activity)), the model and
  effort, with `thinking` after them while the harness says thinking is on — Claude Code's
  session setting, pi's level for a model with reasoning — and nothing when it is off or not told, the prompt cache, and at the right edge the context as used and window. A value rewake does
  not know is left out; a window without a measure reads `–/<window>`. The model reads as the
  harness's display name (`configuredModelName` of the telemetry, less a trailing parenthesis),
  else the template's
  `launch.label`, else the id. The window is the limit the conversation is kept to: Claude Code's
  launch `--autocompact`, which the host applies over the mod's model window. The percent left
  every strip on October 30, 2026 (brief 44, the owner's word): its colour stays on the figures. The cache is `rewake list`'s Cache cell (`cache` and `cacheLife`): the hit
  percent of the last request and the time the cache has left — `90% 42m`, `90% <1m`, `90% —` once
  it expired, `90%` where the harness gives no expiry; left out where it gives no cache. The time is
  counted from `expiresAt` on the shell's own clock every five seconds, so it ticks down between
  polls. On main's strip alone the account's limits follow, `5h 37%` and `7d 9%`, main's `limits` in
  the room view: the share used of the five-hour and seven-day windows Claude Code's mod reads from
  `$.session.usage()` ([claude.md](claude.md#telemetry)); one account's, so a worker's strip does
  not repeat them; left out where the harness reports none, as off a subscription, and on pi,
  which reports none.
- **Where it comes from**: telemetry is shown only to a verified main, and the shell is main's
  terminal and no session, so it proves itself by the process tree: it passes `--wrapper <pid>`,
  the pid of main's wrapper, and the command takes it only for the `servicePid` of a main
  record whose parent is the process that called it, as the shell is main's wrapper's parent
  (`asMain` in the view). Then the view carries the sessions' telemetry and marks main's own row
  `own`, which is main's strip; without the proof — before main's record is found — the sessions
  come bare and the strip reads `main` alone, and the next call proves it.
- **The branch**, on the top row, is in the view: the space record's `checkout.branch` while the
  session works in its space's checkout (made by its launch, or entered); otherwise the branch
  checked out at its `path`, read from `HEAD` by the command itself on each call (no process, so no
  cache), a detached head as its first seven characters.
- **Narrow**: the line fits the pane, the context kept whole at the right edge at least a
  separator's width from the rest. Too long, it leaves out the model and effort, then the seven
  days' limit, then the five hours', then the cache; what still does not fit on the left is cut with
  `…`. The name, the activity and the context stay; only a strip narrower than the context cuts it.
- **Colours**: the name bold in its program's colour, main's or the workers'; the rest in the terminal's own
  foreground with faint separators; the context and each limit in the palette's green, yellow or
  red from 50% and 80% used; the cache in the palette's cyan while it is warm, faint once it expired.
- **How fresh**: a Claude Code worker's mod samples after each main-loop step's response, at a
  turn's end and with each heartbeat ([claude.md](claude.md#telemetry)), so the context grows
  through a long turn; pi publishes on its own events. The poll adds up to a second.

## A picture of a window

`rewake screen` draws any session's window in a shadow tree of the shell's, from the same
`Window` as the person's; it is [screen.md](screen.md). Every byte a pane is fed is also written
to its shadow widget.

## The briefs

A brief is a markdown document main keeps in the room for one task ([briefs.md](briefs.md));
the person reads them here and edits nothing. Decided by the owner, October 26, 2026, after a
modal over the panes was tried live: the briefs show in the worker's pane, in place of a worker.
The bar's `briefs` button, the prefix's `b`, or `n`/`p` past the last worker choose them as a
button chooses a worker (`shell/src/briefs.tsx`, `show` in `shell/src/panes.ts`): wide, they take
the worker's place beside main; narrow or zoomed, the whole width. Choosing a worker brings that
worker back, and `b` while the briefs have the keys goes back to what was chosen and focused
before them. Main's pane stays as it is: the keys reach the briefs only while their pane has the
focus, as a worker's reach its terminal, and a click on main's pane or strip gives main the keys
with the briefs still beside it. The prefix's commands all apply but the prefix twice, since the
briefs have no program to pass Ctrl+\\ to. With no worker up the briefs do not open, by the key or
otherwise, and they leave when the last worker goes: they are the navigation's, which shows only
while a worker is up (main's decision, October 26, 2026). A press on a row gives the briefs the focus
before it opens the brief, since the row then unmounts and the press bubbles no further; their
scrollboxes take no focus, so the keys reach the view once. The view's state — the brief opened, the row chosen, the last answers —
outlives a worker being chosen over it, as a worker's screen does.

- **The list**: two lines per brief, as the owner asked on October 27, 2026 («1 строка будет
  заголовок, и который будет как кнопка и взаимодействие как с кнопкой; 2 строка будет инфа по
  пунктам, комментам и агентам которые с брифом, это не будет выделяться как кнопка»). The first,
  `3 · Auth refactor` (`headOf`, `shell/src/briefs.ts`), is a button on the buttons' grey; the
  second, faint and indented under the title, is `2/5 · 4 comments · write-claude, review-claude
  (review, ended)` (`infoOf`): a run given the brief to review is marked `review`, a binding whose
  run has `ended` is marked so, and a closed brief says
  `closed` there, its title's words in a faint of their own for the grey (`faintOnIdle`,
  `shell/src/theme.ts`: the ink on the grey moved 40% towards it), since the faint colour is that
  very grey where the terminal did not say its foreground and background. Oldest first, so a new brief lands at the bottom
  (the owner, the same day: «новое добавляется вверх, а не вниз»): the open ones by id, then the
  closed ones by id (`ordered`); the CLI's own JSON stays newest first. ↑ ↓ and Enter, or a click
  on either line, open one; the selection, in a pane's selection colours, covers the first line
  alone and stays on its brief when a poll changes the order. A record the list could not read
  (`unreadable` in its JSON) is a faint row with its path and why, not an absent brief; an empty
  list says main makes briefs with `rewake brief new`.
- **The fold**, by the owner's question of October 27, 2026 («а вообще если их убирать из списка?»)
  and main's decision on it: the list shows the open briefs only, and under them, when any brief
  is closed, one faint row `closed · N` (`rowsOf`, `foldText`), selectable like a brief's first
  line. Enter or a click unfolds the closed briefs under it, in the two-line form, and again folds
  them; with no open brief and some closed the list is that row alone. The fold is a signal of
  the view, kept while the shell runs and not across restarts. Nothing is deleted: the closed
  briefs stay on disk and `rewake brief list --closed` shows them, which the room view still carries.
- **The strip** under the view says what shows, a shell's row as a worker's strip is:
  `briefs · 3 open · 1 closed` under the list, `brief 3 · 2/5 · 4 comments` under one, the first
  part bold, cut with `…` at the width (`stripOfBriefs`, `shell/src/briefs.ts`).
- **The hints** while the briefs have the keys are their own keys — ↑↓ Enter on the list; the
  scroll, ←→ and Backspace on a brief — then the prefix's way to main, n/p and the zoom. They do not
  name Esc: it still goes back, but it is the harnesses' interrupt, and a slip of the focus would
  send it to a worker.
- **One brief**: a navigation row in place of a header line, by the owner's request of October 27,
  2026 — the header and the markdown's own title read the same words twice, and «чтобы не всегда
  Esc жать (что весь тревожно иногда, это же команда "прервать" агента и случайно можно отправить
  в окно воркера)». The row is ` ‹ briefs `, back to the list, then the open briefs in the list's
  order as ` 3 Title `, and the shown one where it is closed (closed briefs are reached from the
  list only), one cell apart on the bar's grey, the shown one filled as the bar's briefs button is
  while it has the keys (`navOf`, `shell/src/briefs.ts`). The titles are cut to one length, the
  longest that fits the row on one line, whichever brief shows, so changing it moves nothing;
  where not even the numbers fit, the row is cut at the width. A click on an item, or ← →, shows
  another brief; Backspace, or Esc, goes back to the list. The text keeps its title as written;
  who is bound is on the list's second line, the number, done of total and comments on the strip.
  Under the row, a scrolling
  box holding the text, rendered by OpenTUI's `<markdown>` (headings, emphasis, fenced code and
  lists in the shell's colours; a task-list line shows as its `- [ ]` or `- [x]`), a rule, and the
  comments, each its author, its capacity and time (the person's clock) over its text —
  `review-pi · review · 2026-10-27 06:14` (`authorOf`), the name alone for a comment written before
  capacities; a comment's text is rendered as markdown too, so its lines stay lines on the screen
  and when copied (the owner saw a comment copy as one line, October 27, 2026). ↑ ↓, PgUp PgDn, Home End
  and the wheel scroll. The text keeps a column clear of the scrollbar, as the panes keep one from
  the screen's edge (the owner saw it stick to the bar, October 27, 2026): the bar lies over the
  scrolling box's last column and the content leaves two, so the text is as wide whether the bar
  shows or not, in the list too. The compiled shell draws it: the
  default tree-sitter client `<markdown>` starts works inside `bun build --compile`, checked on
  October 26, 2026 on Linux x64.
- **How it reads.** From the same room view as the strip, in the same one call a second, which
  the view adds `--briefs` to while it shows (the list, closed briefs included, an opened
  brief's row being made of it) and `--brief <id>` to while a brief shows (its text and comments);
  nothing is asked while a worker is chosen in their place (`wanting` in `shell/src/room.ts`,
  `takeBriefs` in `shell/src/briefs.tsx`). Briefs come only to the proven shell of main
  (`asMain`); a list that cannot be read comes as `briefsProblem` and shows as the list's line, never as no briefs. An answer is drawn only when it differs
  from the last, and the scroll position stays. A read that fails, does not parse, or comes before
  main's run is known shows one line on the view's bottom row and is replaced by the next good
  answer; it never ends the shell.
- **Rejected**: plain text for the brief — a table or nested list main writes would read badly, and
  `<markdown>` (on `marked`, already in the shell's tree) adds no dependency; and watching `briefs/`
  from the shell, which would build state paths itself and read records past the CLI's checks.
  The shell shows a change within a second and the command's time.

## How rewake starts the shell, and how it ships

- `rewake claude [args]` and `rewake pi [args]` at a terminal: when standard input and output are
  both terminals and the adapter offers spaces, the Go launcher execs the shell as
  `<shell> --room <room> -- <rewake> <its own arguments>` with `REWAKE_SHELL_MAIN=1`, so no rewake
  process stays between the person's terminal and the shell. The shell runs that command line on
  its pty; that launch, seeing the variable, takes it out of its environment and runs main
  ([launch.md](launch.md#the-shell)).
- **Shipping.** The person has no Bun. `bun build --compile` with `@opentui/solid/bun-plugin`
  gives one executable with the runtime and OpenTUI's native library inside. Bun is MIT, but the
  runtime it embeds carries JavaScriptCore, LGPL-2, which the dependency rule does not name; main
  accepted it inside the compiled executable. `shell/build.ts` compiles it for a target;
  `scripts/pack.sh` installs every Linux native library of the lockfile first
  (`bun install --frozen-lockfile --os=linux --cpu='*'`, since Bun installs only this machine's by
  default) and puts `bin/rewake-shell` into each platform package beside the Go binary: about
  99 MB, 49.5 MB packed, for x64 and arm64 alike. rewake finds it next to its own executable;
  `REWAKE_SHELL` names another — an edited copy run from source with `bun`, which
  `shell/rewake-shell` does (`bun install` in `shell/` once). rewake's face, code of its own in
  `face/`, is compiled into the shell as its second entrypoint; `rewake-shell --face-check` starts
  it and says whether it answers, and the release gate runs it on the installed shell
  ([face.md](face.md#the-faces-own-code)). The compiled shell reads no
  `bunfig.toml`, `.env`, `tsconfig.json` or `package.json` of the directory it starts in: a preload
  named in one stopped it in a first build.
