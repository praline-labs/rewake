# Stage 4: the terminal a hosted worker runs on

What step S4-4 built, October 13, 2026: `internal/host/terminal`, a program's
pseudo-terminal and the screen it draws, with the emulator of `charmbracelet/x/vt` behind one
small interface. The design is [stage4-space.md](stage4-space.md) — the pieces in its
section 1, the library's choice in [the terminal library](stage4-space.md#the-terminal-library),
keys and the screen in section 6. Nothing calls the package yet but its tests: the space host
of S4-7 starts the worker on it, and S4-8 maps its screen and keys onto `space/screen` and
`space/keys`. It knows no harness and no mod.

## What it offers

| Call | What |
|---|---|
| `Open(size)` | the pair opened with the standard library: `/dev/ptmx` with `O_NOCTTY`, unlocked, the side opened by its number, the size set before anything starts; then the reading begins |
| `Prepare(cmd)` | the program's stdio on the side, a session of its own with the side as its controlling terminal (`Setsid`, `Setctty`), and its environment through `Environ`; `Side` gives the file to a caller that starts the program otherwise |
| `ReleaseSide()` | closes the parent's copy of the side once the program holds it, so the program's end is seen as the end of the output |
| `Environ(env)` | `TERM=xterm-256color` and `COLORTERM=truecolor` in place of the caller's, which describe the person's terminal and not this one; `LC_CTYPE=C.UTF-8` when the effective locale is not UTF-8 and `LC_ALL` is unset; a set `LC_ALL` is the person's choice and stays |
| `Keys(p)` | one `write(2)` of `p`, whole, under the mutex every write to the program takes, within a two-second write deadline |
| `Resize(size)` | the grid sized first and then the pseudo-terminal, under the lock that feeds the grid; every row published again |
| `Screen(ctx, after)` | a long poll: the rows changed since the sequence `after`, at once when there are any, when `after` is 0 or a sequence never given, or when the output is over; otherwise when the screen changes or `ctx` ends, with the same sequence and no rows |
| `Done()` | closed when the output is over: the program's last process closed the side, or the terminal was closed |
| `Clipboard()`, `Stats()` | the latest OSC 52 text and how many arrived; the answers and clipboard requests dropped rather than waited on |
| `Close()` | hangs up — the master closed, so the program's session gets SIGHUP — then writes the rest of the output, stops the reply pipe and joins every goroutine; a second call returns the first's answer |

A `Screen` carries its sequence, its size, the changed rows, the cursor (row, column,
visible), the modes a key encoder needs — cursor keys, keypad, bracketed paste, mouse
tracking and its SGR encoding, focus reports, the alternate screen — and whether the output
ended. A row is a list of runs, each its text, the columns it covers, its foreground and
background (default, indexed or direct) and its attributes (bold, dim, italic, underline,
inverse, strikethrough); a row's runs always cover its width. The sequence moves only when
something changed: a quiet screen costs a reader one wait and no rows.

## How it is driven

One goroutine reads the master; a second writes each read into the emulator whole, under the
terminal's lock, and only then wakes the readers, since the emulator's callbacks run inside
the write and a look between them would see half a frame. A third takes the emulator's
answers to the program's queries off the library's pipe at once into a queue of 64 and counts
what does not fit; a fourth writes them to the program. So a program that queries and never
reads its input — a full-screen program in raw mode, flooding `ESC [ c` — loses answers and
never stops the emulator in the middle of a write. A look walks the cells into runs and
compares each row with the one published; only rows that differ, the cursor or the modes move
the sequence.

## What the library does not do

Read in the source of `charmbracelet/x/vt` v0.1.0 and held by the package's tests, October 13,
2026:

- **It ends a glyph cluster at the end of every write.** A mark, a variation selector or a
  joiner that arrives in the next read becomes a cell of its own. The feeder holds back the
  last cluster of a read, at most 64 bytes, and writes it with the next read or alone after
  5 ms. A test splits a combining mark, VS16, a ZWJ sequence, a keycap, a conjunct and a flag
  at every byte; another fails when the library starts joining across writes, so the hold can
  go. The cost: the last character of a burst reaches the screen up to 5 ms late.
