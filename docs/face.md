---
description: rewake's own face for a live session — the mirror model in the session's wrapper, its socket, the dev worker, the shell's face with its switch, its own code in face/ with its contract, and hot reload.
---

# rewake's face

A worker's pane shows one of two **faces**: `native`, the harness's own TUI as the pty draws it, or
`rewake`, drawn by rewake's shell from the **mirror**, a harness-free model of the session's
conversation kept in the session's wrapper. A **dev worker** is one raised with the mirror on
(`rewake space open … --dev`); the face draws dev workers on Claude Code and every pi worker, dev
without the flag (the owner's word, October 31, 2026: pi drawn as Claude Code looks).

**The owner's word (October 28, 2026):** a conversation drawn by rewake's face passes to the shell as
data in memory only. The mirror reads no transcript file and writes nothing: prompt, reply and tool
input and output come from live events, from the session's wrapper to the person's shell, never to
disk, a letter, a log, the room or another session. The interface is meant for every session later,
main included, so the model, its socket and the room view's field are the session's, not a
worker's: `LaunchRequest.Mirror` turns them on, and a dev worker is only the first to set it.

## The model — `internal/core/mirror`

One schema, `mirror.schema.json`; `tools/protogen` writes its Go types and the shell's TypeScript
types (`face/src/mirror.gen.ts`), held by the generator's drift test. The model holds
**items** — a `prompt` with its `origin` (person, face, …), a `text` or `thinking` block
(`streaming`, then `settled`), a `tool` call with its name, input, output and end (`running`,
`done`, `error`, `refused`), a `notice`, `error` when it tells a failure or an interrupt — and the turn's state (idle, working, compacting, unknown,
ended). An item the link lost part of is a `gap`, never shown as settled.

A call that ended may carry its **permit**, who let it run or refused it, where the session can
tell: `{by, allowed, reason?}`, `by` one of `rule` (a settings rule decided), `mode` (the permission
mode or the tool's own check, nobody asked), `classifier` (the mode's own decider: auto mode's
classifier on Claude Code), `person` (the person at the session's own dialog), `rewake` (rewake's
permission request, answered by main, by the person through it or by its deadline) and `hook`;
`reason` is a refusal's text as the session's own screen writes it after the decider's name, a link
included, within a short field: the face draws it as given and knows no harness's address. The model also holds the session's
permission **mode** — `default`, `plan`, `edits`, `auto`, `bypass` or `unasked` (never asking,
refusing instead), absent until the session says it. Both are the session's whatever its role: main
drawn by the face later carries them as a worker does. (Shape of October 30, 2026, brief 44.)

Every bound counts serialized bytes: an item is at most 256 KiB, a text past it keeping its head and
its latest tail with a cut mark, a tool's input and output 64 KiB each; the model is at most 1000
items and 4 MiB. Every byte it keeps counts, a block still streaming, a call still running and a
settle still being put together from its parts alike; past either bound the oldest settled items
are dropped and counted first, then the oldest settle being put together, its block left a gap, and
last the oldest items in flight, so a stream that never settles stays within the bounds too.

