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
items and 4 MiB, the oldest settled items dropped past either and counted.

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
