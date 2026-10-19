# Design: the rewake shell

The owner's decision of October 19, 2026: the host mode (`internal/host/hostmode`, a compositor
written by hand in Go) gives way to a **shell** — an application in TypeScript on OpenTUI
(`@opentui/core` and `@opentui/solid`, MIT), running inside the person's terminal, with a terminal
pane per program. Solid is the owner's choice of framework, the same day. No web UI. The layout
stays the approved one ([host-mode.md](host-mode.md), "Layout"): main left, the chosen worker
right from 120 columns, the bar choosing the worker, focus by a click or the prefix, zoom; below
120 columns the bar switches the whole window. The prefix stays Ctrl+\\. Once the shell works the
host mode is removed. "Checked live" below means the spike of October 19, 2026: Claude Code
2.1.302 and pi 1.2.0 in two panes of `@opentui/core` 0.6.6 on Bun 1.4.0, under a stand-in
terminal in a pty, as the host mode's steps were checked; the person's own terminal is not yet.

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

A new mode `b` (bytes) of the space socket, beside the view mode `v`, which goes with the host
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
  Shift+Enter — Superset's bug, the lesson of the research. The modes are read without draining
  the emulator's events (`look` takes them), so a copy or bell meant for another client is not
  lost.
- **The cut is safe inside a sequence.** A pty read has no sequence boundary, so a commit can end
  inside one: after `ESC [ ? 2004` the emulator shows paste still off, and the `h` comes later.
  The grid keeps the bytes committed since its parser last stood in the ground state, and the
  preamble ends with them, so the widget's parser finishes the sequence from the live bytes as the
  host's did. Without it a late widget kept paste off and sent a paste of two lines bare, with a
  carriage return (rev-pi's probe against the widget, October 19, 2026).
- The client writes the frames the view takes today: `i` keys, `r` a size; the last size wins.
  A client more than 4096 pieces behind is dropped, its stream closed rather than given a gap; the
  shell attaches again a second later, on its widget reset and seeded anew.
- **The host keeps answering the program's queries**, as it must while nobody is attached; the
  pane's widget answers too, so the shell drops what the widget sends with source `response`
  (the `onData` callback says which) for a host-fed pane — never for main's. What a host-fed
  program hears is the host's answer: DA1, `CSI ? u`, OSC 10/11, DECRQM 2026 as unsupported;
  XTVERSION, kitty graphics and OSC 4 go unanswered, as in the host mode today. The spike's
  widget answers more (XTVERSION as `libghostty`, a kitty graphics query, OSC 4); that is not
  the host's set, and the widget is not made a second responder to make up for it.
- Scrollback before the attach is not replayed: Claude Code runs fullscreen
  (`CLAUDE_CODE_NO_FLICKER=1`, kept from the host mode) and scrolls its own history.

## What the widget lacks and what the shell does about it

- **OSC 52, title.** The widget swallows them. The shell scans the bytes before `write()` — a
  stateful scanner, since a sequence can split across reads — and calls
  `renderer.copyToClipboardOSC52` and `setTerminalTitle` (the focused pane's). Both checked live.
- **Bell, OSC 9/99/777 notifications.** OpenTUI offers no way to write them to the terminal; its
  native session takes only the title, the mouse, kitty flags and program status. A raw write to
  stdout between frames is untested. Main's decision of October 19, 2026: step 2 first tries that
  raw write; if it is not clean, an issue goes upstream.
- **OSC 8** stays lost, as in the host mode.
- **Indexed colours** are resolved to RGB by the widget. The shell asks the person's terminal for
  its palette (`renderer.getPalette`; the spike took 16 entries, the call takes up to 256) and
  default colours at start and writes them into every widget as OSC 4/10/11: checked live,
  palette index 6 drew as the terminal's own colour 6. Main's decision of October 19, 2026: the
  palette and the default colours are seeded at start, and a theme changed while the shell runs
  is not followed; patching OpenTUI's resolution of indexed colours is not on the plan. The
  default background goes out as an explicit colour (`transparentBackground` untried).
- **The hosts are seeded too**, since a host-fed program hears the host's answer, not the
  widget's: the person's default colours reach every host before its program starts, as in the
  host mode — `REWAKE_HOST_COLORS=fg;bg` in main's environment, which `space open` inherits and
  the space host reads to seed its grid. The shell takes them from the same `getPalette` answer
  as the widgets' seed and starts main with them, so the terminal is asked once; the host mode's
  `colors.go` goes with it. Unseeded, a worker on a light terminal hears a black background while
  its pane shows white.
- **Mouse.** The widget forwards the wheel, clicks and drags in the program's mode, and scrolls its
  own scrollback when the program takes no mouse. OpenTUI's own selection starts on a left press
  and takes the drag; so the shell keeps a pane `selectable` only while its program has no mouse
  tracking on (it reads 9/1000/1002/1003 in the same scanner) and copies a finished OpenTUI
  selection through OSC 52. Checked live both ways.
- **Keys.** OpenTUI pushes kitty flags 5 on the person's terminal itself, parses every key and the
  widget re-encodes it in the focused program's current mode, so the host mode's mirroring of each
  program's flags goes away. Key releases reach a program only if the terminal sends them.

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
  seconds for its command, and the releases of the prefix and of the key taken are swallowed. The commands are the host mode's: `n`/`p`, `0`–`9`, `m`, ← →, `z`, the prefix
  again. Checked live in the CSI-u form with ← → and `z`.
- **The room** comes from rewake's own commands, polled once a second: `rewake list --json` for
  each session's activity, model, effort and context, `rewake space --json` for the spaces, which
  gains each open space's socket path so the shell never builds a path itself. A stream comes only
  if polling is felt.

## How rewake starts the shell, and how it ships

- `rewake --shell claude [args]` replaces `--host` (an alias keeps `rewake = ["--shell"]`): the Go
  launcher checks the role and the terminal as `--host` does and execs the shell as
  `<shell> --room <room> -- <rewake> <its own arguments>` with `REWAKE_SHELL_MAIN=1`, so no rewake
  process stays between the person's terminal and the shell. The shell runs that command line on
  its pty; that launch, seeing the variable, takes it out of its environment and runs main
  ([launch.md](../launch.md#the-shell)). An alias works unchanged: the same arguments expand it
  again.
- **Shipping.** The person has no Bun. `bun build --compile` with `@opentui/solid/bun-plugin`
  gives one executable with the runtime and OpenTUI's native library inside: built and run in the
  spike, 100 MB, 31 MB compressed. Bun is MIT, but the runtime it embeds carries JavaScriptCore,
  LGPL-2, which the dependency rule does not name; main's decision of October 19, 2026 accepts it
  inside the compiled executable. It goes into each
  platform package beside the Go binary (`scripts/pack.sh`), and rewake finds it next to its own
  executable; `REWAKE_SHELL` names another — the owner's edited copy run from source with `bun`.
  The sources live in `shell/` at the repository's root, outside the Go module; until step 3,
  `shell/rewake-shell` runs them with `bun` (`bun install` in `shell/` once), named by
  `REWAKE_SHELL`.
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
2. **Events and colours.** Bell and notifications settled. Check: a copy from each pane lands in
  the clipboard of the person's terminal.
3. **Shipping**: the compiled shell in the npm packages, found by rewake. Check: an install by the
  release gate starts it.
4. **The host mode removed**: `internal/host/hostmode`, the canvas, the view mode, `--host`, and
  their documents to the archive.

Panels that are not terminals (inbox, reports, permission buttons) and the modal come after,
as ordinary Solid components over the same layout.