The adapter hands the model neutral events (`prompt`, `delta`, `settle` in parts, `tool`, `input`,
`result` with its permit, `turn`, `mode`, `lost`, `resync`). A settle replaces what streamed with the whole block once all
its parts arrived in order; a missing part, a `lost` for the id or the turn's end leaves the item a
gap with what streamed. A `resync` marks everything in flight as a gap, leaves the turn unknown until
the next turn event and adds a notice counting what was lost. How Claude Code's mod feeds it, with
its outbox's caps, is [claude.md](claude.md#the-mirror); pi's extension, with the same caps and
its tools named as their Claude Code kin, [research-pi.md](research-pi.md#the-face-of-a-pi-worker).

## The socket

The wrapper of a run with `Mirror` serves the model on `sock/<name>.<epoch>.mirror.sock`
(`registry.MirrorFor`, hashed past the socket path's bound), from before the harness starts to the
run's end; the room view's session row carries the path as `mirror` for main's view while the socket
is there, and the shell builds no path itself. Access is the space socket's: any process of the
user, `sock/` 0700. A harness that does not implement `adapter.MirrorHarness` is refused before
anything is served.

The stream is JSON lines, each at most 512 KiB. On connect: `hello{v, inputs}`,
`begin{items, turn, mode?, since?, took?, at?, stopped?}`, one `item` line per item and `end`, taken under the model's lock with
the subscription, so no change falls between the snapshot and the stream. Then `add`, `append`,
`set`, `turn` (to idle with `stopped: true` when an interrupt ended the turn: the adapter's idle
turn event carries status `error`, pi's `agent_settled` with `aborted`), `mode` and `drop`.

**The turn's times are the model's** (brief 55, October 31, 2026), so a face that connects or reloads
after a turn draws `Done in 1m 9s · 8:43 AM` as one that watched it. The model stamps them with its
own clock when a turn event moves the turn, in Unix milliseconds: `since`, on `begin` and on `turn`
while the turn is `working`, is when it began (a repeated `working` keeps it; a `resync` or a
compaction clears it); `took` (how long it worked, ms) and `at` (when it ended) ride `turn` to
`idle` from `working`, and `begin` while the last turn that worked is the latest thing to have
happened, with `stopped` as on `turn`. A turn that begins forgets them. The face keeps no clock of
its own for a turn: it draws `since`, `took` and `at` as given (wrapper and face share a machine).
Nothing else changes; the schema's `version` stays, one build per room being its guard. A face
sends `{id, kind: "prompt", text}`, or `{id, kind: "interrupt", text: ""}` for the person's Esc, and
is answered `answer{id, ok, detail?}`; hello's `inputs` lists the kinds the session takes (pi's
both, Claude Code's the prompt alone), and any other input is refused. A face whose queue passes 4096 lines or 4 MiB past its snapshot is cut off rather than
given a gap, and reconnects for a fresh snapshot.

## The dev worker

`rewake space open <name> --alias <alias> --dev` records `worker.dev` in the space record (absent
reads false) and refuses, exit 2, a harness that does not offer the mirror. The worker's launch
reads the flag from its record and sets `LaunchRequest.Mirror`; on Claude Code and on pi the hello
answer then says `mirror: true`. Without it no mirror code runs: no events, no socket, no field in the view.

On a harness implementing `adapter.FacedWorkersHarness`, pi alone, every worker is recorded dev,
`--dev` or not; main is not drawn yet, so the flag only chooses which launches set `Mirror`. pi has
no permission mode, so its face shows none.

## The face

The shell starts a face for each worker pane whose session row carries `mirror`, and ends it with
the pane or the socket. A dev worker opens on rewake's face; the prefix's `v` (Ctrl+\ then `v`)
flips the shown pane between `rewake` and `native`, and the strip says `dev rewake` or
`dev native`. The face is a layer over the pane's rectangle: the native widget under it stays
mounted, sized and fed, so its pty is never resized and its TUI runs on, and the switch costs
nothing on either side. It is laid over a box of the widget's own size inside the pane's padded
box: an absolute layer starts at the padding's edge, and over the padded box it left the widget's
last column showing (seen live on 2.11.0). Its body is the conversation; under it the input between
two rules sends a prompt to the session, refused with its reason where the session does not take it;
the row under the rules says the face's last error, or the link while it is not live. It has no
header: the strip says the turn's state, and the input's top rule carries the session's own
permission mode at its left end and its name with its type, then its context, at the right,
`── ⏵⏵ auto mode ──── w-claude (code) · 46k/272k ─`, as Claude Code draws a session's name there
(the look, "Rules"); the strip under a pane drawn by the face leaves the name and the context out,
a native pane's keeps them. The mouse wheel over the face
scrolls the conversation as a native pane scrolls (below, "The scroll"), and PgUp and PgDn a page
less a row; at the bottom the face follows new rows, scrolled up it holds the rows the person put
there while more arrive below (`face/src/scroll.ts`), and back at the bottom, or after a
prompt it sent, it follows again. A drag selects and a click copies a part (below, "Copying"). The
wheel and the mouse never reach the pty. A prompt
waits for its answer only while the answer can come: at most the latest 16, each for 30 s, none past
the link it went by, a reload or the face's end. Under
rewake's face the keys go to that input, never to the pty. Esc drops a selection; with none, while
the turn works, it interrupts it where the session takes an interrupt, whatever the input holds, as
pi's own Esc stops its run: the face sends `interrupt` and the harness stops as on its own Esc, a
refusal said under the input (`not interrupted: …`). Esc while idle, or to a session that takes no
interrupt (a Claude Code dev worker), does nothing. `rewake screen`
takes the window as the person sees it: the shadow window draws the face too.

### The look

The face repeats the look of Claude Code's own terminal, for every harness it draws (the owner's
decision, October 30, 2026: first the same look, customised later), as read from its screen in
2.1.310 and, for the rules a picture does not show, from an older Claude Code source tree (brief
40; the screen wins where they differ). The code is rewake's own: `render.ts` lays out the items,
`tools.ts` a call's head, `results.ts` what is under it, `fold.ts` runs of calls, `markdown.ts` and
`table.ts` a reply, `turn.ts` the row under the conversation.

- **Prompt**: a faint `❯ ` and the text as typed, no markdown, on the band's background across the
  pane, a wrapped or second line hung under the text (the owner's word, October 31, 2026: better
  than native, which wraps back to the edge); over 10 000 characters, its first and last
  2 500 with `… +N lines …` between.
- **Interrupt**: an `error` notice, the session's own words for a stop or a failure, in red behind
  `  ⎿  ` under what it stopped, hung under its text; another notice is faint behind `· `.
- **Letter**: a letter rewake delivered reaches the harness as its `<task-notification>` and shows
  as the notification's summary after a `●` coloured by its status — green completed, red failed,
  amber killed, else the text's colour. The notice's first line follows the dot, each `↳` preview
  after its newline sits on its own row under it, and every wrapped row hangs under its text
  (native wraps all back to the edge; the owner chose this, October 31, 2026).
- **Reply**: `● ` and its markdown hung at two columns: bold, italic, inline code in the theme's
  blue; a heading bold, the first level also italic and underlined, a blank row after it; blank
  rows run together; list items `-` or their number, hanging under the marker; a quote behind a
  faint `▎` in italic; a table in box lines with a rule between every row, the header bold, and
  when it is too wide each column its longest word and the room left shared out by how much its
  widest cell exceeds that, the cells wrapped; a fenced block as written.
- **Thinking**: the block's text, when the session sent any (the summary a provider streams, which
  pi's own screen draws as markdown in its thinking colour, italic, whole — read in
  `assistant-message.js` of the bundled 1.4.0, October 31, 2026, brief 54), as markdown rows faint and
  italic after a faint `∴ ` hung at two columns; more than ten rows fold to ten and `… +N lines`.
  A block with no text is the faint italic `∴ Thinking`, as the terminal draws it. Claude Code's
  mod carries thinking text only where the session gives it; it is drawn the same.
- **Runs**: consecutive Read, Grep, Glob and Bash calls fold into one faint row hung at two columns,
  `Searched for 1 pattern, read 2 files, listed 1 directory, ran 2 shell commands`, the numbers
  bold, a failed call counted too. Bash is read by the first word of each part of its pipeline:
  `grep`, `find`, `rg`… search, `cat`, `head`, `wc`… read, `ls`, `tree`, `du` list, `echo` and its
  like count for nothing, any other word makes it a shell command; reads count files, each once.
  A call a gap lost leaves its mark under the run's line. Text, a prompt, thinking or another call ends a run, so each sits where it came; a notice does not, and shows after it;
  ToolSearch and TodoWrite are not drawn. While a call of the run is running, or the run is the
  last thing a working turn did, it is open: a faint `●`, the running command's description or
  what the run is doing (`Reading 1 file…`), and under `⎿` the path, pattern or command of its
  latest call.
- **Call**: `● Name(argument)`, the name bold, the dot green when done, red on error, amber on a
  gap, faint while running. The names are the terminal's — `Update` (`Create` for an empty old
  string), `Search`, `Fetch`, `Web Search`, `server - tool (MCP)` — and the argument a path (under
  the home directory as `~/…`), `pattern: "x", path: "y"`, a quoted query, a subagent's
  description, a skill's name, else the input as `key: value` pairs; a head is at most three rows.
