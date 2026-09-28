# Grants hardened after the reconnaissance of September 28, 2026

A read-only reconnaissance of the code that landed September 26–28 found holes in the
grant tiers, in who a grant is taken from, and in how long a task keeps its grant after a
resume. Main set the scope the same day: the grant and resume findings, and the tests the
reconnaissance found missing there. The worktree findings are the next package.

## What was found and done

- **An edit re-registered the grant its letter named.** `rewake edit` copied the grant
  fields of the unread letter on disk, which the worker can rewrite, and registered them
  in main's name. Main's wrapper now carries over what it holds for the task replaced
  (`grantauth.Carry`); a letter naming a grant the wrapper does not hold is refused.
- **Windows short names passed the hard tier.** `/mnt/c/PROGRA~1` is the directory
  `/mnt/c/Program Files`, checked live on WSL by inode, and compared as a string it was
  neither. A short element is read back to its long name by inode in its parent's
  listing, and one that matches none is refused (`internal/grant/shortnames.go`).
- **An unread task of an ended run kept its grant.** Main's wrapper counted it open, and
  a resume, which cannot read another run's mail, could be handed the grant. It is closed
  once the run it was written for has ended.
- **Main held another conversation than the letter was pinned to.** The grant check
  named the session's conversation at the first try, a task that waited for an idle
  reader could be pinned to another after a `/clear`, and main kept the first. The check
  now names none; the wrapper names the pinned conversation under the mailbox lock as
  the letter becomes readable (`inbox.Server.PinGrant`). Acceptance found main still
  keeping the first pin: a delivery refused after it — `turn/start` refused while the
  conversation compacts — is pinned again at the retry, maybe into another
  conversation, and a resume of that one was refused. Main now keeps the latest name
  from the recipient's wrapper, as the recipient's own record does.
- **A shielded directory given as the root passed.** `.git`, `.claude`, `.codex` and
  `.agents` are shielded inside a grant, and nothing shielded the grant's own root. A
  directory with one of them in its path is broad; the metadata `--grant-git` adds is
  exempt (`Rules.CheckGitMetadata`).
- **The hard tier missed places that run as the owner, and a link in `/tmp` hid its
  target.** Added `~/.config/gh`, `~/.config/fish`, `~/.local/share/systemd`,
  `~/.local/share/applications`, the Go module and build caches, and the toolchain above
  each `bin` on `PATH`. A path through a shared temporary directory to somewhere outside
  it is refused, and send prints each granted directory as resolved. Acceptance found a
  link outside passing through one on its way — `~/work/link` to `/tmp/out` to
  `~/notes` — let through, since only the name given was looked at; every step of the
  resolution is looked at now (`grant.steps`).
- **Adoption restarted the resume window.** A wait taken over now keeps the time the
  earlier run read the task.
- **A failed delivery did not settle its task**, and a wait record a worker wrote
  outweighed a status saying taken back or failed. Such a status is read first now, in
  `inbox.Settled` and `inbox.TaskOpen` alike.
- **The help taught the tiers from before.** Moved onto main after the CLI-truth round,
  whose send notes and main's playbook listed the hard tier and the broad one as they
  were; they now name the added places, a path through `/tmp` on its way elsewhere, a
  short name that cannot be read back, and a directory with `.git`, `.claude`, `.codex`
  or `.agents` in its path as broad.
- **Tests** for each of those, each checked against the fix taken out, and for the gaps
  the reconnaissance listed in the grant authority, the keeper, the tiers, the Claude Code
  permission decisions, the Codex grant path and the resume.

## What stays open

- On Claude Code a message granted a second time is not refused, while
  [grants.md](../grants.md#taking-a-grant-back) says it is; out of this package's scope.
- The hard tier is a list of known places, not every one
  ([grants.md](../grants.md#what-a-grant-does-not-stop)).
- The keeper's journal entry on Claude Code still names the conversation read just
  before the pin, not at it; the two differ only if the conversation changes within
  those milliseconds.
