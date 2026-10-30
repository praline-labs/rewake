---
description: rewake's own face for a live session — the mirror model in the session's wrapper, its socket, the dev worker, the shell's face with its switch and hot reload.
---

# rewake's face

A worker's pane shows one of two **faces**: `native`, the harness's own TUI as the pty draws it, or
`rewake`, drawn by rewake's shell from the **mirror**, a harness-free model of the session's
conversation kept in the session's wrapper. A **dev worker** is one raised with the mirror on
(`rewake space open … --dev`); the first slice draws only dev workers on Claude Code.

**The owner's word (October 28, 2026):** a conversation drawn by rewake's face passes to the shell as
data in memory only. The mirror reads no transcript file and writes nothing: prompt, reply and tool
input and output come from live events, from the session's wrapper to the person's shell, never to
disk, a letter, a log, the room or another session. The interface is meant for every session later,
main included, so the model, its socket and the room view's field are the session's, not a
worker's: `LaunchRequest.Mirror` turns them on, and a dev worker is only the first to set it.

## The model — `internal/core/mirror`

One schema, `mirror.schema.json`; `tools/protogen` writes its Go types and the shell's TypeScript
types (`shell/src/face/mirror.gen.ts`), held by the generator's drift test. The model holds
**items** — a `prompt` with its `origin` (person, face, …), a `text` or `thinking` block
(`streaming`, then `settled`), a `tool` call with its name, input, output and end (`running`,
`done`, `error`, `refused`), a `notice` — and the turn's state (idle, working, compacting, unknown,
ended). An item the link lost part of is a `gap`, never shown as settled.

Every bound counts serialized bytes: an item is at most 256 KiB, a text past it keeping its head and
its latest tail with a cut mark, a tool's input and output 64 KiB each; the model is at most 1000
items and 4 MiB. Every byte it keeps counts, a block still streaming, a call still running and a
settle still being put together from its parts alike; past either bound the oldest settled items
are dropped and counted first, then the oldest settle being put together, its block left a gap, and
last the oldest items in flight, so a stream that never settles stays within the bounds too.

The adapter hands the model neutral events (`prompt`, `delta`, `settle` in parts, `tool`, `input`,
`result`, `turn`, `lost`, `resync`). A settle replaces what streamed with the whole block once all
its parts arrived in order; a missing part, a `lost` for the id or the turn's end leaves the item a
gap with what streamed. A `resync` marks everything in flight as a gap, leaves the turn unknown until
the next turn event and adds a notice counting what was lost. How Claude Code's mod feeds it, with
its outbox's caps, is [claude.md](claude.md#the-mirror).

## The socket

The wrapper of a run with `Mirror` serves the model on `sock/<name>.<epoch>.mirror.sock`
(`registry.MirrorFor`, hashed past the socket path's bound), from before the harness starts to the
run's end; the room view's session row carries the path as `mirror` for main's view while the socket
is there, and the shell builds no path itself. Access is the space socket's: any process of the
user, `sock/` 0700. A harness that does not implement `adapter.MirrorHarness` is refused before
anything is served.

The stream is JSON lines, each at most 512 KiB. On connect: `hello{v, inputs}`, `begin{items}`,
one `item` line per item and `end`, taken under the model's lock with the subscription, so no change
falls between the snapshot and the stream. Then `add`, `append`, `set`, `turn` and `drop`. A face
sends `{id, kind: "prompt", text}` and is answered `answer{id, ok, detail?}`; any other input is
refused. A face whose queue passes 4096 lines or 4 MiB past its snapshot is cut off rather than
given a gap, and reconnects for a fresh snapshot.

## The dev worker

`rewake space open <name> --alias <alias> --dev` records `worker.dev` in the space record (absent
reads false) and refuses, exit 2, a harness that does not offer the mirror. The worker's launch
reads the flag from its record and sets `LaunchRequest.Mirror`; on Claude Code the hello answer then
says `mirror: true`. Without it no mirror code runs: no events, no socket, no field in the view.

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
header: the strip says the turn's state, and the session's name with its type stands at the right
end of the input's top rule, `──── w-claude (code) ─`, as Claude Code draws a session's name there,
cut with `…` on a narrow pane and left out where not a letter of it fits; the strip under a pane
drawn by the face leaves the name out, a native pane's keeps it. The mouse wheel over the face
scrolls the conversation as a native pane scrolls (below, "The scroll"), and PgUp and PgDn a page
less a row; at the bottom the face follows new rows, scrolled up it holds the rows the person put
there while more arrive below (`shell/src/face/scroll.ts`), and back at the bottom, or after a
prompt it sent, it follows again. The wheel never reaches the pty. A prompt
waits for its answer only while the answer can come: at most the latest 16, each for 30 s, none past
the link it went by, a reload or the face's end. Under
rewake's face the keys go to that input, never to the pty, and Esc does nothing. `rewake screen`
takes the window as the person sees it: the shadow window draws the face too.

### The look

The face repeats the look of Claude Code's own terminal, for every harness it draws (the owner's
decision, October 30, 2026: first the same look, customised later), as read from its screen in
2.1.310 and, for the rules a picture does not show, from an older Claude Code source tree (brief
40; the screen wins where they differ). The code is rewake's own: `render.ts` lays out the items,
`tools.ts` a call's head, `results.ts` what is under it, `fold.ts` runs of calls, `markdown.ts` and
`table.ts` a reply, `turn.ts` the row under the conversation.

