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

1. `docs/roadmap.md` — what is done and what comes next. A milestone is closed by
   its acceptance criterion, not by code existing.
2. `docs/design.md` — how it works: processes, state directory, delivery, interface.
3. `docs/research.md` — facts about each harness, marked with where they were
   verified. They age with harness versions: re-check before touching an adapter.

## Checks

```bash
gofmt -l .        # empty
go vet ./...
go test ./...
```

All three green is the condition for a commit. A red check is never somebody
else's: everything in the working tree belongs to the current work.

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

Author the commit as the owner and keep the agent as co-author: every commit
here ends with the `Co-Authored-By` trailer of the agent that wrote it, plus the
session link. The work is shared, so the record says so.
