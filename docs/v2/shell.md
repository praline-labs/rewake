# Design: the rewake shell

The owner's decision of October 19, 2026: the host mode (`internal/host/hostmode`, a compositor
written by hand in Go) gives way to a **shell** — an application in TypeScript on OpenTUI
(`@opentui/core` and `@opentui/solid`, MIT), running inside the person's terminal, with a terminal
pane per program. Solid is the owner's choice of framework, the same day. No web UI. The layout
is the one the owner approved for the host mode: from 120 columns, once a worker exists, main on
the left and the chosen worker on the right, half of the width less a divider column, the
narrower part when the width is odd; below 120 columns, with no worker, or zoomed, the focused
program takes the whole width and the bar switches the window between main and the workers. Each
pane keeps a column of padding on both sides; the program's size is the width less two. The
bar is a button per program, under main's pane while both panes show and across the whole width
while one does; the line of key hints is always the bottom row across the whole width
([below](#the-bar)); a click on a
button or in a pane focuses it. The prefix is Ctrl+\\, which no harness binds. The host mode was
removed on October 19, 2026, deleted rather than archived (the owner's rule: what is no longer needed
goes at once). "Checked live" below means the spike of October 19, 2026: Claude Code
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
- **The closing screen** (the owner's wish, October 19, 2026): from the moment main's end begins
  until the shell exits, the whole screen is one spinner and "closing rewake…" in the middle
  (`shell/src/closing.tsx`), and nothing of the work — no pane, no bar — shows, nor do keys reach a
  program. It is a layer over the work, as the modal will be, and the work underneath is hidden as
  one, so a terminal background the shell does not know never lets a pane show through.
- **What begins it.** First, **main leaving its screen** (decided October 20, 2026, after the
  measurements below): Claude Code runs fullscreen and leaves the alternate screen at once at
  every end of its process, before `session.end`, so the shell starts the layer on the
  `?1049l` (or 47, 1047) the scanner reads in main's stream (`shell/src/scan.ts`), and the frame
  that would show main's left screen shows the layer instead. A leave may be no end, so under a
  layer begun by a leave alone main keeps its focus and keys, and the layer is lifted when main
  comes back to the alternate screen — an external editor's end, 6 ms after the leave, before a
  frame drew it — or when main still runs 5 s after the last sign. Ctrl+Z leaves the screen too
  (below), so a leave within a second after the shell sent main a Ctrl+Z (0x1a, or its
  kitty or modifyOtherKeys form, `isSuspend` in `shell/src/keys.ts`) begins nothing; Claude Code
  2.1.302 leaves 4 ms after the key. Text main writes after a leave is no sign of a pause: an exit
  prints "Resume this session with: claude --resume …" on the screen it went to. Then **main's closing mark** (the owner's idea, October 19,
  2026): main's mod tells its wrapper `session/closing` at `session.end`, for `/exit`, Ctrl+C, Ctrl+D and `/logout` alike ([the record](../research-claude-mod.md#the-end-of-the-process-as-the-mod-hears-it)),
  and the wrapper of a main in the shell writes `ESC ] 6262 ; rewake-closing BEL` on its terminal,
  main's pane (`internal/host/start.go`), which the scanner reads (the widget ignores an OSC it
  does not know); it makes the layer an end, the keys held, where no leave began it, and restarts
  the 5 s, since the session has ended and only its process is left to exit. Until October 20 the mod also told it at
  `/exit`'s `command.run`, and the shell held main's bytes 250 ms after a leave waiting for the
  mark; `/exit` with work running opens Claude Code's dialog "Background work is running" after
  that hook, so the layer hid the dialog for 5 s with the keys going nowhere, and after the dialog
  the mark comes only once the tasks are stopped, past any fixed hold. That hold also drew main's
  pane 256–259 ms late after an external editor (vim) and after Ctrl+Z; since it went, 19–22 ms
  after the editor ends (live, October 21, 2026; nvim takes 180–300 ms of its own after `:wq`
  before it leaves its screen). **Ctrl+Z** suspends and resumes at once: Claude Code 2.1.302 leaves
  the screen, writes "has been suspended", waits for `SIGCONT` and sends `SIGTSTP` to its group,
  which on the shell's pane is orphaned, so the kernel dropped the stop and Claude Code stayed on
  that text, its keys echoed by the pty, until Ctrl+C. Main's wrapper now catches that `SIGTSTP`
  and sends the `SIGCONT` ([launch.md](../launch.md)); Claude Code's screen is back 19–21 ms after
  the key, its prompt taking keys, the normal screen with "has been suspended" showing for one
  frame between. The fallbacks stay: the room's listing reading main's run
  ended for a space the shell shows, main's process ending, and the shell ending itself on
  SIGTERM, SIGINT or SIGQUIT; not when the terminal is lost (a hangup, its input ended, the
  parent gone), with nowhere to draw.
- **When it ends.** Once main has ended, the layer stays until every worker the shell showed reads
  closed in the room's listing (polled every 200 ms, at most 10 s, the space host's own bounds for
  ending a worker), and at least 600 ms. Measured live on October 19, 2026 (Claude Code 2.1.302 on
  a cheap model, one worker, under a stand-in terminal): after `/exit`'s Enter, or the second
  Ctrl+C, Claude Code leaves the alternate screen within 15 ms, and main's process, wrapper and all,
  exits 340–420 ms later; the room's listing reads main's run ended only after that, and the worker
  reads closed 0.9–1.0 s after main's exit (from October 21, 2026 the workers' close starts at the
  closing mark, about half a second before main's exit: the last worker read closed 1.5–1.9 s after
  `/exit`'s Enter, 2.6–3.4 s before). The shell used to exit 30 ms after main, before a frame
  of the layer was drawn, and the worker ended unseen.
