---
description: The index of the numbered rules — effects, turns, launch, space — and the unnumbered ones of layout, the adapter API and state; how a rule is written.
---

# The rules

The rules rewake keeps, each in its full wording, as a reference of behaviour. A rule is
stated independently of the code: what the system guarantees, not how one file does it.

- [effects.md](effects.md) — **E1–E8**, the effects of a mail operation: effects undone
  only on evidence, locks, checks under the lock, one durable read, the size bound, three
  outcomes of a lookup, identity and scope, and the order of a turn end's steps; the rules
  the receipts of `send` and `pending` keep. Open it before changing how mail is written,
  read or published.
- [turns.md](turns.md) — **O1–O2**, turn outcomes: an interim wake that must not close a
  task early, failed and interrupted turns.
- [launch.md](launch.md) — **L1–L3**, what a launch adds: the mail and nothing else, where
  and never what, nothing past the run.
- [space.md](space.md) — **SP1–SP10**, a space: a hosted worker, its record and its
  closing. Open it before changing how a space or a hosted launch behaves.
- [layout.md](layout.md) — the layout, rules without a number: what each layer may
  import and no registry of adapters.
- [adapter-api.md](adapter-api.md) — the adapter API, rules without a number: the
  capabilities and what the core does without one, the catalogue, minimum versions.
- [state.md](state.md) — state and builds, rules without a number: the state root, the
  build's identity, the room's lease, views and changes through the write seam.

## How a rule is written

A rule is a list item opening with its number in bold — `- **E4.` — and runs to the next
rule or heading. The group is a reference of behaviour: a rule is written down where
behaviour is not obvious, and it is not required to name tests (the owner's decision of
October 17, 2026). A rule may still end with a `Tests:` line and one item per test — the
file, from the module root, and the function, each in backticks — as a pointer to where the
behaviour is held; no check reads these lists, so a pointer found stale is corrected when
met. A clause the code does not hold is stated in the rule's text as a known limit.

```markdown
- **E4. A read's completion is one durable fact every channel shares.** ...
  Tests:
  - `internal/core/mail/unread_test.go` `TestReadingHandsEachMessageOnce`
```

A move rewrites the paths in the same commit.