- **Permit**: under a call auto mode's classifier let run, after its result, a faint `⎿  Allowed by
  auto mode classifier` (not for a subagent's Agent call; a call folded into a run shows none, as
  natively); a call it refused shows a faint `⎿  Denied by auto mode classifier ∙ <reason>` in place of its
  error, the permit's reason as the adapter gave it (on Claude Code the classifier's reason, then
  `see` and the page explaining auto mode). A rule, the mode, the person, rewake and a hook draw no line: Claude Code 2.1.310
  draws none for them (its binary holds no "Allowed by PermissionRequest hook" any more).
- **Result**, under `  ⎿  `: an edit's `Added N lines, removed M lines` and its old lines on dark
  red and new lines on dark green, from the call's input; a write's `Wrote N lines to path` and its
  first ten lines; an error in red, `Error: ` before a text without it, the tool's error tags
  dropped, at most ten rows; any other output its first three rows (four when only one more),
  a line of JSON pretty-printed, then a faint `… +N lines`; an empty one `(No output)` for Bash,
  `(No content)` for the rest.
- **Turn**: under the conversation, rewake's own line, not Claude Code's (the owner's decision,
  October 31, 2026, brief 53): four dots running clockwise round a braille cell `⠏ ⠛ ⠹ ⢸ ⣰ ⣤ ⣆ ⡇` (clockwise by the owner's word, October 31, 2026) that turns a frame every 120 ms in the terminal's
  bright green (the `accent` tone, the person's palette colour 10, so the shell draws it), and
  beside it what the turn does by its phase, in plain words, no verb picked at random:
  `Thinking…` while a thinking block streams, `Writing…` while the reply does, `Running Bash…` while
  a call runs (the call's name as its head is drawn), `Waiting…` while a request is out with
  nothing back, then the faint bracket `(12s · ↓ 340 tokens)`; after the turn, a faint
  `Done in 31s · 7:52 AM` hung at two columns, or `Stopped after 10s · 7:52 AM` when the turn ended
  by an interrupt (the turn line's `stopped`, not a guess from the conversation: a call stopped
  mid-run ends with its error and no notice). The glyph set in use is `orbit4`
  in `face/src/turn.ts`; others stay beside it, unused, for a switch: the one-dot orbit
  `⠈ ⠐ ⠠ ⢀ ⡀ ⠄ ⠂ ⠁` (`orbit`), the two-dot `⠉ ⠘ ⠰ ⢠ ⣀ ⡄ ⠆ ⠃` and three-dot
  `⠇ ⡆ ⣄ ⣠ ⢰ ⠸ ⠙ ⠋` orbits, all clockwise, the arc `◜ ◝ ◞ ◟` and the pulse `· • ● •`. A running call's dot, and an open run's,
  shows and hides every 600 ms. The bracket also holds an arrow and an
  estimate of what streamed back (`face/src/progress.ts`, on the one clock): `↑` while a request
  is out with nothing back yet — the turn just began, or every call of a step ended — and `↓`
  while a reply, a thinking block or a call's input streams or a call runs; the turn's items are
  those that name its turn, so a prompt queued while it works neither ends it nor resets the
  count; the estimate is the characters of the turn's replies, thinking and calls' inputs over
  four, the shown count moving towards it every 50 ms by 3 under a gap of 70, by 15% of the gap
  (8 at least) under 200, by 50 past, at once past 2000, written `622`, `1.2k`. The bracket
  shows once there are tokens or 16 s; what does not fit the row beside the text goes, the tokens
  first, then the time.
- **Rules**: the two around the input in Claude Code's grey for them, not the text's colour. The
  top one carries the session's permission mode at its left, `⏵⏵ auto mode`, `⏸ plan mode`,
  `⏵⏵ accept edits`, `⏵⏵ bypass permissions`, `⏵⏵ don't ask`, in native's colour for it (auto's
  amber, plan's teal, accept edits' violet, the other two red) and nothing for the default mode or
  one not yet said; at its right the name, ` · `, the context as `46k/272k` (the strip's figures,
  `–/272k` before a measure), with at least one cell of rule between. A narrow pane drops the mode
  first, then the context, then cuts the name with `…`, and leaves it out where not a letter fits.
  Title, mode and context are the session's own, never a worker's by assumption: main drawn by the
  face later takes the same rule. The turn's time is the model's
  (see "The socket"), so a face connected mid-turn or reloaded after one says the same as one
  that watched it. The conversation always keeps one blank row above the input's top rule, the
  `Done` line or not, so a reply never sits on the input.

