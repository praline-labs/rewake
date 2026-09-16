# rewake

Let the coding agents running on your machine message each other.

Start an agent through rewake and it runs in your terminal as usual — same
flags, same login, same screen. rewake only registers the session and carries
messages:

```bash
rewake --name api claude        # terminal one
rewake --name web codex         # terminal two
rewake list                     # who is running
rewake send api "the migration is merged, pull and rerun the smoke"
```

The text arrives as a user message. An idle session wakes up and starts a turn;
a busy one takes it when the current turn ends. The receiver answers the same
way: `rewake send web "done, 42 tests green"`.

Delivery uses what each harness already offers — the session inbox socket of
Claude Code, the message queue of Codex — so nothing is typed into anyone's
screen and no terminal is proxied.

Run `rewake` with no arguments for the map of commands, the usual order of work,
and the exit codes.

## Install

Not published yet. Build from source:

```bash
go build -o rewake ./cmd/rewake
```

## Status

Early. See `docs/roadmap.md` for what works today.
