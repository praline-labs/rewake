# The rules of 2.0

The numbered rules rewake 2.0 keeps, each in its full wording and each naming the tests
that hold it. The wording is the one fixed in [design-rules.md](../v2/design-rules.md);
which 1.x rule each comes from is in [revision-rules.md](../v2/revision-rules.md). A rule
here is stated independently of the code: what the system guarantees, not how one file
does it. Created in stage 3's first step, S1 ([stage3.md](../v2/stage3.md#documents)).

- [effects.md](effects.md) — **E1–E8**, the effects of a mail operation: unknown effects
  never discarded, locks, checks in the critical section, one durable read, the size
  bound, three outcomes, identity and scope, evidence and the stop. Open it before changing
  how mail is written, read, published or stopped.
- [turns.md](turns.md) — **O1–O2**, turn outcomes: an interim wake that must not close a
  task early, failed and interrupted turns.
- [tools.md](tools.md) — **T1–T11**, what binds every tool transport: bindings, one
  operation per call, bounds, a read only on proof, commits stopping at the turn's end, one
  build per room.
- [channel.md](channel.md) — **C1–C8**, the channel record of a run: observations, what
  proves a channel works, silence, event order, notices, policy refusals.
- [launch.md](launch.md) — **L1–L3**, what a launch adds: the mail and nothing else, where
  and never what, nothing past the run.
- [host.md](host.md) — the host's tests and the grant scheme, rules without a number.

Later steps add the operator decision's rules **D1–D8** to `effects.md` (S18, part B),
and `layout.md`, `adapter-api.md` and `state.md` (S19).

## How a rule names its tests

A rule is a list item opening with its number in bold — `- **E4.` — and runs to the next
rule or heading. It ends with a `Tests:` line and one item per test: the file, from the
module root, and the function, each in backticks, optionally followed by a dash and what
happens to the test in a later step. A clause no test holds yet is a `Gap:` item naming
the step that closes it, `closed in S<n>`. The host's rules, which have no number, use the
same `Tests:` block; each is a section the rules test knows by its heading, so a section
or its block cannot disappear unnoticed.

```markdown
- **E4. A read's completion is one durable fact every channel shares.** ...
  Tests:
  - `internal/cli/bridge_rules_test.go` `TestALateAcknowledgmentOwesNothingTwice`
  - `internal/inbox/claims_test.go` `TestALetterBeingReadInPartsCannotBeTakenBack`
  - Gap: one fact across every channel — today tool and shell only — closed in S7.
```

A move rewrites the paths in the same commit. Tests that hold only the 1.x migration,
`settle` or a removed harness mechanism are not named; which those are, and why, is in
[stage3-tests.md](../v2/stage3-tests.md) and [stage3-tests-tcl.md](../v2/stage3-tests-tcl.md).

## What checks them

`docs/rules_test.go` runs with the five checks of `AGENTS.md`. It parses these documents
and the Go files they name, and runs nothing. It fails when:

- a number of E1–E8, T1–T11, C1–C8, L1–L3 or O1–O2, or a section of the host's rules, is
  missing from these documents, or appears twice, or a number no group has appears;
- a rule has no `Tests:` block, or names no test and no gap, or a `Tests:` block belongs
  to no rule;
- a named test does not exist: the path is not inside the module, or the file declares no
  `Test`, `Fuzz` or `Example` function of that name — a file under a build tag is parsed,
  not built;
- `go test` would not run it: a lower-case letter after the prefix, a signature other
  than `func(*testing.T)` or `func(*testing.F)`, an example without an output comment;
- no `go test` command of `AGENTS.md`'s five checks reaches its package (`./...` skips a
  directory named with a leading `.` or `_`, `testdata` and another module) and builds
  its file with that command's own tags and `-race` mode;
- a `go test` command of the five checks uses a flag that may leave tests out, such as
  `-run`, ends with a `-count` below one, or names packages other than by directory
  patterns; or runs other than as `go test`, optionally behind `env -u NAME`: a prefix
  that sets a variable, such as `GOFLAGS=-count=0`, or wraps the command fails rather
  than being read past;
- a gap names no step, or a step that is not a row of the table under
  [the build order](../v2/stage3.md#the-build-order); a table or fenced example
  elsewhere names no step, and a build order that cannot be read fails.

These documents, `AGENTS.md`'s checks, the build order and the map's links are read by
one Markdown reader (`docs/markdown_test.go`, the same text as
`internal/layout_markdown_test.go`, which the layout test reads the build order with). A
fence opens with three or more backticks or tildes and closes only with a run of the same
character at least as long and no trailing text. Fenced examples supply no rules or steps.
Test commands are read only from the first fenced block under Checks in AGENTS.md. What
the reader cannot place without modeling the lists around it fails: a fence indented four
or more or closed at another indentation, a line less indented than its fence, a backtick
in a backtick fence's info string, a tab before a fence, a fence never closed, an HTML
comment in a rule document, AGENTS.md or the build order, a setext heading, an indented
build-order table, a Tests line indented four or more.

The map blanks HTML comments before using the reader, preserving its existing treatment of
hidden links and headings.

From S19, any gap fails: that is how the stage's acceptance, "every carried rule names its
tests and they exist", is checked rather than read.
