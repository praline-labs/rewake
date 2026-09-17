# rewake

Let the coding agents running on your machine message each other.

Start an agent through rewake and it runs in your terminal as usual — same
flags, same login, same screen. rewake only registers the session and carries
messages:

```bash
rewake --main --name lead claude   # terminal one: lead-claude hands out work
rewake --write codex              # terminal two: write-codex takes tasks
rewake list                       # exact addresses in this room
rewake send write-codex "the migration is merged, pull and rerun the smoke"
```

Run the send command from lead-claude. The receiver sees
`Rewake: lead-claude task, 1 new message(s)` and reads the text with `rewake inbox`.
An idle session wakes up; the server transport can steer an active turn. The
receiver ends its turn with the result, which returns as `Rewake: write-codex finished`.

Without a role flag, every session starts as general, even in an empty room.
`rewake codex` starts general-codex, then general-codex-2. Only `--main` creates
an orchestrator; `--name main` changes the address, not the role.

Names use the selected role as a prefix, followed by the harness ID. `--name`
replaces only the prefix: `rewake --write --name megamozg codex` starts
megamozg-codex. Automatic collisions add -2, -3; explicit conflicts refuse.
The complete address must fit 32 characters. Existing sessions keep their names.

Delivery uses what each harness already offers — the session inbox socket of
Claude Code, the owned app-server of Codex — so nothing is typed into anyone's
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
