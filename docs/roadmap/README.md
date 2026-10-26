---
description: The open work and its order — the owner's looks and logins, and the acceptance of additive fields — and the ideas not scheduled.
---

# Roadmap

The open work and its order. A step closes when it works live and its acceptance passes, not
when its code exists; a closed step leaves this file, its commit's subject is its record, and
what it taught goes into the living documents.

## Open, in order

1. **The owner's looks and logins** — built and checked under a stand-in terminal, waiting for
   the owner on a real one:
   - the shell's rows and band, main's strip with the hints under it and the bar of workers under
     the worker's pane ([shell.md](../shell.md#the-bar));
     a terminal that reports no background gets no band;
   - the strip: its cache part in the owner's room, its type part, a real model for pi's cache
     life ([shell.md](../shell.md#the-strip));
   - `/logout` live: read from Claude Code's reason list, not seen, since a real one would sign
     the owner's account out;
   - the workers' configuration directory after the owner's `/login`, then main's live check of
     a first start, the plan and usage, and the `/resume` picker
     ([worker-profile.md](../worker-profile.md#the-login));
   - a Claude Code worker raised from a pi main, which needs the same login.
2. **Acceptance of additive fields** — the cache telemetry, the `status/line` endpoint,
   `worker.type`, `configuredModelName`, `worker.label`, and `thinking` (the telemetry's and
   `StatusLineRequest`'s, October 26, 2026) changed the protocol or the state
   by adding fields; only the dialog fields' acceptance is on record. Whether the others need
   it is main's call (`AGENTS.md`, "Delegation and review").

## Not scheduled

- A headless worker as a fallback where no terminal is at hand.
- A room panel, and panels that are not terminals, with the modal.
- Conversation reset — `rewake clear` from main — and a conversation marker in `list` and in
  notices.
- A `rewake models` command listing the models and efforts a harness offers.
- A slot in the room for heavy test runs: an agent takes it before a heavy run and waits for
  it, woken when it is its turn; it would replace the manual rule of `AGENTS.md`, "Checks".
- Recording the directory a session actually works in, for `rewake worktree rm`
  ([worktree.md](../worktree.md)).
