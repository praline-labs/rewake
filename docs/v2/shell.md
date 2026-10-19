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

- **Every program runs under a host, and the shell is only a client.** A worker already does:
  its space host owns its pty and emulator ([space.md](../rules/space.md), SP1 unchanged). Main
  gets the same — a detached **main host**, the space host's code with no main run to watch, which
  owns main's pty and ends when main's harness ends. Closing or crashing the shell then ends no
  session, and the owner can restart a shell they have just edited without losing work. Nothing
  runs longer than its session, so "no service beside the sessions" holds.
- **One kind of pane.** Every pane is fed from a host's socket the same way, so the shell has no
  code for owning a process. The first step still lets the shell start main itself in its own pty
  (`Bun.spawn` with `terminal`), as the spike did; the main host comes in step 3, if the owner
  agrees (question 1).

## How a pane is fed: the raw stream

A new mode `b` (bytes) of the space socket, beside the view mode `v`, which goes with the host
mode:

- On connect, the host writes, under the grid's lock and before any live byte, a **preamble**
  built from its emulator: the alternate screen if on, the screen as ANSI, the cursor's place and
  visibility, the title, and the modes — DECCKM, bracketed paste 2004, focus 1004, mouse 9, 1000,
  1002, 1003 and 1006, the kitty flags as one push, modifyOtherKeys. Then every byte the program
  writes, as it comes. One lock around the snapshot and the subscription loses or repeats no byte.
  A client that attached late without the modes would lose Shift+Enter — Superset's bug, the
  lesson of the research.
- The client writes the frames the view takes today: `i` keys, `r` a size; the last size wins.
- **The host keeps answering the program's queries**, as it must while nobody is attached; the
  pane's widget answers too, so the shell drops what the widget sends with source `response`
  (the `onData` callback says which) for a host-fed pane. The spike showed the widget answering
  DA1, XTVERSION as `libghostty`, `CSI ? u`, DECRQM 2026, a kitty graphics query and OSC 4/10/11.
- Scrollback before the attach is not replayed: Claude Code runs fullscreen
  (`CLAUDE_CODE_NO_FLICKER=1`, kept from the host mode) and scrolls its own history.

## What the widget lacks and what the shell does about it

- **OSC 52, title.** The widget swallows them. The shell scans the bytes before `write()` — a
  stateful scanner, since a sequence can split across reads — and calls
  `renderer.copyToClipboardOSC52` and `setTerminalTitle` (the focused pane's). Both checked live.
- **Bell, OSC 9/99/777 notifications.** OpenTUI offers no way to write them to the terminal; its
  native session takes only the title, the mouse, kitty flags and program status. A raw write to
  stdout between frames is untested: step 2 settles it or asks upstream (question 4).
- **OSC 8** stays lost, as in the host mode.
- **Indexed colours** are resolved to RGB by the widget. The shell asks the person's terminal for
  its palette (`renderer.getPalette`; the spike took 16 entries, the call takes up to 256) and
  default colours at start and writes them into every widget as OSC 4/10/11: checked live,
  palette index 6 drew as the terminal's own colour 6. A theme changed while the shell runs is
  not followed, and the default background goes out as an explicit colour
  (`transparentBackground` untried) — the owner's wish of October 18 met at start only
  (question 3).
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
  the programs (not yet tried).
  Closing restores the pane's focus on the next tick, after the modal unmounts — restored at once,
  the first key after closing was lost.

## The prefix and the room

- **Ctrl+\\** is read in `prependInputHandlers` on raw sequences, before OpenTUI parses them: the
  byte 0x1c, its CSI-u forms with event types and lock bits, its modifyOtherKeys form; a release
  is swallowed. The commands are the host mode's: `n`/`p`, `0`–`9`, `m`, ← →, `z`, the prefix
  again. Checked live in the CSI-u form with ← → and `z`.
- **The room** comes from rewake's own commands, polled once a second: `rewake list --json` for
  each session's activity, model, effort and context, `rewake space --json` for the spaces, which
  gains each open space's socket path so the shell never builds a path itself. A stream comes only
  if polling is felt.

## How rewake starts the shell, and how it ships

- `rewake --shell claude [args]` replaces `--host` (an alias keeps `rewake = ["--shell"]`); once
  main has its host, it starts the main host, then runs the shell as a client of the room, and
  `rewake shell` alone attaches a new shell to a running main (question 2).
- **Shipping.** The person has no Bun. `bun build --compile` with `@opentui/solid/bun-plugin`
  gives one executable with the runtime and OpenTUI's native library inside: built and run in the
  spike, 100 MB, 31 MB compressed. Bun is MIT, but the runtime it embeds carries JavaScriptCore,
  LGPL-2 — a licence the dependency rule does not name (question 5). It goes into each platform
  package beside the Go binary (`scripts/pack.sh`), and rewake finds it next to its own
  executable; `REWAKE_SHELL` names another — the owner's edited copy run from source with `bun`.
  The sources live in `shell/` at the repository's root, outside the Go module.
- **The mod does not change**: it knows nothing of who draws the screen.

## Steps, each checked live

1. **Main and one worker in two panes.** Go: the `b` mode and its preamble, the socket path in
  `rewake space --json`. Shell: main in its own pty, the worker from its space host, the split,
  the bar, the prefix, resize, the shell ending cleanly when its terminal closes — in the spike a
  shell whose terminal was killed once kept running with both sessions for minutes. Check:
  Shift+Enter in both, a worker shown after main opened it, a resize across 120 columns.
2. **Events and colours.** The scanner, the palette seed, the selection rule, bell and
  notifications settled. Check: a copy from each pane lands in the clipboard.
3. **Main under its own host**, if the owner agrees. Check: close the shell, main keeps working,
  `rewake shell` shows it again.
4. **Shipping**: the compiled shell in the npm packages, found by rewake. Check: an install by the
  release gate starts it.
5. **The host mode removed**: `internal/host/hostmode`, the canvas, the view mode, `--host`, and
  their documents to the archive.

Panels that are not terminals (inbox, reports, permission buttons) and the modal come after,
as ordinary Solid components over the same layout.
