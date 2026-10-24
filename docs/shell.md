---
description: rewake's shell — its layout and panes, who owns each pseudo-terminal, closing, the raw stream a worker's pane is fed, the space host's emulator, keys, colours, the bar, the strip, and how it ships.
---

# The rewake shell

The shell is an application in TypeScript on OpenTUI (`@opentui/core` and `@opentui/solid`,
MIT), running inside the person's terminal with a terminal pane per program; Solid is the
framework. There is no web UI. From 120 columns, once a worker exists, main is on the left and
the chosen worker on the right, half of the width less a divider column, the narrower part when
the width is odd; below 120 columns, with no worker, or zoomed, the focused program takes the
whole width and the bar switches the window between main and the workers. Each pane keeps a
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
  `renderer.copyToClipboardOSC52` and `setTerminalTitle` (the focused pane's). A program that
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
  read in the same scanner) and copies a finished selection through OSC 52.
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
  commands: `n`/`p` the next or previous tab; `1`–`9` a worker, `0` or `m` main, each focused;
  ← → the focus to main or to the chosen worker; `z` zooms the focused program or ends the zoom;
  the prefix again sends Ctrl+\\ to the focused program; any other key cancels.
- **The room** comes from rewake's own commands, polled once a second (`shell/src/room.ts`):
  `rewake space list --json` for the spaces, which carries each open space's socket path so the
  shell never builds a path itself, and whose `mainRun` reading `ended` begins the closing screen;
  `rewake list --json` for the strip (below).

## The bar

The bar and the hints are the shell's two lines of navigation. The hints are the bottom row across
the whole width in every layout. While both panes show, the bar sits under main's pane, in main's
column, the way the strip sits under a worker's pane: main's pane ends two rows above the bottom,
and the worker's column ends one row above it with its strip on the bar's row, so both columns end
together. While one pane shows (narrow, or a zoom), the bar and the hints take the two bottom lines
across the whole width, and the pane the rest.

- **The buttons** (`shell/src/bar.tsx`): ` main `, then each worker by its name, in the order the
  spaces opened, with no number — the prefix's digits still choose by place. The focused one is
  filled in its colour, bold, with near-black or near-white text, whichever reads; the others sit
  on a grey with their colour as the text. Main's colour is apart from the workers': the person's
  palette entry 4 for main and 2 for the workers, 8 for the grey, each with a fallback of its own
  (`shell/src/theme.ts`). In the wide layout, while main has the focus, the worker shown beside it
  is underlined: filled is where the keys go, underlined is shown beside it without them. A zoom
  shows `zoomed` at the right end.
- **The hints** (`shell/src/hints.ts`): what the keys do now — where typing goes, the prefix
  commands that apply, and after a Ctrl+\\ the commands themselves until one is taken or the two
  seconds pass. A hint is a row of one table keyed by the context (main focused, a worker focused,
  the prefix waiting), kept by a `when` over the focus, the zoom and the number of workers; a new
  hint or a new context is a row there. "keys go to <name>" comes first, the rest after it in the
  table's order. A line longer than the width is cut at its end.

**One look for the shell's own rows**: the strip, the bar and the hints are each a `ShellRow`
(`shell/src/bar.tsx`), and they share these rules.

- **Inset and alignment.** A row keeps the panes' column of padding on both sides, and its content
  starts at the left of what is inside, in the column of the programs' own text. Nothing is
  centred; only `zoomed` keeps the right end.
- **Air.** A row is drawn on a band, the terminal's background moved 6% towards the ink that reads
  on it (`#282828` gives `#343434`), across its whole width, padding included: wide, the bar's and
  the strip's row and the hints' row read as one band of two rows; narrow, the strip, the bar and
  the hints as one of three. A terminal that does not say its background gets no band.
- **Colours and weights.** Words are in the terminal's own foreground; what names a program is bold
  in its colour — the focused button filled, the strip's name, the hints' keys in the focused
  program's colour. Parts are separated by ` · ` in a faint colour, halfway from the foreground to
  the background (palette 8 where the terminal did not say both): OpenTUI 0.6.6 writes no dim over
  the terminal's own foreground.
- **Selection.** A selection over these rows is one colour, a pane's (`selectionBg`/`selectionFg`,
  the stand-in grey where the terminal said no foreground), not each part in its own colour.

## The strip

Each worker pane's last row, a shell's row, is one line drawn by the shell (`shell/src/strip.tsx`),
the same for every harness; the worker's program has one row less. Main gets none: its own status
line shows the same.

```
write-claude (code) · working · ⎇ wt-fd7596 · <model> low · 23% 230k/1M · 90% 42m
```

- **What it says**: the session and its type, the activity (`working`, `waiting for <name>`,
  `waiting at a dialog` ([a dialog nobody is asked about](permission-requests.md#a-dialog-nobody-is-asked-about)),
  `idle`, `, compacting`, ` (stale)` once the run stopped publishing), the branch, the model and
  effort, the context as filled percent, used and window, and the prompt cache. A value rewake does
  not know is left out; a window without a measure reads `–/<window>`. The model reads as the
  harness's display name (`configuredModelName` of the telemetry), else the template's
  `launch.label`, else the id. The window is the limit the conversation is kept to: Claude Code's
  launch `--autocompact`, which the host applies over the mod's model window, the percent
  recomputed against it. The cache is `rewake list`'s Cache cell (`cache` and `cacheLife`): the hit
  percent of the last request and the time the cache has left — `90% 42m`, `90% <1m`, `90% —` once
  it expired, `90%` where the harness gives no expiry; left out where it gives no cache. The time is
  counted from `expiresAt` on the shell's own clock every five seconds, so it ticks down between
  polls.
- **Where it comes from**: `rewake list --json` shows telemetry only to a verified main
  (`internal/cli/session_state.go`), and the shell is main's terminal, so it asks as main's run.
  That run's record is the main whose `servicePid` is the shell's own child, main's wrapper; its
  name and epoch go to `REWAKE_SESSION` and `REWAKE_EPOCH` of the poll, and a new epoch is read
  again at once.
- **The branch** is the space record's `checkout.branch` for a worker in a checkout of its own
  (made by its launch, or entered); otherwise the branch checked out where the session started,
  read from `git` at most every five seconds, a detached head as its short commit.
- **Narrow**: the line fits the pane. Too long, it leaves out the model and effort, then the cache,
  then the branch; what still does not fit is cut with `…`. The name, the activity and the context
  stay.
- **Colours**: the name bold in the workers' colour of the bar; the rest in the terminal's own
  foreground with faint separators; the context in the palette's green, yellow or red from 50% and
  80%; the cache in the palette's cyan while it is warm, faint once it expired.
- **How fresh**: a Claude Code worker's mod samples after each main-loop step's response, at a
  turn's end and with each heartbeat ([claude.md](claude.md#telemetry)), so the context grows
  through a long turn; pi publishes on its own events. The poll adds up to a second.

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
  `shell/rewake-shell` does (`bun install` in `shell/` once). The compiled shell reads no
  `bunfig.toml`, `.env`, `tsconfig.json` or `package.json` of the directory it starts in: a preload
  named in one stopped it in a first build.