- **Prompt**: a faint `❯ ` and the text as typed, no markdown, on the band's background across the
  pane, a wrapped or second line back at the left edge; over 10 000 characters, its first and last
  2 500 with `… +N lines …` between.
- **Letter**: a letter rewake delivered reaches the harness as its `<task-notification>` and shows
  as the notification's summary after a `●` coloured by its status — green completed, red failed,
  amber killed, else the text's colour — wrapped back to the edge.
- **Reply**: `● ` and its markdown hung at two columns: bold, italic, inline code in the theme's
  blue; a heading bold, the first level also italic and underlined, a blank row after it; blank
  rows run together; list items `-` or their number, hanging under the marker; a quote behind a
  faint `▎` in italic; a table in box lines with a rule between every row, the header bold, and
  when it is too wide each column its longest word and the room left shared out by how much its
  widest cell exceeds that, the cells wrapped; a fenced block as written.
- **Thinking**: a faint italic `∴ Thinking`; its text shows in the terminal's transcript view only.
- **Runs**: consecutive Read, Grep, Glob and Bash calls fold into one faint row hung at two columns,
  `Searched for 1 pattern, read 2 files, listed 1 directory, ran 2 shell commands`, the numbers
  bold, a failed call counted too. Bash is read by the first word of each part of its pipeline:
  `grep`, `find`, `rg`… search, `cat`, `head`, `wc`… read, `ls`, `tree`, `du` list, `echo` and its
  like count for nothing, any other word makes it a shell command; reads count files, each once.
  A call a gap lost leaves its mark under the run's line. Text, a prompt or another call ends a run; thinking and a notice do not, and show after it;
  ToolSearch and TodoWrite are not drawn. While a call of the run is running, or the run is the
  last thing a working turn did, it is open: a faint `●`, the running command's description or
  what the run is doing (`Reading 1 file…`), and under `⎿` the path, pattern or command of its
  latest call.
- **Call**: `● Name(argument)`, the name bold, the dot green when done, red on error, amber on a
  gap, faint while running. The names are the terminal's — `Update` (`Create` for an empty old
  string), `Search`, `Fetch`, `Web Search`, `server - tool (MCP)` — and the argument a path (under
  the home directory as `~/…`), `pattern: "x", path: "y"`, a quoted query, a subagent's
  description, a skill's name, else the input as `key: value` pairs; a head is at most three rows.
- **Result**, under `  ⎿  `: an edit's `Added N lines, removed M lines` and its old lines on dark
  red and new lines on dark green, from the call's input; a write's `Wrote N lines to path` and its
  first ten lines; an error in red, `Error: ` before a text without it, the tool's error tags
  dropped, at most ten rows; any other output its first three rows (four when only one more),
  a line of JSON pretty-printed, then a faint `… +N lines`; an empty one `(No output)` for Bash,
  `(No content)` for the rest.
