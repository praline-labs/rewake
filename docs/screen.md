---
description: rewake screen — a picture of a session's window as the person sees it in the shell: who draws it, the request socket, the shadow window, the snapshot of a space host, the PNG and what it refuses to write.
---

# rewake screen

`rewake screen <name> [--png <path>] [--text]` gives any session of the room a picture of a
session's window as the person sees it in [the shell](shell.md): the pane, its strip under it and
the bar. The owner asked for it on October 28, 2026, for a coming dev mode (a worker drawn by a
new interface of rewake's and checked by agents), and for a reviewer or main to see what a worker's
screen shows, a dialog it is stuck in, how the shell looks. It is a view
([rules/state.md](rules/state.md#what-is-a-writer)): it takes no lease and writes nothing of the
room; the one file it writes is the picture the caller names. `<name>` is a worker (its space
name or its address from `rewake list`) or main (`main`, or main's own address); nothing of another
room is reachable.

## Who draws

The shell draws, since only it knows the strip's and the bar's drawing. At start it makes, beside
the person's window, a **shadow window** (`shell/src/window.tsx` is the one drawing of both,
`shell/src/main.tsx` the shadow): an OpenTUI tree on `testRender`, never put on the person's
screen, with a second widget beside each pane's, written the same bytes (main's from the start, a
worker's from its first attach). A request sets the shadow window's view — the named session in the
focus and, for a worker, beside main (alone when the shell is narrow or zoomed), the live size and
zoom — renders it once, and reads the cells with `captureSpans()`. The person's screen does not
flicker or switch, and main's window is main's even while the shell shows a worker. The shadow
widgets follow the live layout (an effect on the workers, the size, the chosen worker and the
zoom), so each has the size its program draws for; they have no `onData` and no resize hook, so
nothing of them reaches a program, and nothing of the window is focused, titled or copied. Their
cost is a second parse of every byte a program writes. Not drawn: the briefs (a request for a
session never shows them), selections, a pending prefix.

The hints say the shadow window's own focus, the named session's keys, not the person's.

## The request

The shell listens on `sock/shell.sock` of the room. The launcher that execs the shell passes the
path in `REWAKE_SHELL_SOCK` (`state.SocketPath(dir, "shell", "")`, taken out of main's
environment); the room's folder is made by the main the shell starts, so the shell tries each
time the room is polled until it can listen, and removes a stale file first (one main per room).
One request a connection: a line `{"pane": "main"}` or `{"pane": "<space id>"}`, answered by one
line, a frame or `{"error": "…"}`, and the end. A request over 4 KiB is dropped, the draw has 5 s,
the CLI waits 2 s to connect and 10 s for the answer and reads at most 4 MiB. Requests are served
one at a time. The shell is the writer of this socket file, a TypeScript process outside
`infra/state`; the room's `sock` folder is 0700.

The frame is `internal/core/screen`'s model: `cols`, `rows`, default `fg` and `bg` where the
terminal said them, the cursor — present only when the pane shows it —, and per line runs
`{t, w, fg, bg, a}`: text, columns (a wide glyph counts two), `#rrggbb` where the cell has a color
of its own (the terminal's default and a transparent cell stay empty), and the attributes as bits
1 bold, 2 dim, 4 italic, 8 underline, 16 inverse, 32 strikethrough. OpenTUI's own capture omits
the cursor's visibility and has its own attribute bits: `shell/src/screen.ts` takes the first
from the pane's screen and maps the second. A frame is at most 400 by 200 cells, checked by both
ends (`Frame.Check`) and by the shell before it writes. The answer is written as the socket takes
it (a large frame is a partial write, the rest goes on `drain`), and a connection that sends no
complete request in 5 s is dropped.

**The person's colors.** The frame carries what the terminal reported at start (`askColors`,
[shell.md](shell.md#what-the-widget-lacks-and-what-the-shell-does-about-it)): `fg` and `bg`, the default foreground and
background, and `palette`, its sixteen colors with an empty entry where it gave none. Cells reach
the frame with their colors resolved (the widget resolves an indexed color through the seeded
palette), a cell in the default or a transparent one stays empty, and the picture draws those in
`fg` and `bg`. A color the terminal did not report is drawn in a stand-in (foreground `#d8d8d8`,
background `#282828`, xterm's palette) and the answer says so: `standIn` in `--json`, a line of the
text form, naming `foreground`, `background` or `palette`. herdr 0.9.3 reports OSC 4 and often not
OSC 10/11, so a picture taken there names the two. The host path seeds `fg` and `bg` from
`REWAKE_HOST_COLORS` as its emulator does; the palette is not passed to a host, so it always names
`palette`.

## No shell: the space host

When nothing listens at `sock/shell.sock` (main runs without a terminal) a worker's pane alone
comes from its space host: mode `s` of the space socket ([space.md](space.md#the-stream-to-the-shell)),
which answers one JSON line of the emulator's screen — styled rows, the cursor when visible, the
default colors empty, indexed colors as xterm's palette — and closes. It takes no frame and sets
no size, it reads under the grid's lock and writes after releasing it (`Grid.Snapshot`). The answer
says `source: host`, and the text line says the pane is alone. Main has no host: refused, exit 1. A
shell that answers with an error is not asked around.

## The picture

`Frame.PNG` draws with the Go standard library only: GNU Unifont 16.0.01's Basic Multilingual
Plane (`internal/core/screen/unifont-bmp.hex.gz`, about 0.9 MB), 8 by 16 pixels a cell, 16 by 16
for a wide glyph, box drawing included; used under the SIL Open Font License 1.1, copyright the
Unifont authors, accepted by main on October 28, 2026. No system font is assumed. Color,
bold (drawn twice a pixel apart), dim, inverse, underline, strikethrough and the cursor (an
inverted cell) are drawn; italic is not; a glyph the font lacks is a hollow box; a mark that
combines is drawn over the cell before it. A picture is at most 3200 by 3200 pixels.

Unifont is a bitmap font, where the person sees an antialiased one. Drawing with an embedded
monospace TTF was evaluated on October 28, 2026 and not done: `golang.org/x/image/font/opentype`
(and `sfnt` under it) imports `golang.org/x/text/encoding/charmap`, so it needs two modules the
project does not have, x/image and x/text, and the dependency rule allows no further module
([rules/layout.md](rules/layout.md)). The cell, 8 by 16, has the proportions of the person's
(about 16 by 33 in a 2000 px shot), at half the size.

`--png <path>` is written by `screen.WritePNG` and refuses (exit 2): a path inside rewake's state
(the base directory with every room, and `~/.config/rewake`), a path through a symbolic link (its
folder must be its real path), a folder that is missing, and an existing file that is not a regular
one; a regular file is replaced, by a temporary file in the same folder and a rename. Those writes
are in the table of `internal/infra/state/bypass_table_test.go` with that reason.

## Checked

Live, October 28, 2026 (Claude Code 2.1.310, the shell from source in a herdr tab, a scratch room):
a worker's window and main's, with a worker shown beside main, taken from outside the room's
sessions, the person's tab unchanged; and, with the socket moved away, the worker's pane alone
from its host and main refused. Compared with the owner's shots of the live shell the same day, a picture differs in: the font
(bitmap, not antialiased, half the size); italic, which is not drawn; bold, drawn by a second
pixel, not a bold face; the colors, which equal the person's where the terminal reported them
(the stand-in background `#282828` was read off the shots). A pi worker's picture has not been taken; whether pi can open an
image is not known (nothing in [research-pi.md](research-pi.md)), so the text form serves it.