- **Timeline with the mark** (live, the same day, main alone on a cheap model in the shell under a
  stand-in terminal that read its screen after every write). Before: `/exit`'s Enter emptied main's
  pane at +39 ms and the spinner came at +633 ms; the second Ctrl+C or Ctrl+D, +22–24 ms and
  +612–643 ms, one run each. After: the spinner at +44–56 ms after `/exit`'s Enter (three runs)
  and +80–132 ms after the second Ctrl+C (three) or Ctrl+D (one), drawn in the frame that took
  main's last screen away, its pane never seen empty.
- **Timeline with the leave** (live, October 20, 2026, Claude Code 2.1.302 on a cheap model with
  one worker, the shell from source under a stand-in terminal, alone and inside herdr 0.9.3; ms
  after the key; each process logged on one clock). The owner's report of an empty main pane
  before the spinner matched the "before" above; one of the two closings they watched was of a
  run started before the mark's build was installed.

  | Closing | Leave | Mark | Spinner, mark alone (before) | Spinner, leave (after) |
  |---|---|---|---|---|
  | `/exit` | +8–28 | +20–125 at `command.run` before, +72 at `session.end` after | +40–173 | +22 |
  | Ctrl+C twice | +3–5 | +63–66 | +75–81 | +29–36 |
  | `/exit` with a task, then Exit | +5 | +123–132 | the dialog hidden 5 s; +136 after Exit | the dialog shown; +28 after Exit |

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
  program hears is the host's answer: DA1, `CSI ? u`, OSC 10/11, XTVERSION as `libghostty`, DECRQM 2026 as known (reset, set inside a frame);
  kitty graphics and OSC 4 go unanswered. A worker's flicker of the cursor (October 19, 2026) was
  the missing XTVERSION answer: Claude Code sends its mode queries, DECRQM 2026 among them, only
  after it hears a terminal's name, so it never asked and sent no synchronized output; the widget
  then drew each piece of a frame, with the cursor hidden between `?25l` and `?25h`. The DECRQM
  answer stays (not tried without it): the emulator draws at once and the bytes pass to the widget, which batches. The spike's
  widget answers more (a kitty graphics query, OSC 4); that is not
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
  its 16 colours and default colours (`askColors` in `shell/src/colors.ts`: OSC 4/10/11 written once,
  replies read through `renderer.subscribeOsc` for up to 2.5 s) at start and writes them into every widget as OSC 4/10/11: checked live,
  palette index 6 drew as the terminal's own colour 6. Main's decision of October 19, 2026: the
  palette and the default colours are seeded at start, and a theme changed while the shell runs
  is not followed; patching OpenTUI's resolution of indexed colours is not on the plan. Without a
  background reply the panes are `transparentBackground`, so the terminal's own shows, not black.
  Without a foreground reply the widget would write its default foreground as white (`38;2;255;255;255`)
  where the program asked for none; the shell seeds it with a stand-in grey no palette names,
  `#c0c1bf`, and after the widget draws turns the cells in that colour into OpenTUI's
  default-foreground intent, which the renderer writes as SGR 39, so the terminal draws them in
  its own colour (`keepDefaultForeground`). A part answer is common inside a multiplexer: herdr
  0.9.3, the owner's, answers OSC 4 from its own palette and OSC 10/11 only when it learned its
  host's colours itself, and on October 19, 2026 it answered neither — the owner's shell got the
  palette alone, `REWAKE_HOST_COLORS` stayed unset, and the panes showed white text on the
  terminal's background, read as "black" (the owner's check, the same day). Under herdr, then,
  an empty `REWAKE_HOST_COLORS` and a transparent background are not a fault.
  `renderer.getPalette` is not used: it gives up after a fixed 300 ms of silence before the first
  reply whatever its timeout, and returns nothing without failing (a slow Windows Terminal over
  WSL lost the colours that way; replies delayed 1 and 2 s are now heard).