- **Turn**: under the conversation, a glyph that turns a frame every 120 ms, through its six frames
  and back, and a verb rewake picks for the turn (`Mulling… (12s)`) in Claude Code's orange, the
  verb pulsing towards a lighter shade and back every two seconds, the time faint; after it, a faint
  `Brewed for 37s · done 7:01 AM` hung at two columns. A running call's dot, and an open run's,
  shows and hides every 600 ms.
- **Rules**: the two around the input in Claude Code's grey for them, not the text's colour. The face times the turn from the changes it
  saw, so a face connected mid-turn or reloaded says `Working…` and no time.

A row breaks at spaces, a word cut only when it is wider than the row; what still passes the
pane's edge — a mark or a gutter on a pane narrower than it — is cut there; and escapes and control
characters of a tool's output are dropped, since the face's widths do not count them. The faint
colour and the default one are the person's; the rest are Claude Code's fixed theme values,
named in `shell/src/theme.ts` by the element each colours, its dark set or, on a terminal whose
background is light, its light one. What the face cannot repeat, for want of a field the mirror
does not carry: paths relative to the session's directory, an edit's line numbers and context rows,
a call's progress, a turn's tokens, syntax colours of a
code block (a highlighter the face does not have), and ctrl+o's expansion, a key the face does not
have.

## One clock

Everything the face shows moves on one clock, as the terminal program's own animation frame does
(`shell/src/face/ticker.ts`): a change from the mirror or a resize only marks the face to draw,
and the next tick, every 50 ms, lays it out with the glyph, the verb's glow, a running dot's blink
and the turn's time all read from that tick's one moment; a tick that draws what was posted last
posts nothing, and the clock stops while nothing changes or moves. A scroll alone is painted at
once (below, "The scroll"). Seen live before it
(October 30, 2026, brief 41, the face beside a native pane on the same task, every paint logged):
a change painted 33 ms after it came and a separate one-second timer painted the time, its phase
the worker's start, while the glyph turned on the wall clock's second and the time on the turn's,
so a delta's paint showed one moved and not the other; and the glyph turned once a second where
the native one turns every 120 ms beside a pulsing verb and a blinking dot.

## The scroll

A scroll is the person's hand, not a change of the conversation, so it waits for no tick: the
worker moves the view over the frame the last tick committed — the whole conversation's rows and
the turn's under them, with that tick's moment for the blink — and posts it at once. It lays
nothing out and shows nothing the tick did not: a change that came after the tick waits for the
next one, scroll or not (found in review: a notch between two ticks painted a delta early). The
tick lays the whole conversation out again only when it or the width changed, about 2.3 ms at the
model's bound of 1000 items (12 000 rows), where its tail alone took 0.25 ms. A notch moves as many rows as the native pane moves for it (`shell/src/face/wheel.ts`, from
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

## Hot reload

The model lives in the wrapper and the face only draws it, so a face can be replaced at any moment
and lose nothing: a new one connects and gets the snapshot. The face is a Bun `Worker`
(`shell/src/face/worker.ts`, with `model.ts` and `render.ts`) that connects to the socket itself,
keeps its copy of the model and posts rows of styled spans — a tone, bold, italic, underlined, a
diff's mark, a row on the band — to the shell's thread, which paints them in the theme's colours
(`host.ts`, `view.tsx`, `shell/src/theme.ts`);
the rows are laid out by `render.ts`, with the modules of "The look", and an item is
laid out again only when what is drawn of it changed. The shell run from its sources (`shell/rewake-shell` as
`REWAKE_SHELL`, a second build beside the installed one, [install.md](install.md)) watches
`shell/src/face/` and starts every face again 150 ms after the last change; what reloads is the
worker's code, while `faces.ts`, `host.ts` and `view.tsx` are the shell's own and change only with a
new shell. A compiled shell holds the worker as a second entrypoint and watches nothing. A worker,
not a module re-imported in the shell's thread: a module can neither be unloaded nor stopped when it
loops, and its timers and sockets would stay; a worker is ended whole. The cost is a thread, a
socket and a copy of the model (at most 4 MiB) per face, and rows of spans rather than a widget tree.