A row breaks at spaces, a word cut only when it is wider than the row; what still passes the
pane's edge — a mark or a gutter on a pane narrower than it — is cut there; and escapes and control
characters of a tool's output are dropped, since the face's widths do not count them. The faint
colour and the default one are the person's; the rest are Claude Code's fixed theme values,
named in `shell/src/theme.ts` by the element each colours, its dark set or, on a terminal whose
background is light, its light one. What the face cannot repeat, for want of a field the mirror
does not carry: paths relative to the session's directory, an edit's line numbers and context rows,
a call's progress, syntax colours of a
code block (a highlighter the face does not have), and ctrl+o's expansion, a key the face does not
have.

## One clock

Everything the face shows moves on one clock, as the terminal program's own animation frame does
(`face/src/ticker.ts`): a change from the mirror or a resize only marks the face to draw,
and the next tick, every 50 ms, lays it out with the glyph, a running dot's blink
and the turn's time all read from that tick's one moment; a tick that draws what was posted last
posts nothing, and the clock stops while nothing changes or moves. A scroll alone is painted at
once (below, "The scroll"). Seen live before it
(October 30, 2026, brief 41, the face beside a native pane on the same task, every paint logged):
a change painted 33 ms after it came and a separate one-second timer painted the time, its phase
the worker's start, while the glyph turned on the wall clock's second and the time on the turn's,
so a delta's paint showed one moved and not the other; and the glyph turned once a second where
the native one turns every 120 ms beside a blinking dot.

