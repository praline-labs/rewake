# rewake

A tool that lets interactive coding-agent harnesses on one machine talk to each
other. A person starts an agent through it — `rewake claude`, `rewake codex` —
and gets the ordinary program in their terminal; the session is registered, and
any other session writes to it with `rewake send <name> "text"`. The receiver
is told in one line that a message is waiting, which wakes it when it is idle,
and reads the text with `rewake inbox`.

The caller of these commands is an agent running them from its own shell. So the
machine-readable form matters more than the pretty one, a refusal must name the
next action, and nothing ever waits for input: a CLI that prompts hangs an agent
forever.

## Where to start a session

0. `docs/flow.md` — the whole path in one read: a session starts, a message is
   sent, announced, read, and the report comes back.
1. `docs/roadmap.md` — what is done and what comes next. A milestone is closed by
   its acceptance criterion, not by code existing. Findings and fixes of the
   earlier review rounds are in `docs/reviews.md` and `docs/reviews-later.md`.
2. `docs/design.md` — how it works: processes, state directory, interface. Two
   parts live next to it: `docs/launch.md` (launching a harness, signals) and
   `docs/delivery.md` (sending, reading, reports, the answer to a question).
3. `docs/research.md` — facts about each harness, marked with where they were
   verified. They age with harness versions: re-check before touching an adapter.

## Keeping the documentation true

The documents above are the memory of the project; the code is not. Whatever
is not written there has to be recovered from the code next time, and that is
slower and less reliable than writing it down while it is fresh. So the
documentation is part of every change, not a task after it:

- A change of behaviour or contract lands in the same commit as the code:
  `docs/flow.md` when the path of a message changes, `docs/design.md`,
  `docs/launch.md` or `docs/delivery.md` when the mechanism does,
  `docs/research.md` when a fact about a harness is learned or found wrong.
- Every review round and every milestone is recorded in `docs/roadmap.md` as
  soon as it closes: what was found, what was done, what stays open. Older
  rounds move to `docs/reviews-later.md`; a file that passes 400 lines is split
  by date.
- An owner decision is written down where it applies, dated, in their words
  where it matters.
- Before a commit, ask what the change taught that the documents do not yet
  say — and what they say that is no longer true. Rewriting what has gone
  stale is part of the same change: outdated text is corrected or removed,
  never left beside the new and never postponed to a clean-up later.
- A fact about a harness carries where it was verified (which version, live or
  read in the source), because these facts age.

## Checks

```bash
gofumpt -l .                    # empty (stricter than gofmt, so gofmt is covered)
go vet ./...
staticcheck ./...
golangci-lint run ./...         # config in .golangci.yml; golangci-lint fmt formats
go test -race -shuffle=on ./...
```

All five green is the condition for a commit. A red check is never somebody
else's: everything in the working tree belongs to the current work. The tools
are installed with `go install` into `~/go/bin`, which has to be on `PATH`:

```bash
go install honnef.co/go/tools/cmd/staticcheck@latest mvdan.cc/gofumpt@latest \
  github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
```

Live runs happen in a separate `/tmp` directory with its own `REWAKE_DIR`. Never
touch the harness sessions the owner is working in. Codex runs spend
subscription quota: cheap model, short messages, warn the owner first.

## Adding a harness

Meant to be one move:

1. A new package `internal/harness/<name>` with a type implementing
   `harness.Harness`: `ID`, `Title`, `Summary`, `Examples`, `Notes`, and from
   milestone 3 on, launch and delivery.
2. One line in `internal/harness/catalog/catalog.go`.

Everything else follows: the launch command, the guide entry, its own `--help`
page, the step in FLOW, the `harnesses` field of the machine form. A harness
cannot be half-registered — either it is in the catalogue and fully described, or
it does not exist.

## Adding a role or a message kind

Also one move each. A role is a value in `internal/role/role.go` plus its line in
the list; the launch flag, help and briefing follow. A message kind is a file
`internal/cli/send_<kind>.go` with a `messageKind` plus its line in `sendKinds`.

## How the CLI is organised

The reference is `i-plane` (`~/code/self/free-plane/i-plane`), which took these
patterns furthest.

- **One command table** — `internal/cli/registry.go`. The parser, the guide, the
  help pages and the hint on a refusal are all derived from it. A table that has
  drifted from behaviour is worse than none: it teaches a wrong call with
  confidence.
- **No arguments prints the guide**, not an error: the first call of a session
  should teach how the tool behaves.
- **Model and printing are separate**: a command builds a model, `printValue`
  prints either the lines or that same model under `--json`. Forgetting `--json`
  support is not possible this way.
- **An unknown flag or a stray positional is always a refusal.** A silently
  ignored flag returns an unfiltered answer with exit code 0, and the caller
  believes it.
- **A refusal** carries the reason, the syntax, the examples, the flags and
  `full help:`.
- **Examples are real invocations** — they get copied verbatim; a test parses
  every one of them.
- **No colour, no TTY detection**: the output is the same everywhere.

Exit codes are a contract, printed in the guide: 0 done, 1 the target refused or
could not be reached, 2 the call was wrong, 3 the message was accepted but not
delivered yet.

## Boundaries

- No pseudo-terminal proxying and no typing into anyone's screen: delivery only
  through handles a harness offers itself.
- The user's harness configuration is never edited; what is needed is passed as
  flags for a single launch.
- Transcript contents are not read.
- There is no daemon: a wrapper lives exactly as long as its session.

## Code

Go, standard library only. Comments and doc comments answer "why" rather than
restating the line below them. Everything — code, comments, docs, commit
messages — is in English; commits are a single subject line, no body.

Keep a file under 400 lines; split by subject, not by size.

## Commits

Author the commit as the owner and keep both agents as co-authors: every commit
here ends with two `Co-Authored-By` trailers, one for Codex and one for Claude,
whichever of them wrote the change — the orchestrator sets the work and checks
it, the executor writes it, so the record names both. Never one without the
other. The current lines:

```
Co-Authored-By: Codex (gpt-6-astra) <noreply@openai.com>
Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>
```
