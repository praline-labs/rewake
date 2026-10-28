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
nothing on either side. Its header says the turn's state, or the link while it is not live, and
the face's last error; its body is the conversation's tail, PgUp and PgDn scroll it; its input line
sends a prompt to the session, refused with its reason where the session does not take it. A prompt
waits for its answer only while the answer can come: at most the latest 16, each for 30 s, none past
the link it went by, a reload or the face's end. Under
rewake's face the keys go to that input, never to the pty, and Esc does nothing. `rewake screen`
takes the window as the person sees it: the shadow window draws the face too.

## Hot reload

The model lives in the wrapper and the face only draws it, so a face can be replaced at any moment
and lose nothing: a new one connects and gets the snapshot. The face is a Bun `Worker`
(`shell/src/face/worker.ts`, with `model.ts` and `render.ts`) that connects to the socket itself,
keeps its copy of the model and posts plain lines with their tones to the shell's thread, which
paints them (`host.ts`, `view.tsx`). The shell run from its sources (`shell/rewake-shell` as
`REWAKE_SHELL`, a second build beside the installed one, [install.md](install.md)) watches
`shell/src/face/` and starts every face again 150 ms after the last change; what reloads is the
worker's code, while `faces.ts`, `host.ts` and `view.tsx` are the shell's own and change only with a
new shell. A compiled shell holds the worker as a second entrypoint and watches nothing. A worker,
not a module re-imported in the shell's thread: a module can neither be unloaded nor stopped when it
loops, and its timers and sockets would stay; a worker is ended whole. The cost is a thread, a
socket and a copy of the model (at most 4 MiB) per face, and lines rather than a widget tree.

A reload never restarts or exits the shell and never writes to, resizes or closes a pane's pty,
main's included: closing the shell ends main and every worker ([shell.md](shell.md)). Each failure
is contained in the shell's thread, and the person can always switch to native with the prefix's
`v`, since the input line is the shell's:

- **fails to load** (a syntax error, a missing import): the worker's `error` event; the face stops,
  its last lines stay with the error in its header.
- **throws**: a drawing or a message that throws is caught in the worker and posted as the error,
  the last lines staying; one that escapes ends in the `error` event, as above.
- **hangs**: no answer to a ping within 2 s; the shell terminates the worker and shows why.
- **its socket fails**: shown as `lost`, and the worker connects again a second later.

A stopped face starts again at the next change of its sources, or when the person switches to it.
Tests: `shell/src/face/host.test.ts`.

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
