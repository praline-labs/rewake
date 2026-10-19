# Design: the rewake shell

The owner's decision of October 19, 2026: the host mode (`internal/host/hostmode`, a compositor
written by hand in Go) gives way to a **shell** — an application in TypeScript on OpenTUI
(`@opentui/core` and `@opentui/solid`, MIT), running inside the person's terminal, with a terminal
pane per program. Solid is the owner's choice of framework, the same day. No web UI. The layout
is the one the owner approved for the host mode: from 120 columns, once a worker exists, main on
the left and the chosen worker on the right, half of the width less a divider column, the
narrower part when the width is odd; below 120 columns, with no worker, or zoomed, the focused
program takes the whole width and the bar switches the window between main and the workers. The
bar numbers `main` 0 and the workers from 1, the focused tab bracketed; a click on a tab or in a
pane focuses it. The prefix is Ctrl+\\, which no harness binds. The host mode was removed on
October 19, 2026, deleted rather than archived (the owner's rule: what is no longer needed goes
at once). "Checked live" below means the spike of October 19, 2026: Claude Code
2.1.302 and pi 1.2.0 in two panes of `@opentui/core` 0.6.6 on Bun 1.4.0, under a stand-in
terminal in a pty; the owner checked step 1 on their own terminal the same day.

## Who owns which pty