- **The hosts are seeded too**, since a host-fed program hears the host's answer, not the
  widget's: the person's default colours reach every host before its program starts —
  `REWAKE_HOST_COLORS=fg;bg` (XParseColor forms) in main's environment, which `space open`
  inherits and the space host reads to seed its grid. The shell takes them from the same
  `askColors` answer as the widgets' seed and starts main with them, so the terminal is asked
  once; a colour the terminal did not give is left empty, the stand-in foreground included. Unseeded, a worker on a light terminal hears a black background while its pane shows
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
  The closing screen is the first such layer, full-size, with every pane blurred under it.

## The prefix and the room

- **Ctrl+\\** is read in `prependInputHandlers` on raw sequences, before OpenTUI parses them: the
  byte 0x1c, its CSI-u forms with event types and lock bits, its modifyOtherKeys form; it waits two
  seconds for its command, and the releases of the prefix and of the key taken are swallowed. The
  commands: `n`/`p` the next or previous tab; `1`–`9` a worker, `0` or `m` main, each focused;
  ← → the focus to main or to the chosen worker; `z` zooms the focused program or ends the zoom;
  the prefix again sends Ctrl+\\ to the focused program; any other key cancels. Checked live in
  the CSI-u form with ← → and `z`.
- **The room** comes from rewake's own commands, polled once a second (`shell/src/room.ts`):
  `rewake space list --json` for the spaces, which gains each open space's socket path so the
  shell never builds a path itself, and its `mainRun` reading `ended` begins the closing screen;
  `rewake list --json` for the strip (below). A stream comes only if polling is felt.

## The bar

The owner's wishes of October 19, 2026. The bar and the hints are the shell's two lines of
navigation. Where they sit is the owner's decision of October 23, 2026: the hints are shared, the
bottom row across the whole width in every layout. The bar, while both panes show (the wide layout),
sits under main's pane, in main's column, the way the strip sits under a worker's pane: main's pane
ends two rows above the bottom, and the worker's column ends one row above it with its strip on the
bar's row, so both columns end together. While one pane shows (the narrow layout, or a zoom), the bar
and the hints take the two bottom lines across the whole width, and the pane the rest.

- **The buttons** (`shell/src/bar.tsx`): ` main `, then each worker by its name, in the order the
  spaces opened, with no number — the prefix's digits still choose by place. The focused one is
  filled in its colour, bold, with near-black or near-white text, whichever reads; the others sit
  on a grey with their colour as the text. Main's colour is apart from the workers': the person's
  palette entry 4 for main and 2 for the workers, 8 for the grey, each with a fallback of its own
  (`shell/src/theme.ts`). In the wide layout, while main has the focus, the worker shown beside it
  is underlined: filled is where the keys go, underlined is shown beside it without them, and
  narrow, where only the focused program shows, no button is underlined. A zoom shows `zoomed` at
  the right end.
- **The hints** (`shell/src/hints.ts`): what the keys do now — where typing goes, the prefix
  commands that apply, and after a Ctrl+\\ the commands themselves until one is taken or the two
  seconds pass. A hint is a row of one table keyed by the context (main focused, a worker focused,
  the prefix waiting), kept by a `when` over the focus, the zoom and the number of workers; a new
  hint or a new context is a row there. "keys go to <name>" comes first, the rest after it in the
  table's order, all from the left; until October 24, 2026 the rest were centred (the owner's wish
  of October 19), given up for one alignment of the shell's rows. A line longer than the width is
  cut at its end.

**One look for the shell's own rows** (October 24, 2026, on the owner's wish that the strip, the
bar and the hints read as one): each is a `ShellRow` (`shell/src/bar.tsx`), and they share these rules.

- **Inset and alignment.** A row keeps the panes' column of padding on both sides, and its content
  starts at the left of what is inside, so the buttons, the hints and the strip begin in the column
  of the programs' own text, in both layouts. Nothing is centred; only `zoomed` keeps the right end.
