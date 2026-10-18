# Design: the workers' panel in main's Claude Code

The owner's decision of October 18, 2026 ([project.md](../project.md#what-the-tool-does), "Main
starts its workers"): main's Claude Code lists the workers of its spaces and shows the chosen
one's screen, live, taking keys. Built on Claude Code 2.1.302; the mod API is the `mods` skill's.
`rewake attach` stays as it is.

## The path of a screen and a key

```
worker ⇄ pty ⇄ space host ──(space socket, "view")──▶ main's wrapper (modhost) ──(mod socket)──▶ mod: pane "Workers"
                 emulator                               one view per shown space               Raster + blit, Client keys
```

**The space host** feeds every read of the pty master to the emulator restored from
`eb52049^` (`emulator.go`, `vt.go`, `screen.go`, `hold.go`; `charmbracelet/x/vt` v0.1.0 pinned,
confined to `internal/host/terminal` by depguard again, under the owner's exception of October
13). The pty and the relay's pump stay as they are; the emulator is a reader beside the attach
client, never in its way. Its answers to the program's queries (cursor report, device
attributes) are written to the program only while no attach client is connected — an attached
terminal answers them itself, and two answers would reach the program twice.

**The space socket** takes a first byte from the client naming the mode: `a` is the attach of
today, unchanged after it; `v` is a view. A view is accepted at once (several may be open, none
makes attach busy), and the host writes it the whole screen as one frame — kind, length of four
bytes, JSON of `adapter.Screen` — at its open, then 30 ms after each change, so at most one per
30 ms: taken at the change itself, a frame of Claude Code caught its cursor hidden while it drew,
and the panel's cursor blinked with every frame a working worker drew. The
client writes the attach's `i` and `r` frames. A size from either kind of client is the
worker's size until the next one (the person attached and the panel take turns; the last wins).

**The host composes itself into main's run**, as it does a `TurnHandler`: a new optional field
of the plan, `Spaces adapter.SpacePanel` (`ShowSpaces(adapter.SpaceView)`), is handed, for a run
whose role opens spaces, the host's `SpaceView`: `Workers()` — this main run's spaces not over,
each with its record (name, harness, state) joined with the worker session's snapshot as
`rewake list` reads it (model, effort, context filled, activity) — and `View(space)`, a view
connection as above. The values (`Screen`, `Run`, `Color`, `Worker`) live in `internal/adapter`;
the import rule holds: `modhost` never sees the host.

**modhost** keeps at most one view per space, opened on the first request naming it and closed
after 30 s with none; it serves four new endpoints, only to a run that hosts spaces:

| Path | Asks | Answers |
|---|---|---|
| `workers/list` | — | the workers, as `Workers()` gives them |
| `workers/screen` | the space, the last sequence the mod has | within 15 s, the next screen as a Raster's `cells` (base64 of `[code point, fg, bg]`), its columns and rows, whether the worker lives |
| `workers/keys` | the space, the bytes as a string | done once written to the view |
| `workers/resize` | the space, columns and rows | done |