## The scroll

A scroll is the person's hand, not a change of the conversation, so it waits for no tick: the
worker moves the view over the frame the last tick committed — the whole conversation's rows and
the turn's under them, with that tick's moment for the blink — and posts it at once. It lays
nothing out and shows nothing the tick did not: a change that came after the tick waits for the
next one, scroll or not (found in review: a notch between two ticks painted a delta early). The
tick lays the whole conversation out again only when it or the width changed, about 2.3 ms at the
model's bound of 1000 items (12 000 rows), where its tail alone took 0.25 ms. A notch moves as many rows as the native pane moves for it (`face/src/wheel.ts`, from
the time the shell's thread heard it): one alone; while notches come within 40 ms of each other,
0.3 more a notch up to six; the first notch against the way the view moved is held and moves
nothing, as a wheel's bounce, and the next one the new way turns it; a notch back the old way within
200 ms marks the wheel's own stream, which ramps faster, up to 15 a notch, and decays with time.
This is Claude Code's fullscreen window curve at its base of one row, as 2.1.310 has it; a
Windows Terminal, an xterm.js terminal or a set `CLAUDE_CODE_SCROLL_SPEED` give its native pane
another base or curve, which the face does not follow.

Measured live before it (October 30, 2026, brief 42; Claude Code 2.1.310, the shell from its
sources under a pty of 200 by 50, one dev worker's 250-row reply, its two faces in turn, SGR wheel
events written to the pty and each paint timed): the face moved 3 rows a notch at any speed and
painted 55 ms after a notch, on the next tick of a clock that had stopped, a fast stream in jumps of
15 to 18 rows every 50 ms; the native pane moved one row a notch alone, 19 for ten notches 10 ms
apart and 67 for twenty 4 ms apart, a first notch back held, and painted 3 ms after a notch, then
at the renderer's own frames, every 16 ms. After it the face moved the same rows for the same
streams (1 a notch alone, 19, 67, a first notch back held) and painted 2 to 4 ms after a notch.

## Copying