A reload never restarts or exits the shell and never writes to, resizes or closes a pane's pty,
main's included: closing the shell ends main and every worker ([shell.md](shell.md)). Each failure
is contained in the shell's thread, and the person can always switch to native with the prefix's
`v`, since the input line is the shell's:

- **fails to load** (a syntax error, a missing import): the worker's `error` event; the face stops,
  its last lines stay with the error under them.
- **throws**: a drawing or a message that throws is caught in the worker and posted as the error,
  the last lines staying; one that escapes ends in the `error` event, as above.
- **hangs**: no answer to a ping within 2 s; the shell terminates the worker and shows why.
- **its socket fails**: shown as `lost`, and the worker connects again a second later.

A stopped face starts again at the next change of its sources, or when the person switches to it.
Tests: `shell/src/face/host.test.ts`, the look in `render.test.ts` and `view.test.tsx`, the clock
in `ticker.test.ts`, the scroll in `scroll.test.ts`, a notch's rows in `wheel.test.ts` and through
the worker in `host.test.ts`.

## Checked

Live, October 28, 2026 (Claude Code 2.1.310, the shell from its sources under a pty of 200 by 50,
a scratch state directory and project, a dev worker on the cheapest model): a prompt typed in the
face ran as the worker's turn, with its Bash call, the call's output and the reply drawn; the
switch both ways left the worker's pty at 97 by 48; a change to `render.ts` while the shell ran
was drawn by the next face with nothing restarted, main's process, the shell's and the pty
unchanged; `rewake screen` took the face. Two findings fixed then: a layer over a terminal whose
background the shell does not know let the native face show through every blank cell, so every
row is filled and painted with the default background; and a call the engine refused before any
hook (a tool not offered) stayed `running` until its turn's end now makes it a gap.

Live, October 30, 2026 (Claude Code 2.1.310, the shell from its sources under a pty of 200 by 50
that answered the colour queries, a private state directory and project, a dev worker on the
cheapest model): a letter, two Bash calls, a Read, a failing call, a reply with markdown and a
prompt typed in the face drew as above beside main's native pane; a change to `render.ts` while it
ran was drawn by the next face with nothing restarted. Found then and fixed: a styled text given to
a `<text>` as its content drew `[object Object]`, and a span's `attributes` in its style drew no
bold, so a row is spans with `bold` and `italic`; a colour by its index came out in `rewake screen`
as the renderer's own, not the person's, so the theme resolves it from the palette the terminal
said. Still seen: `rewake screen`'s picture draws a `✻` within a longer run two cells wide,
shifting the rest of that row by one and cutting its last character; the frame's cells are right.

Live, October 30, 2026 (brief 40; Claude Code 2.1.310, the shell from its sources under a pty of
200 by 50, a private state directory and project, a dev worker and a native one on the cheapest
model, the same task to both): the letter, the run of a Bash call, two Reads, a Grep the session did not
offer and a failing Bash folded into `Searched for 1 pattern, read 2 files, ran 2 shell commands` and
`Read 1 file` as natively; the MCP call, the edit's counts and its red and green lines, the
reply's heading, list, code block and table drew beside the native pane's; a change to `table.ts`
was drawn by the next face with nothing restarted. Still differs, for want of mirror fields: paths
are absolute or `~/…` where the native face makes them relative, an edit has no line numbers or
context rows; and a code block has no syntax colours, which need a highlighter the face does not
have.

Live, October 30, 2026 (brief 41; Claude Code 2.1.310, the shell from its sources under a pty of
200 by 50, a private state directory and project, a dev worker on the cheapest model beside main's
native pane, the same task to both: a 15-second Bash, then a long reply): every paint of the face
fell on the 50 ms grid, the glyph and the time moving in the same frames as the text, at native's
cadence; three notches of the wheel while a reply streamed held the rows shown until it ended, PgUp
and PgDn moved a page, and the wheel back to the bottom followed the stream again; the name stood on
the input's rule and left the strip; a change to `turn.ts` was drawn by the next face with the shell's
process unchanged. Still differs: the native pane reveals a streaming reply a whole row at a time,
where the face shows a row as it grows; a running Bash's own time and line count (`· 13s`,
`(13s · 14 lines)`) and the turn's tokens are not in the mirror.
