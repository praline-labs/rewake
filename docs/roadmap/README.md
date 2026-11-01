---
description: The open work and its order — rewake's face after its first slice — and the ideas not scheduled.
---

# Roadmap

The open work and its order. A step closes when it works live and its acceptance passes, not
when its code exists; a closed step leaves this file, its commit's subject is its record, and
what it taught goes into the living documents.

## Open, in order

The architecture pass of November 1, 2026 (the owner asked for every candidate, main sets the
order): deepening the modules the last stretch changed most, each step one module with its tests
at its interface, the old tests that reached past it removed.

1. One mirror feed under both adapters: the bounded outbox, text bounding and settle-in-parts of
   `internal/adapter/pi/outbox.ts` and the mod's `hooks/mirror.ts` + `sizes.ts` as one module, the
   adapters mapping only their harness's events; one decoder of a neutral event in `core/mirror`.
2. Reading a mailbox, `rewake pending` and the owed hint as operations of `core/mail`, the lock
   and the order of effects inside them, not in `internal/cli` closures.
3. A tool-call module in the face: one place says a call's heading, argument, fold, copied field
   and result kind, for `tools.ts`, `parts.ts`, `fold.ts`, `results.ts`, `render.ts` and pi's
   `toolmap.ts`.
4. A sent letter as one module of `core/mail`: send, edit, withdraw and addendum hand it an
   intent and a reference; it re-resolves, locks, checks standing, writes and returns an outcome.
5. A turn's phase and retry attempt carried by the mirror model from the adapters' events, not
   inferred in the face's `progress.ts`.
6. The face's input rule from the mirror alone (mode and thinking level beside each other), and
   the shell's `check.ts` derived from the face's `messages.ts`.
7. A typed part reference in the face, owning what a row is and what a click on it does.
8. Settling a letter with an outcome as an operation of `core/mail`, the delivery server keeping
   only scheduling, the window and retries.
9. One handle on a session's mail instead of `(dir, self, epoch)`; `mail.world` folded away.
10. The face's worker as a module with an interface (post, socket, clock), tested in process.
11. A command's form parsed once, answering both its access class and its run; a send kind
    owning its step after the write.
12. Each owner's on-disk layout beside it (mail, brief, space), `infra/state` keeping the lease,
    the guard and atomic writes.

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