- **Air.** A row is drawn on a band, the terminal's background moved 6% towards the ink that reads
  on it (`#282828` gives `#343434`), across its whole width, padding included. The band sets the
  shell's rows apart from a program's last line without a row of emptiness: wide, the bar's and
  the strip's row and the hints' row read as one band of two rows; narrow, the strip, the bar and
  the hints as one of three. A terminal that does not say its background gets no band. Compared on
  screen with a row of `▄` in the band's colour above it, which reads as half a row of air but
  takes a row from every program, and with no band at all.
- **Colours and weights.** Words are in the terminal's own foreground; what names a program is bold
  in its colour — the focused button filled, the strip's name, the hints' keys in the focused
  program's colour. Parts are separated by ` · ` in a faint colour, halfway from the foreground
  to the background (palette 8 where the terminal did not say both): OpenTUI 0.6.6 writes no dim
  over the terminal's own foreground, so a dim attribute there never showed.
- **Selection.** A selection over these rows is one colour, a pane's: the panes' widget swaps a
  selected cell's colours, and their text is mostly the default foreground on the default
  background, so the rows select in the terminal's foreground behind its background
  (`selectionBg`/`selectionFg`, the stand-in grey where the terminal said no foreground), not each
  part in its own colour.

## The strip

Main's decisions of October 21, 2026, after the owner's screenshot showed workers with no status
line. Each worker pane's last row, a shell's row ([above](#the-bar)), is one line drawn by the shell
(`shell/src/strip.tsx`), the same for every harness; the worker's program has one row less. Main
gets none: its own status line shows the same.

```
write-claude (code) · working · ⎇ wt-fd7596 · <model> low · 23% 230k/1M · 90% 42m
```

- **What it says**: the session and its type, the activity (`working`, `waiting for <name>`,
  `waiting at a dialog` ([a dialog nobody is asked about](../permission-requests.md#a-dialog-nobody-is-asked-about)),
  `idle`, `, compacting`, ` (stale)` once the run stopped publishing), the branch, the model and
  effort, and the context as filled percent, used and window. A value rewake does not know is left
  out; a window without a measure reads `–/<window>`. The model and effort stay even where a
  worker's own status line shows them: the strip is the one line every harness has. The model reads
  as the harness's display name (`configuredModelName` of the telemetry: pi's registry name), else the
  template's `launch.label`, else the id. The window is the limit the conversation is kept to: Claude
  Code's launch `--autocompact`, which the host applies over the mod's model window, the percent
  recomputed against it. (October 23, 2026.) Last, the prompt cache as `rewake list`'s Cache cell
  has it (`cache` and `cacheLife` of the telemetry): the hit percent of the last request and the time
  the cache has left — `90% 42m`, `90% <1m`, `90% —` once it expired, `90%` where the harness gives no
  expiry; left out where it gives no cache. The time is counted from `expiresAt` on the shell's own clock,
  every five seconds, so it ticks down between polls. (October 23, 2026.)
- **From the left**, inside the pane's padding, like the bar and the hints (it was centred until
  October 24, 2026).
- **Where it comes from**: `rewake list --json` shows telemetry only to a verified main
  (`internal/cli/session_state.go`), and the shell is main's terminal, so it asks as main's run.
  That run's record is the main whose `servicePid` is the shell's own child, main's wrapper; its
  name and epoch go to `REWAKE_SESSION` and `REWAKE_EPOCH` of the poll, and a new epoch is read
  again at once. No Go change: the record already names its pid.
- **The branch** is the space record's `checkout.branch` for a worker in a checkout of its own
  (made by its launch, or entered); otherwise the branch checked out where the session started,
  read from `git` at most every five seconds, a detached head as its short commit.
- **Narrow**: the line fits the pane, whatever the width. Too long, it leaves out the model and
  effort, then the cache, then the branch; what still does not fit is cut with `…`. The name, the activity and the
  context stay.
- **Colours**: the name bold in the workers' colour of the bar; the rest in the terminal's own
  foreground with faint separators, no colour forced (herdr answers no OSC 10/11); the context in the
  palette's green, yellow or red from 50% and 80%; the cache in the palette's cyan while it is warm,
  faint once it expired.
- **How fresh**: a Claude Code worker's mod samples after each main-loop step's response, at a
  turn's end and with each heartbeat ([design-claude.md](design-claude.md)), so the context grows
  through a long turn; pi publishes on its own events. The poll adds up to a second.

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
