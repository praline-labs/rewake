---
description: The open work and its order — rewake's face after its first slice — and the ideas not scheduled.
---

# Roadmap

The open work and its order. A step closes when it works live and its acceptance passes, not
when its code exists; a closed step leaves this file, its commit's subject is its record, and
what it taught goes into the living documents.

## Open, in order

Updating without losing the sessions. The owner decided this on November 1, 2026, on the design https://claude.ai/artifact/DK7YF2q9kQsbLjez2JMUgt. Main's session lives only as long as the shell, because the shell holds main's pty.

1. **A restartable shell.** A Go host in the person's pane holds main's pty, its emulator and a stream with a preamble, as a space host does for a worker. The shell becomes its replaceable child.
   - A restart on request (`rewake restart shell` or a key) leaves main and the workers running, and so does one restart after a crash.
   - Closing the window, losing the terminal or a stop signal still ends everything.
   - `room --json`'s proof of main goes through the host.
2. **Updating from inside rewake.** A manifest of the parts inside one release, giving each part's digest, the contracts it needs and its restart class. Any one part can be restarted on its own with `rewake restart <part>`, for a quick reload; the manifest lists the parts. The owner's words: «рестарт "кусочка" будет выборочным, как раз для быстрой перезагрузки». The shell shows a banner of an update, with what it will restart. A changelog by part, its "what restarts" computed from the manifests.
3. **A Go update without a room restart.** Each long-lived process `exec`s the new build, keeping its pid, its children and its fds, and hands over the state that lives only in memory. It starts with a probe on one idle wrapper of the same build.
4. **A door for plugins.** The parts' contracts and the manifest admit a part that is not rewake's own. A project's main can attach its own part, or a ready one, to rewake: a pane or a strip segment in the shell, a command, a tool for the sessions, a listener of the room's events. Steps 1 and 2 keep their seams generic and versioned, so that a plugin is one more client.

rewake's face ([face.md](../face.md)), after its first slice — a dev worker on Claude Code and every
pi worker, read with a prompt back, the switch and hot reload:

1. Asks in the face, answered there through the run's ask broker.
2. Abort from the face on a Claude Code dev worker (pi's Esc is in), compaction notices,
   `dev = true` in a template with `--no-dev`, `rewake list`'s Type cell `code, dev`,
   `rewake screen --face native|rewake`.
3. A workflow scenario of a dev worker on the fixture harness, once the fixture mirrors.
4. Later, each its own decision: subagents in the face, main as a drawn session.

## Not scheduled

- Two live checks left from the owner's looks and logins, which the owner accepted on October 28,
  2026 as he sees them: `/logout` live (it would sign the owner's account out) and a Claude Code
  worker raised from a pi main.
- A place for main's questions to the person (the owner, October 31, 2026: "я иногда не вижу что
  ты мне пишешь и очень легко могу прозевать"). main marks a message as one that waits for the
  person's answer, and it lands whole in a panel of the shell beside the conversation; there can be
  many, each with an input under it for the answer, and one submit at the bottom sends main all the
  answers at once. main can edit or withdraw an entry to keep it current. The owner's word on the
  rest: a button with an envelope and the count of open entries opens the panel; where main offers
  choices they are picked as Claude Code's own question dialog picks them, not crude buttons;
  entries are kept across a restart of the room until answered; an answered entry leaves the view
  quietly, reachable but out of the way ("ответив я уже навряд ли войду смотреть прошлые
  сообщения"); workers' blocking questions do not go there — main decides them or stops and waits
  for the person.
- A headless worker as a fallback where no terminal is at hand.
- A room panel, and panels that are not terminals, with the modal.
- Conversation reset — `rewake clear` from main — and a conversation marker in `list` and in
  notices.
- A `rewake models` command listing the models and efforts a harness offers.
- A slot in the room for heavy test runs: an agent takes it before a heavy run and waits for
  it, woken when it is its turn; it would replace the manual rule of `AGENTS.md`, "Checks".
- Recording the directory a session actually works in, for `rewake worktree rm`
  ([worktree.md](../worktree.md)).
