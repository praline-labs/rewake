# Research: Claude Code's worktree

Facts about Claude Code's own worktree that rewake's `--worktree`
([worktree.md](worktree.md)) is built beside: how a conversation is continued, where
trust is kept, what `.worktreeinclude` copies. Split out of
[research-launch.md](research-launch.md) on September 27, 2026 by subject; what a live
`-w` session showed stays there
([research-launch.md](research-launch.md#a-worktree-at-launch)). Codex's own worktree is
in [research-codex.md](research-codex.md#--worktree-with-a-remote-terminal).

The facts come three ways, each marked: `claude --help` of the installed binary, the
owner's reading of that binary, and a reference copy of Claude Code's source whose
version is not recorded — older than 2.1.280, so it can differ, and does in one place
below. Recheck before touching the Claude Code adapter's worktree.

## Continuing a conversation, and trust

**[`claude --help`, Claude Code 2.1.280; September 27, 2026]** The flags that continue a
conversation rather than start one: `-c`/`--continue` (the most recent in the current
directory), `-r`/`--resume [value]` (by id, or a picker), `--from-pr [value]` (one linked
to a pull request), `--teleport [session]`, and `--fork-session` (a new id for any of
them). `-w`/`--worktree [name]` makes Claude Code's own worktree; `--tmux` opens it in
tmux and requires `--worktree`. There is no flag that names a working directory: the
harness works where it is started. rewake takes the long `--worktree` for its own
checkout and refuses `-w`, `--tmux` and every continuation beside it
([worktree.md](worktree.md#the-launch)).

**[`claude --help`, Claude Code 2.1.280; September 27, 2026]** Beside those: the
subcommands `attach <id>`, which opens a background session in this terminal, and
`respawn [id]`, which restarts one; `--cloud [description|session_id|url]`, which
creates a cloud session or attaches to an existing one; `--environment
<environment_id>`, which creates a new cloud session that runs on the given self-hosted
environment; and
`--session-id <uuid>`, "use
a specific session ID for the conversation". The short flags taking a value are `-d`
(`--debug [filter]`), `-n` (`--name <name>`), `-r` (`--resume [value]`) and `-w`
(`--worktree [name]`); `-c`, `-h`, `-p` and `-v` take none. rewake refuses `attach`,
`respawn`, `--cloud` and `--environment` beside `--worktree`, and reads a cluster of short flags as ending
at the first letter that takes a value.

**[live, Claude Code 2.1.280, a scratch home with no login, no model call; September
27, 2026]** A word after `--` is not taken for a subcommand: `claude logs zz9` ran the
subcommand and answered that no job matches, `claude -- logs zz9` started a session
with it as the prompt and stopped at the missing login. So a prompt that is the bare
word `attach` goes after `--`, as the refusal says.

**[reference source, `main.tsx:1276-1300`; commit of April 4, 2026, its version not
recorded; read September 27, 2026, not run]** `--session-id` starts a new conversation
under that id: an id already in use is refused ("Session ID … is already in use"), and
beside `--continue` or `--resume` it is accepted only with `--fork-session`, which
rewake refuses anyway. So it is not a continuation, and rewake lets it through with
`--worktree`.

**[reference source, `utils/crossProjectResume.ts`, `utils/sessionRestore.ts`,
`utils/config.ts`; commit of April 4, 2026, its version not recorded; read September 27,
2026, not run]** A conversation is kept with the directory it ran in. Resuming one begun
in another directory does not move the session there: it prints a
`cd <path> && claude --resume <id>` to run instead. One begun in Claude Code's own
worktree is resumed by changing into that worktree. So a continuation launched in a new
checkout either finds nothing, for `--continue`, or leads back out of it. Trust is kept
under the canonical git root — `getProjectPathForConfig` through `findCanonicalGitRoot`
— which for a linked worktree is its main checkout: a checkout of a trusted repository
should need no dialog, not checked live.

## Names with a slash

**[reference source, `utils/worktree.ts:48-88`, `:207-227`; commit of April 4, 2026, its
version not recorded; read September 27, 2026, not run]** A worktree name may hold `/`:
each element must be non-empty and of `[a-zA-Z0-9._-]`, neither `.` nor `..`, and the
whole at most 64 characters. `flattenSlug` writes each `/` as `+` both in the directory,
`.claude/worktrees/feat+x`, and in the branch, `worktree-feat+x`: nested, a branch
`worktree-feat` would stand where `worktree-feat/x` needs a directory, and a checkout
`feat/x` would lie inside the checkout `feat`, whose removal deletes it. `+` is outside
the allowed characters, so the mapping is one to one. rewake takes the characters and
the directory's `+`; its branch keeps the slash, by the owner's decision
([worktree.md](worktree.md#the-launch)), and a branch above or below the name is
refused rather than avoided.

## `.worktreeinclude`


**[the owner's check of the 2.1.280 binary, September 27, 2026]** After making its
worktree, Claude Code copies from the main checkout the files git ignores that a
`.worktreeinclude` at the repository's top names: `.gitignore` syntax, only ignored
files — a tracked file the file names is left to the commit — symbolic links skipped,
and a copy rather than a link.

**[reference source, `utils/worktree.ts:378-500`, `copyWorktreeIncludeFiles`; commit of
April 4, 2026, its version not recorded; read September 27, 2026]** How it finds them:

- the file is read from the repository's top; no file, or only blank and `#` lines,
  copies nothing;
- `git ls-files --others --ignored --exclude-standard --directory` lists the ignored
  entries, a wholly ignored directory collapsed to one `dir/` entry so `node_modules`
  is not walked; every file entry the patterns match (the `ignore` library, `.gitignore`
  semantics) is copied;
- a collapsed directory is listed again with a second `ls-files -- <dirs>` only when a
  pattern opens it: the pattern, with a leading `/` dropped, starts with the directory's
  path; the literal part of a glob before its first `*`, `?` or `[` is a prefix of the
  directory; or the directory itself matches. `**/x` and patterns with no slash do not
  open one;
- each file is copied with `copyFile`, its directories made first; a failure is logged
  and skipped.

The source differs from the binary in one place: `copyFile` follows a symbolic link
and copies what it points to, where the binary was seen skipping links — its strings
hold "Skipping symlink in .worktreeinclude". rewake follows the binary. Beside it the source has an opt-in `worktree.symlinkDirectories` setting
that links named directories into the worktree instead of copying; rewake has no
counterpart.

## What else Claude Code's worktree does

**[reference source, `utils/worktree.ts`; commit of April 4, 2026, its version not
recorded; read September 27, 2026]** Three things the source does that rewake does not
repeat. The worktree is made with
`git worktree add -B <branch>` (`:328`), which resets a branch of that name that already
exists; rewake refuses such a name. It sets `core.hooksPath` in the configuration the
checkouts share, pointing them at the main checkout's hooks (`:540-570`); rewake writes
no configuration. And a periodic cleanup removes agent worktrees older than a cutoff,
looking at them with `git status -uno`, which counts untracked files as build output
(`:1044-1132`); rewake removes nothing by itself, and untracked files keep a checkout.
