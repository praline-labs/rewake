# The rules of 2.0

The numbered rules rewake 2.0 keeps, each in its full wording, as a reference of
behaviour. The wording is the one fixed in [design-rules.md](../v2/design-rules.md);
which 1.x rule each comes from is in [revision-rules.md](../archive/v2/revision-rules.md). A rule
here is stated independently of the code: what the system guarantees, not how one file
does it. Created in stage 3's first step, S1 ([stage3.md](../archive/v2/stage3.md#documents)).

- [effects.md](effects.md) — **E1–E8**, the effects of a mail operation: effects undone
  only on evidence, locks, checks under the lock, one durable read, the size bound, three
  outcomes of a lookup, identity and scope, and the order of a turn end's steps; D1–D8, the
  operator decision, were removed on October 18, 2026. Open it before changing how mail is
  written, read or published.
- [turns.md](turns.md) — **O1–O2**, turn outcomes: an interim wake that must not close a
  task early, failed and interrupted turns.
- [launch.md](launch.md) — **L1–L3**, what a launch adds: the mail and nothing else, where
  and never what, nothing past the run.
- [space.md](space.md) — **SP1–SP10**, the first working space of stage 4: a hosted worker,
  its record and its closing. A rule enters in the step that builds its test;
  SP3, SP4 and SP9 are in force from S4-2, SP2 and SP7 from S4-3, SP1 from S4-7, SP10 from `space close`, October 19, 2026. Open it before changing how a space or a hosted
  launch behaves.
- [layout.md](layout.md) — the layout, rules without a number: what each layer may
  import and no registry of adapters.
- [adapter-api.md](adapter-api.md) — the adapter API, rules without a number: the
  capabilities and what the core does without one, the catalogue, minimum versions.
- [state.md](state.md) — state and builds, rules without a number: the state root, the
  build's identity, the room's lease, views and changes through the write seam, clearing
  the state of 1.x.

## How a rule is written

A rule is a list item opening with its number in bold — `- **E4.` — and runs to the next
rule or heading. Since the owner's decision of October 17, 2026 this group is a reference
of behaviour: a rule is written down where behaviour is not obvious, and it is not required
to name tests. A rule may still end with a `Tests:` line and one item per test — the file,
from the module root, and the function, each in backticks — as a pointer to where the
behaviour is held; no check reads these lists, so a pointer found stale is corrected when
met. A clause the code does not hold is stated in the rule's text as a known limit, dated.

```markdown
- **E4. A read's completion is one durable fact every channel shares.** ...
  Tests:
  - `internal/core/mail/unread_test.go` `TestReadingHandsEachMessageOnce`
```

A move rewrites the paths in the same commit. Tests that hold only the 1.x migration,
`settle` or a removed harness mechanism are not named; which those are, and why, is in
[stage3-tests.md](../archive/v2/stage3-tests.md) and [stage3-tests-tcl.md](../archive/v2/stage3-tests-tcl.md).