The Raster encoding is the adapter's, since a Raster is Claude Code's: a palette index becomes
xterm's RGB, the default stays `0x01000000`, inverse swaps the two (a default turned into the
other's place takes a fixed grey), a space of any kind becomes the plain one, a wide glyph, a
non-BMP, private-use or control character becomes `?` (a wide glyph's second cell a blank), and
the visible cursor is drawn inverse. Which glyph is wide is the emulator's word, not a table of
blocks: the emulator gives a wide glyph a run of its own, two columns for one glyph, and a run
whose columns do not match its glyphs draws every character outside ASCII as `?`. A block table
let wide symbols of the BMP such as U+2705 through as one cell, and Claude Code refused the whole
screen over one (the panel's review, October 18, 2026). Bold, dim, italic and
underline are lost: a Raster has no attributes.

**The mod.** Only `/workers` opens the pane `rewake-workers`, with the focus and
`closeOnEscape`, and answers how to use it; the mod no longer opens it by itself when the first
worker appears, since the person got stuck in a pane they had not asked for (the owner, October
18, 2026, until the pane is quiet and closes reliably). The pane draws one line per worker —
`1 name  harness  model  effort  ctx 34%  working`, a `Button` with its digit as hotkey selects
it — then the chosen worker's screen as one `Raster`, and the key surface: a `Client` drawn over
the screen in an absolutely placed `Box`, as tall as the screen and the key line under it, which
draws only that last line, so the `Raster` shows through. The pane body's size (`bodyColumns`, `bodyRows` less
the list and the key line) is sent as `workers/resize` on a selection and when it changes. While
the pane is open with a worker chosen, a detached loop (a `$.clock.after` chain, outside every
hook's budget) long-polls `workers/screen` and blits the cells; a screen of another size
redraws the pane instead (`$.ui.invalidate`), and a blit refused because the pane is not drawn
waits a second. The list is polled every 2 s while the pane is open and every 5 s while it is
closed, so `/workers` opens it on a current list. The first worker is chosen until the person picks
another; one whose output ended, or whose space left the list, is said to have ended in place of
its last screen, and takes no keys.

**Keys.** A click anywhere on the screen or the key line gives the key surface the keyboard: a
`Raster` takes no click, and Claude Code made a click on it a text selection, the keys staying
the prompt's. Nothing in 2.1.302 gives a `Client` the keyboard without a click — `autoFocus` and
`$.ui.focus` reach only a `Button`, `Input` or `Select` — so after `/workers` typing goes to the
prompt until the person clicks the screen. The key surface turns each key into
the bytes a terminal sends — P15b's encoder, its table and its prefix: Alt+a then `c` sends
Ctrl+C, `e` or `[` Escape, `x` Ctrl+X, Alt+a Alt+a itself, any other key cancels. The line reads
`keys → name · click the screen to type · Esc: to prompt · Alt+a: c=^C e=Esc x=^X` and, while a
prefix waits, the prefix's hint.

**Closing.** Escape closes the pane while the pane holds the keyboard, as right after
`/workers`. While the key surface has it, Escape hands it to the prompt, and a second Escape at an
idle, empty prompt closes the pane; at a busy prompt that Escape is main's turn's. Ctrl+X then X
closes only while the pane itself holds the keyboard (after `/workers`, or Ctrl+X Tab): with the
key surface holding it, neither key reaches the pane or the surface. The pane's ✕ closes it
from anywhere. A `Client` delivers one post a frame and a later post replaces one not yet delivered,
so each post carries every key not yet acknowledged, numbered; the hooks module types each
number once, in order through one queue to `workers/keys`, and answers the last number taken as
the key line's props, which drops what was acknowledged. What cannot reach the worker stays
the outer Claude Code's: Escape returns the focus, paste goes to main's prompt, and the Ctrl
keys beyond A and B were not seen to reach a Client (P15b) — the prefix carries the three that
matter.

## Not in this step

Raising a worker from the pane (main's shell has `rewake space open`); changing a running
worker's model or effort; paste; mouse; the VS Code extension and Desktop, where a Raster does
not draw (the pane says so in text and lists the workers without a screen).

## Tests

The protocol's drift test (`tools/protogen`); the space host's view — a program's output reaches
a view as a screen, a key from a view reaches the program, answers to queries are written only
while unattached; modhost's relay over a fake `SpaceView` — list, screen in Raster cells, keys,
a view closed when idle; the cell encoding as pure logic; the mod's pane in `claude plugin test`
— the list drawn, `/workers` opening the pane with the focus and `closeOnEscape` and its answer
naming the worker, a selection, a blit after a screen poll, keys and the prefix posted. No
workflow scenario: the path has no stand-in harness that draws a pane.

## Seen live

October 18, 2026, Claude Code 2.1.302, main in a terminal of 200 by 50, a Claude Code worker
raised with `rewake space open api --harness claude -- --model haiku`: the pane opened by itself
docked beside the transcript, listed `1: ▸ api  claude  claude-haiku-5-5  idle`, and drew the
worker's own screen live; after a click on the key line, typed text showed in the worker's
prompt, Alt+a `c` cleared it, `/exit` typed there ended the worker, and the pane then read
`api ended`. A no-break space drawn as `?` before the fix showed the mapping above was needed,
and a letter lost from text typed 30 ms apart, before the posts carried every unacknowledged key,
showed the replaced post; after it, text typed 10 ms apart arrived whole. `rewake attach` beside
the pane: the attached terminal showed the worker and typed into it, the pane followed, `Ctrl-\`
detached with exit 0.

Later the same day the owner found the cursor cell blinking, Escape not closing the pane,
Ctrl+X X not always closing it, a click on the screen not giving it the keys, and `/workers`
answering an empty line. After the fixes above, driven through a pseudo-terminal of 200 by 50 with
a working haiku worker: no pane until `/workers`; a click on the screen typed into the worker;
frames with the cursor shown only, about 9 a second while it worked, none while idle; each way of
closing as written under "Closing" ([the probe](../research-claude-mod.md#the-panes-focus-closing-and-frames-later-the-same-day-21302)).