- **Its reply pipe is unbounded and blocking.** A write that answers a query waits for the
  pipe's reader; the drain goroutine above is that reader.
- **Closing the emulator races its reply reader** (a field written by `Close` and read by
  `Read` without a lock). The package closes the pipe's write end, which the library hands out
  as an `io.Closer`, and never the emulator.
- **Its damage tracking is never cleared.** `Touched` reports every row ever written, so the
  package diffs the rows itself.
- **It keeps 10 000 lines of scrollback**, shifting the whole buffer per scrolled line. The
  screen shown is the visible one; scrollback is set to one line, the least it takes.
- **It has no OSC 52 handler.** The package registers one: base64 up to 1 MiB is taken, a
  read request (`?`), an empty, oversized or unreadable one is refused and counted, and
  nothing is answered.
- **Its modes are not one value.** They are kept from its enable and disable callbacks, which
  a full reset runs too.
- **What a run cannot carry is dropped**: concealed text is drawn as blanks, blink, the
  underline's style and color, and hyperlinks are left out, and a wide glyph cut at the right
  edge is drawn as blanks.

What it does as wanted, held by the canary (`testdata/canary.golden`): dim text, an emoji
with VS16 and wide glyphs in their columns, a title kept off the grid, a scroll region
scrolled, the alternate screen entered and left. A library update that changes any of them
fails that one test; its golden is rewritten with `-args -canary.update` and reviewed like
code.

## What it brings in

Measured with `go list -deps` on October 13, 2026. `go list -deps ./cmd/...` is the same 150
packages before and after: nothing imports the package yet, so the binary does not change at
this step. Linked into `cmd/rewake` (a blank import through `-overlay`, not committed), it
makes 185 packages: 20 from other modules —

- `charmbracelet/x/vt`, `x/ansi`, `x/ansi/parser`, `x/ansi/kitty`, `ultraviolet`,
  `ultraviolet/screen`, `colorprofile`, `x/term`, `x/termios`, `x/windows`, `x/exp/ordered`;
- `clipperhouse/uax29/v2/graphemes`, `clipperhouse/displaywidth`, `rivo/uniseg`,
  `mattn/go-runewidth`, `lucasb-eyer/go-colorful`, `muesli/cancelreader`, `xo/terminfo`;
- `golang.org/x/sys/unix`, `golang.org/x/sync/errgroup` —

and 15 more of the standard library (`compress/flate`, `compress/zlib`,
`database/sql/driver`, `hash/adler32`, `hash/crc32`, `hash/maphash`, `image`, `image/color`,
`image/png`, `io/ioutil`, `log`, `log/internal`, `net/url`, `os/user`). Built with `-trimpath`,
the binary grows from 9 308 330 to 9 712 494 bytes, by 4.3 %, with only the package's
initialization linked; the emulator in use will add more. `go.mod` gains three direct
requirements (`x/vt`, `x/ansi`, `ultraviolet`) and fourteen indirect ones. The layout test
holds the exception to its place: `github.com/charmbracelet/` is imported by
`internal/host/terminal` alone ([the import rule](../rules/layout.md#the-import-rule)).

## Left to the steps after

- `ScreenAnswer` in the mod's protocol carries no modes; the key surface needs cursor keys,
  bracketed paste and the mouse to encode keys, so S4-8 adds them to the schema.
- The default foreground and background are the library's, white on black, and its answers to
  OSC 10 and 11 say so; whether to answer with the person's colors stays with S4-8.
- Synchronized output (mode 2026) is not honored: a frame is published as it arrives.
- Whether the worker is alive and how it ended belongs to the space host (S4-7), which waits
  for the process; the terminal only says that the output is over.

## Tests

All in `internal/host/terminal`, run with `-race`: a program on a real pseudo-terminal (the
test binary as a helper) drawing styled runs, echoing keys, resized through SIGWINCH, hung up
by `Close`, flooding queries it never reads, and seeing the emulator's terminal; clusters split
at every byte; the cells-to-runs table; the published diff; the modes; the clipboard's cap; each
post of keys one write, concurrent posts never interleaved; resizes, reads and keys at once;
the canary.
