---
description: Restart traps — main's host closing on SIGPIPE and a room switch repeating a launch against a running harness — each with its live failure, cause and remedy.
---

# Traps when restarting

The restart failures of [traps.md](traps.md), by symptom.

## Main's host and its shell

### `rewake restart shell` answered "rewake is closing", and the whole room closed

On a restart of the shell right after a landing, the command printed `the shell was not restarted:
rewake is closing`, and main, every worker and the terminal tab main ran in ended within seconds.
Neither the landing nor the new code was the cause: a checkout moved to new code under the
running shell, `bun install` and `bun run check` in its directory, a restart from the old code to
the new and a switch of the room's build all passed, alone and together. Main's host took
SIGPIPE among the signals of the terminal's end and closed as on a hangup. In Go a process that
subscribes to SIGPIPE gets it for a broken write on *any* descriptor. So when a client of
`host.sock` left with main's bytes unread, the host's next write to it closed the room. That
client was a shell that stopped reading while it gave its screen back and then exited or was
killed, or any stream or door client. A raw-stream client that stops reading while main draws and
then leaves closed a private room every time; so did a restart whose old shell was stalled. No log
of the owner's room survived, which leaves the stalled old shell's release, under the landing's
load and a Claude Code main drawing, the likely client. The host now takes SIGPIPE only so that
the write fails where it is made ([shell.md](shell.md#who-owns-which-pty)). `TestFixtureShellRestart`
has a stream client leave unread while main draws, and failed on the old code.

*November 3, 2026, in the owner's room; reproduced and checked in a private room with the real
shell and the fixture.*

## A switch of the room's build

### After `rewake restart rewake` main's Claude Code ended, and the room with it

The switch committed, then main's wrapper on the new build printed `rewake: the run handed over
could not be adopted: …/sock/<id>.sock is a live socket; another session is using this name` and
exited; its exit hung up main's terminal, and Claude Code, the shell, the room and the writer
ended. The adopted image ran the whole of a launch's plan again against a harness already
running: Claude Code's adapter refuses a messaging socket something listens on, as another
session's, and the one listening was the run's own Claude Code. Every Claude Code run, main or
worker, failed so; pi has no such check, and the live checks of the switch had run pi and the
fixture only. The adapter's plan also wrote the worker profile again, and the host ran the
version gate, which the next build's pinned pi could have refused.

An adoption now plans only what the run is served with (`Harness.Serve`, which a launch
composes and an adoption calls alone, [claude.md](claude.md#what-an-adoption-repeats)), and an adoption that fails anyway no longer
exits under its harness: it holds it, unserved, and a worker's main is told
([rules/state.md](rules/state.md#switching-a-running-room)). `TestClaudeAdopt` in the workflow
suite runs the real adapter against a stand-in listening on the socket, and failed on the old
code with the owner's line.

*November 3, 2026, in the owner's room; checked live in a private room with a Claude Code main,
a Claude Code worker and a pi worker.*
