# rewake

[![npm](https://img.shields.io/npm/v/@praline-labs/rewake)](https://www.npmjs.com/package/@praline-labs/rewake)
[![downloads](https://img.shields.io/npm/dm/@praline-labs/rewake)](https://www.npmjs.com/package/@praline-labs/rewake)
[![license](https://img.shields.io/npm/l/@praline-labs/rewake)](https://github.com/praline-labs/rewake/blob/main/LICENSE)

Let one coding agent run a team of others on your machine.

You start one session, **main**, in your project. Main raises its own workers — a writer, a
reviewer, a helper — each in its own terminal pane, hands them tasks, and reads their reports
as they come back. A worker that sits idle wakes when mail arrives; every task gets its
report, and who owes whom one is always on record. You watch it all in rewake's shell: main on
the left, the worker you choose on the right.

The harnesses are Claude Code and pi, in any mix: a main on Claude Code can run workers on
pi, and the other way round.

## Install

```bash
npm install -g @praline-labs/rewake
rewake --version
```

Linux on x64 or arm64. npm installs rewake, its shell and the pi it runs, which needs Node.js
22.19 or newer. Claude Code you install yourself, 2.1.287 or newer, and log in to as usual.

## Your part

1. In your project's directory, from your own terminal:

   ```bash
   rewake claude    # or: rewake pi
   ```

   This is main, `main-claude`, in the room of this directory. Claude Code's or pi's own
   flags go after the word, as with the plain command: `rewake claude --continue`.

2. Tell main what you want, in plain words:

   > Raise a writer and a reviewer. Have the writer fix the failing login test in a checkout
   > of its own, have the reviewer check it, and tell me when it is ready to land.

3. The first time a Claude Code worker opens, its pane says "Not logged in", and main tells
   you so: click that pane, type `/login`, and log in with your account. Once is enough for
   every later worker. On pi, log in once with `/login` in the first pi session.

That is all a person does. Workers are raised from **templates**, which main reads with
`rewake workers`; rewake ships none, so the first time main will ask for some — let it write
`~/.config/rewake/aliases.toml` for you, or write it yourself
([the guide](docs/user-guide.md#worker-templates)).

## What main does from there

- **Raises workers** from the templates, each on the harness, model and effort the template
  names, in a checkout of its own when asked: `rewake space open api --alias write`.
- **Sends tasks, questions and notes** — `rewake send api-claude "..."` — and reads every
  report when the worker's turn ends; a worker that is still at it says what it waits for.
- **Answers the workers' permission requests and questions**, which come to it as mail; you
  may answer first in the worker's pane.
- **Watches and steers**: each worker's activity, model and context fill; compacts a
  worker's conversation or stops its turn.
- **Lands the work** from a worker's checkout with `rewake worktree land`, and closes a worker
  whose work is done.

Every command is meant for the agents and prints its own help: `rewake guide` is the map,
`rewake <command> --help` the details. Closing the shell, or its terminal, ends main and all
its workers; nothing runs on unseen.

## Read more

- [docs/user-guide.md](docs/user-guide.md) — the few things only you do: logins, templates, a
  project's `.rewake/`, the shell's keys, stopping and restarting, where things live,
  upgrading.
- [docs/project.md](docs/project.md) — what rewake is and the principles it holds.
- [docs/flow.md](docs/flow.md) — the path of one message, from launch to report.
- [docs/README.md](docs/README.md) — the map of the documentation.

To build from source you need Go 1.25 or newer and [Bun](https://bun.sh) for the shell;
[docs/install.md](docs/install.md) has the steps.

## License

MIT. See [LICENSE](LICENSE).