- **Main's pty is the shell's.** The shell starts main's ordinary wrapper (`rewake claude
  [args]`, transparent, the launch's options kept) in a pty of its own with `Bun.spawn` and
  `terminal`, as the spike did, so main's run is that wrapper's and the shell is its terminal.
  Main's pane gets the widget's answers to main's queries (source `response`), since no other
  emulator stands between them.
- **A worker's pty stays its space host's** ([space.md](../rules/space.md), SP1 unchanged); its
  pane is fed from the host's socket (below).
- **Closing ends everything** (the owner's decision, October 19, 2026: sessions must not live on
  unseen; no detach, no re-attach). The shell ends when main ends, and when its own terminal goes —
  a hangup, or its input at an end. Ending, it closes main's pty, so main hears a hangup, and
  waits a bound for main's group before SIGTERM and SIGKILL; main's run ends, and each space host,
  which reads that run, ends its worker. A shell killed outright closes main's pty the same way, by
  the kernel. In the spike a shell whose terminal was killed kept running with both sessions for
  minutes: OpenTUI's own handlers of SIGHUP and the other ending signals only destroy the
  renderer, and the panes' ptys keep the process alive. So the shell makes its renderer with no
  exit signals and follows its terminal itself — SIGHUP, SIGTERM, SIGINT, SIGQUIT, SIGPIPE, its
  input at an end or failing, and its parent gone, since a closed window takes the shell's parent
  with it. Checked in step 1: the stand-in terminal killed, the shell and main were gone within a
  second, the space host and its worker within three; `/exit` in main ended the shell with code 0.

## How a pane is fed: the raw stream

The space socket's mode `b` (bytes), its only one since the view mode `v` went with the host
mode:

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
  Shift+Enter — Superset's bug, the lesson of the research.
- **The cut is safe inside a sequence.** A pty read has no sequence boundary, so a commit can end
  inside one: after `ESC [ ? 2004` the emulator shows paste still off, and the `h` comes later.
  The grid keeps the bytes committed since its parser last stood in the ground state, and the
  preamble ends with them, so the widget's parser finishes the sequence from the live bytes as the
  host's did. Without it a late widget kept paste off and sent a paste of two lines bare, with a
  carriage return (rev-pi's probe against the widget, October 19, 2026).
- The client writes two frames: `i` keys, `r` a size; the last size wins.
  A client more than 4096 pieces behind is dropped, its stream closed rather than given a gap; the
  shell attaches again a second later, on its widget reset and seeded anew.
- **The host keeps answering the program's queries**, as it must while nobody is attached; the
  pane's widget answers too, so the shell drops what the widget sends with source `response`
  (the `onData` callback says which) for a host-fed pane — never for main's. What a host-fed
  program hears is the host's answer: DA1, `CSI ? u`, OSC 10/11, DECRQM 2026 as unsupported;
  XTVERSION, kitty graphics and OSC 4 go unanswered. The spike's
  widget answers more (XTVERSION as `libghostty`, a kitty graphics query, OSC 4); that is not
  the host's set, and the widget is not made a second responder to make up for it.
- Scrollback before the attach is not replayed: Claude Code runs fullscreen
  (`CLAUDE_CODE_NO_FLICKER=1`, which the claude adapter adds for a shell's main unless the person
  set it, `LaunchRequest.Shell`; a worker inherits it) and scrolls its own history.

## What the widget lacks and what the shell does about it

- **OSC 52, title.** The widget swallows them. The shell scans the bytes before `write()` — a
  stateful scanner, since a sequence can split across reads — and calls
  `renderer.copyToClipboardOSC52` and `setTerminalTitle` (the focused pane's). Both checked live.
  A program that copies through the machine's clipboard sends no OSC 52, and inside the shell such
  a copy never reaches the person's terminal: pi 1.3.0 in fullscreen selects with the mouse itself
  and on WSL without `WT_SESSION` copies with PowerShell, showing "Copied!" with nothing in the
  person's clipboard (the owner's report, October 19, 2026). The shell starts main with
  `PI_CLIPBOARD_OSC52=1` unless the person set it, pi's own switch for a multiplexer, so pi sends
  OSC 52 after its own write; main's workers inherit it like the colours. Checked under a
  stand-in terminal with pi as main: OSC 52 reached the terminal, none before the switch.
- **Bell, OSC 9/99/777 notifications** are not delivered to the person's terminal: the owner's
  decision of October 19, 2026, for now. OpenTUI offers no way to write them anyway; its native
  session takes only the title, the mouse, kitty flags and program status.
- **OSC 8** is lost.
- **Indexed colours** are resolved to RGB by the widget. The shell asks the person's terminal for
  its palette (`renderer.getPalette`; the spike took 16 entries, the call takes up to 256) and
  default colours at start and writes them into every widget as OSC 4/10/11: checked live,
  palette index 6 drew as the terminal's own colour 6. Main's decision of October 19, 2026: the
  palette and the default colours are seeded at start, and a theme changed while the shell runs
  is not followed; patching OpenTUI's resolution of indexed colours is not on the plan. The
  default background goes out as an explicit colour (`transparentBackground` untried).
- **The hosts are seeded too**, since a host-fed program hears the host's answer, not the
  widget's: the person's default colours reach every host before its program starts —
  `REWAKE_HOST_COLORS=fg;bg` (XParseColor forms) in main's environment, which `space open`
  inherits and the space host reads to seed its grid. The shell takes them from the same
  `getPalette` answer as the widgets' seed and starts main with them, so the terminal is asked
  once. Unseeded, a worker on a light terminal hears a black background while its pane shows
  white.
- **Mouse.** The widget forwards the wheel, clicks and drags in the program's mode, and scrolls its
  own scrollback when the program takes no mouse. OpenTUI's own selection starts on a left press
  and takes the drag; so the shell keeps a pane `selectable` only while its program has no mouse
  tracking on (it reads 9/1000/1002/1003 in the same scanner) and copies a finished OpenTUI
  selection through OSC 52. Checked live both ways.
- **Keys.** OpenTUI pushes kitty flags 5 on the person's terminal itself, parses every key and the
  widget re-encodes it in the focused program's current mode. Key releases reach a program only if
  the terminal sends them. A space host answers a worker's `CSI ? u` from the flags it follows:
  unanswered, Claude Code 2.1.302 stays on legacy keys, where Shift+Enter is Enter; answered, it
  pushes 5 and sets modifyOtherKeys 2, and pi 1.2.0 pushes 7 (seen October 19, 2026). Under the
  kitty protocol Ctrl+C arrives as `CSI 99;5u`, and Claude Code 2.1.302 does not take a bare 0x03
  as one.
- **A title with non-ASCII text.** The emulator library (x/ansi v0.11.7 and v0.11.8) ends an OSC
  string at the byte 0x9C even inside a UTF-8 character, which drew Claude Code's `✳ Claude Code`
  over its logo; a host's grid drops the non-ASCII bytes of an OSC string before its emulator and
  keeps the title whole beside it for the preamble.
- **The locale.** A program in the C locale draws a wide glyph as question marks, so a space host
  gives its worker `LC_CTYPE=C.UTF-8` when the locale names no UTF-8 (`terminal.Environ`), and
  main's wrapper in the shell does the same for main (`terminal.UTF8Locale`), whose workers
  inherit it. Checked live October 19, 2026: under `LANG=C` main saw `LC_CTYPE=C.UTF-8`, under
  `LANG=en_US.UTF-8` nothing added.

## Solid: registering the widget, room for a modal

- `extend({ pane_terminal: PaneTerminal })`. Solid constructs with `{ id }` only, so a subclass
  fixes in its constructor what the widget reads only there: `maxScrollback`, and the mouse-down
  hook — assigning `onMouseDown` later **replaces** the widget's own mouse forwarding (found in the
  spike: presses stopped reaching Claude Code). `onData`, `onTerminalResize` and `selectable` are
  ordinary props.
- **A modal fits without touching the layout** (checked in the spike): a `<box position="absolute"
  zIndex>` centred over the row of panes, mounted under `<Show>`. Opening it blurs the focused pane
  and focuses the modal's input; the panes keep reading their programs and drawing underneath.
  Mouse events over the panes are meant to go to a full-size backdrop below the modal, not to
  the programs (not yet tried). Closing restores the pane's focus on the next tick, after the
  modal unmounts — restored at once, the first key after closing was lost.

## The prefix and the room

- **Ctrl+\\** is read in `prependInputHandlers` on raw sequences, before OpenTUI parses them: the
  byte 0x1c, its CSI-u forms with event types and lock bits, its modifyOtherKeys form; it waits two
  seconds for its command, and the releases of the prefix and of the key taken are swallowed. The
  commands: `n`/`p` the next or previous tab; `1`–`9` a worker, `0` or `m` main, each focused;
  ← → the focus to main or to the chosen worker; `z` zooms the focused program or ends the zoom;
  the prefix again sends Ctrl+\\ to the focused program; any other key cancels. Checked live in
  the CSI-u form with ← → and `z`.
- **The room** comes from rewake's own commands, polled once a second: `rewake list --json` for
  each session's activity, model, effort and context, `rewake space list --json` for the spaces, which
  gains each open space's socket path so the shell never builds a path itself. A stream comes only
  if polling is felt.

## How rewake starts the shell, and how it ships

- `rewake claude [args]` at a terminal (the owner's decision of October 19, 2026; `--shell` went
  with it): when standard input and output are both terminals and the adapter offers spaces, the
  Go launcher execs the shell as
  `<shell> --room <room> -- <rewake> <its own arguments>` with `REWAKE_SHELL_MAIN=1`, so no rewake
  process stays between the person's terminal and the shell. The shell runs that command line on
  its pty; that launch, seeing the variable, takes it out of its environment and runs main
  ([launch.md](../launch.md#the-shell)).
- **Shipping.** The person has no Bun. `bun build --compile` with `@opentui/solid/bun-plugin`
  gives one executable with the runtime and OpenTUI's native library inside: built and run in the
  spike, 100 MB, 31 MB compressed. Bun is MIT, but the runtime it embeds carries JavaScriptCore,
  LGPL-2, which the dependency rule does not name; main's decision of October 19, 2026 accepts it
  inside the compiled executable. `shell/build.ts` compiles it for a target;
  `scripts/pack.sh` installs every Linux native library of the lockfile first
  (`bun install --frozen-lockfile --os=linux --cpu='*'`, since Bun installs only this machine's by
  default) and puts `bin/rewake-shell` into each platform package beside the Go binary: about
  99 MB, 49.5 MB packed, for x64 and arm64 alike. rewake finds it next to its own executable;
  `REWAKE_SHELL` names another — the owner's edited copy run from source with `bun`, which
  `shell/rewake-shell` does (`bun install` in `shell/` once). The compiled shell reads no
  `bunfig.toml`, `.env`, `tsconfig.json` or `package.json` of the directory it starts in: a
  preload named in one stopped it in a first build. The sources live in `shell/` at the
  repository's root, outside the Go module.
- **The mod does not change**: it knows nothing of who draws the screen.

## Steps, each checked live

1. **Main and one worker in two panes.** Go: the `b` mode, its cut and preamble, the socket path in
  `rewake space --json`, `--shell` with the colours. Shell: main in its pty, the worker from its
  space host, the split, the bar, the prefix, resize, and the ending above. Check: Shift+Enter in
  both, a worker shown after main opened it, a resize across 120 columns, a worker on a light
  terminal hearing a light background, and the terminal's window closed leaving no shell, main or
  worker running. Tests: the cut inside a sequence and the preamble's modes, a contract between
  processes.
  Done October 19, 2026, with the scanner, the palette seed and the selection rule from step 2,
  small enough to come along ([roadmap](../roadmap/2026-10-19-shell-step1.md)).
2. **Events and colours.** Bell and notifications settled: not delivered, by the owner's decision.
  Check: a copy from each pane lands in the clipboard of the person's terminal.
3. **Shipping**: the compiled shell in the npm packages, found by rewake. Check: an install by the
  release gate starts it. Done October 19, 2026: the gate checks each package's shell, its
  architecture and, after the install, that the shell beside the installed binary starts and
  draws main's output; live, an npm-installed package ran `rewake --shell --main claude`
  ([roadmap](../roadmap/2026-10-19-shell-ship.md)).
4. **The host mode removed**: `internal/host/hostmode`, the canvas, the view mode, `--host`, the
  host mode's workflow scenario and its document, deleted. Done October 19, 2026
  ([roadmap](../roadmap/2026-10-19-host-mode-removed.md)).

Panels that are not terminals (inbox, reports, permission buttons) and the modal come after,
as ordinary Solid components over the same layout.
