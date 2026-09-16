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

The receiver is told in one line — `rewake: web notify, 1 new message` — and
reads the text itself with `rewake inbox`. An idle session wakes up for it; a
busy one sees it when the current turn ends. It answers the same way, and when
its turn ends the sender hears `rewake: api finished` with its last reply.

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