The face takes the mouse itself, whichever session it draws: its layer is the topmost under the
pointer, a mouse event bubbles only to its parents and the pane's widget is not one, and every
`<text>` of the face is not `selectable`, so OpenTUI's own selection never starts over it
(`view.tsx`). The shell's thread passes the left button and the bare pointer to the worker
(`pointFace`), which keeps the gesture (`face/src/pointer.ts`) and posts what to copy; the
shell sends it by `renderer.copyToClipboardOSC52` and nowhere else, and says it under the input
for four seconds in the terminal program's words and colour, `sent 437 chars via OSC 52 · check
terminal clipboard settings if paste fails`, `sent the code block (18 chars) …` for a part, or
`not copied: the terminal takes no OSC 52`.

- **A click or a drag.** A press and its release with the pointer on the press's screen cell all
  the while is a click; a drag event to any other cell makes it a selection from the press's cell,
  and its release copies the selection. A click copies only on a face that was focused when it was
  pressed, so the click that focuses a face copies nothing. A press drops the selection there was,
  and so does a key or Esc; a finished selection stays drawn, as natively.
- **A selection copies text, not drawing** (`face/src/select.ts`). A span may be a `gutter`:
  a row's mark (`●`, `⎿`, `❯`), its indent and a hanging indent, a list's continuation indent, a
  quote's `▎`, a diff's sign and the padding of its band, a table's lines and its cells' padding. A
  gutter is neither highlighted nor copied, and a row of drawing alone, a table's rule, adds no
  line. A row broken to fit the pane carries `join`, the text the break swallowed, and is copied
  onto the row above with it, so a wrapped paragraph copies as the line it was. A table's row is
  copied by its cells, each wrapped cell whole, a tab between them. Each line loses its trailing
  blanks. A blank row inside the selection — between paragraphs, list items, items, in a code
  block — is lit with one cell and copied as an empty line, as natively (the owner, October 31,
  2026). The selection is held in the conversation's rows, so it stays on its text while the view
  scrolls or more arrives.
- **A drag past the body's top or bottom** scrolls the view two rows every 50 ms, for at most ten
  seconds, with the selection's end on the edge's row, native's rule and numbers. The pointer
  cannot go above the terminal's first row, so where the face's body starts there, as natively in
  a fullscreen pane, scrolling up while selecting is the wheel's.
- **A click copies the part it is on** (`face/src/parts.ts`): every row names its part, and
  the text is taken from the model when the click comes. A click takes the row it was pressed on,
  so a click off the conversation's rows — the blank above a short one —
  copies nothing, though a drag from there starts at its first row. A reply's row copies the
  reply's markdown source; a code block's, its code alone; a call's head, its input as the head shows it (Bash's
  command, a file's path, a pattern, a URL, a query, a skill) or else the input as given; a folded
  run's line, its calls' inputs one a line; an open run's `⎿` row, its latest call's; an output's
  rows, the call's output; an edit's red rows its old lines and green rows its new ones; a write's
  preview, the file's lines.
- **Nothing lights as the pointer moves** (the owner's word, October 31, 2026: a light over the
  whole reply under the pointer got in the way); the shell passes the face no bare pointer. A
  click that copies a part lights its rows on the band's colour, the prompt's, in place, so
  nothing moves, for one second, with the notice under the input; a press puts it out.

Native's rules read for this (the terminal program's `noSelect` gutters, its soft-wrap join, its
drag-to-scroll and its copy notice) are its own; nothing of its code is copied. Still differs: no
double or triple click selecting a word or a line, no keyboard extension of a selection and no copy
key, and a selection over a table copies its whole rows.

## Hot reload

The model lives in the wrapper and the face only draws it, so a face can be replaced at any moment
and lose nothing: a new one connects and gets the snapshot. The face is a Bun `Worker`
(`face/src/worker.ts`, with `model.ts` and `render.ts`) that connects to the socket itself,
keeps its copy of the model and posts rows of styled spans — a tone, bold, italic, underlined, a
diff's mark, a row on the band — to the shell's thread, which paints them in the theme's colours
(`shell/src/face/host.ts`, `view.tsx`, `shell/src/theme.ts`);
the rows are laid out by `render.ts`, with the modules of "The look", and an item is
laid out again only when what is drawn of it changed. A worker, not a module re-imported in the
shell's thread: a module can neither be unloaded nor stopped when it loops, and its timers and
sockets would stay; a worker is ended whole. The cost is a thread, a socket and a copy of the model
(at most 4 MiB) per face, and rows of spans rather than a widget tree.

### The face's own code

The face is code of its own in rewake's repository, `face/` at its root, with its tests and its
checks (`bun run check`, `bun test` there), and is not published apart: rewake's release builds it
into the shell (the owner, October 31, 2026: a plugin in code of its own, then «а мы можем не
публиковать ее? а просто встроить в rewake?»). It imports no package, only Bun's runtime. The
contract between it and the shell is `face/src/messages.ts`, the shell importing its types and
`contract`, the number of its shape, raised with any change of it; `start` carries the shell's,
and a face of another contract says so under the input and draws nothing. The shell holds every
message a face posts to the contract (`shell/src/face/check.ts`: the kinds, each field's type, the
tones and marks it knows, the sizes of strings, the ranges of numbers, at most 2000 rows of 4096
spans) and stops a face that sends anything else as one that threw; what it keeps is its own copy,
built from the fields it checked, since a structured clone keeps an array's own extra properties
(a list with its own `slice` threw in Solid's `Index`; found in review). So no face makes the
shell's thread throw. The face takes no lease or lock and writes nothing; one build per room is the Go
image's ([rules/state.md](rules/state.md#the-builds-identity)).

Which copy runs, read once from the shell's own environment when it starts:

- `REWAKE_FACE`, the path of a worker script, relative to the directory the shell was started in;
  a compiled shell runs it as well as one run from its sources (a compiled Bun 1.4.0 executable
  started a worker from a `.js` or `.ts` file outside it, a relative import included; probe,
  October 31, 2026);
- else, in the shell run from its sources (`shell/rewake-shell` as `REWAKE_SHELL`, a second build
  beside the installed one, [install.md](install.md); `rewake2`), the checkout's
  `face/src/worker.ts`;
- else, in a compiled shell, the face it holds as a second entrypoint (`shell/build.ts`), named in
  the bundle from the common directory of `face/` and `shell/`; `rewake-shell --face-check`
  starts it and waits for its answer to a ping, which the release gate runs on the installed
  shell, since a name that moved breaks only there.

The shell watches the directory of a copy it runs from a file, recursively, and starts every face
again 150 ms after the last change there; what reloads is the worker's code, while `faces.ts`,
`host.ts`, `check.ts` and `view.tsx` are the shell's own and change only with a new shell. The
compiled shell's own copy is watched by nothing.

A reload never restarts or exits the shell and never writes to, resizes or closes a pane's pty,
main's included: closing the shell ends main and every worker ([shell.md](shell.md)). Each failure
is contained in the shell's thread, and the person can always switch to native with the prefix's
`v`, since the input line is the shell's:

- **fails to load** (a syntax error, a missing import): the worker's `error` event; the face stops,
  its last lines stay with the error under them.
- **throws**: a drawing or a message that throws is caught in the worker and posted as the error,
  the last lines staying; one that escapes ends in the `error` event, as above.
- **breaks the contract**: a message the shell does not take ends the worker as above, its reason
  under the last lines.
- **hangs**: no answer to a ping within 2 s; the shell terminates the worker and shows why.
- **its socket fails**: shown as `lost`, and the worker connects again a second later.

A stopped face starts again at the next change of its sources, or when the person switches to it.
A new face, a reload's or a lost link's, follows the bottom with no selection: the scroll is the
worker's and is not carried across, so a change under `face/src/` while the person reads up
puts the view back at the bottom (seen live, October 31, 2026, brief 49). Tests: `shell/src/face/host.test.ts`, the contract in `check.test.ts` and through the worker in `host.test.ts`, the look in `face/src/render.test.ts` and `view.test.tsx`, the clock
in `ticker.test.ts`, the scroll in `scroll.test.ts`, a notch's rows in `wheel.test.ts` and through
the worker in `host.test.ts`, Esc's paths through the worker in `escape.test.ts`, copying in `select.test.ts` (a selection over gutters, a wrapped
line, a diff and a table; what a click on each part copies; a click told from a drag), through the
worker in `host.test.ts` with the light after a click and none on a move, and the pty under the
face hearing no mouse in `view.test.tsx`, the wheel included over a real pane's widget fed as pi's
normal screen and as a fullscreen program's.

## Checked

The face's live checks, dated, each with its setup and what it found, are
[research-face.md](research-face.md).
