# Worktrees for a launch

Split out of [launch.md](launch.md#a-worktree-for-a-launch) on September 27, 2026, when
land, finish, Claude Code's launch and `.worktreeinclude` took it past the project's
400-line limit. How they are used — the owner starts each writer in one, since a session
cannot start another, and main lands and finishes it — is said there.

`rewake codex --worktree` and `rewake claude --worktree` give the session a checkout of
its own, on a branch of its own. Codex's terminal cannot make one under rewake — it
refuses `--worktree` together with `--remote`, before it creates anything, and rewake
always starts it with `--remote`
([research-codex.md](research-codex.md#--worktree-with-a-remote-terminal)) — so the
launch command takes the flag for itself and never passes it on. Decided by the owner on
September 26, 2026, as the variant that keeps only the substance of Codex's worktree:
a checkout made with the public `git worktree add`, and rewake's own record of it.
Codex's private scheme — its directory layout, a `--no-checkout` add, `config.worktree`,
the thread binding in the git metadata — is not repeated: it is no contract, and a copy
would drift from the next version silently. On September 27, 2026 the owner decided that
the checkout is on a new branch rather than a detached HEAD, that `land` and `finish`
bring the work back, and that Claude Code launches get the same checkout.

## The launch

- **The flag.** Read as a switch: `--worktree` alone asks for a generated name, `wt-`
  and six hex digits; `--worktree=<name>` names the checkout and its branch, exactly: a
  branch name `git check-ref-format --branch` takes, slashes included —
  `--worktree=feat/super-feature` makes the branch `feat/super-feature` — written in
  ASCII letters, digits, `.`, `_`, `-` and `/`, at most 250 bytes, neither `HEAD` nor
  forty hex digits, which git would read as something other than a branch, and not
  ending in `.json`. The owner decided on September 27, 2026 that a name may hold a
  slash as a branch's does; before, it was a letter or digit and up to 39 letters,
  digits, `-` or `_`. The characters are Claude Code's for its own worktree names
  ([research-worktree.md](research-worktree.md#names-with-a-slash)): they leave out `+`,
  which stands for the slash in the directory's name, and everything a shell reads, so
  the name goes into the commands a refusal prints as it is. The length is the longest
  whose record, `<name>.json`, and git's `.lock` beside a branch's last element fit the
  255 bytes of a file name. For Codex a spaced value is not
  read: the word after the switch stays the harness's, a prompt most often, as it would
  be for Codex itself. For Claude Code, whose own flag is `--worktree [name]`, such a
  word is refused, naming `--worktree=<word>` and `--worktree -- <word>`. The flag after `--` is prompt text and stays. Given twice, or as
  `--worktree=`, it is a wrong call. For Codex it is Codex's own spelling. For Claude
  Code it is the long spelling of its own flag; the short `-w` stays Claude Code's —
  a worktree inside the repository in `.claude/worktrees/<name>`, on a branch
  `worktree-<name>`, which it removes itself on exit
  ([research-launch.md](research-launch.md#a-worktree-at-launch)) — and a launch has one
  worktree, so `-w` or `--tmux`, which needs `-w`, beside `--worktree` is refused.
- **The checkout.** The repository is the one holding the launch directory — for Codex
  `-C` or `--cd` when given, else the current one; Claude Code has no such flag, so for
  it the current one. rewake adds a checkout of its HEAD commit on a new branch of the
  checkout's name, made when the name is claimed and checked out with
  `git worktree add`: the branch checked out in the source
  stays free, a second launch from the same place does not collide with the first, and
  the commits a session makes sit on a branch rather than on a detached HEAD, where
  nothing would hold them. A branch of that name already in the repository, a branch
  below it — `fix/login` for the name `fix`, beside which git keeps no branch `fix` —
  or a branch above it — `feat` for the name `feat/login` — refuses the launch naming
  the branch in the way with the next step: another name, or `rewake worktree ls` when the worktree is
  already there — a launch in its directory needs no `--worktree`. Uncommitted changes
  in the source do not come along; files git ignores come only as `.worktreeinclude`
  below names them.
- **Where.** `$REWAKE_WORKTREES` when set (absolute), else
  `$XDG_DATA_HOME/rewake/worktrees`, else `~/.local/share/rewake/worktrees`; under it one
  directory per repository, named for it with a short hash of its Git directory so two
  repositories of one name stay apart, and in that one each checkout beside its record:
  `<repository>-<hash>/<name>/` and `<name>.json`, with `+` for each slash of the name:
  `feat+super-feature/` and `feat+super-feature.json`, as Claude Code names its own.
  Directories nested by the slash would put one checkout inside another's when one
  name is below another, `feat` and `feat/login`, and a record among its files; no name
  holds a `+`, so two names never share a directory. Not the state directory — that lives
  in `/tmp` and would not outlive a restart, while a checkout holds work — and not inside
  the repository, where it would show up in `git status` and in the agent's own
  searches: a `$REWAKE_WORKTREES` inside it, compared with symbolic links resolved, is
  refused — inside the checkout the launch came from, or inside the main checkout when
  the launch came from a linked one. One place for every repository, so
  `rewake worktree ls` sees them all.
- **Where the launch starts.** At the launch directory's place within the checkout, as
  Codex does; at the checkout's top when that directory is not in the commit, an
  untracked one say. The wrapper changes into it before the session is registered, and
  for Codex `-C`/`--cd` are taken out of the arguments, so the record and the harness —
  Codex's terminal and app-server alike — all work there. Relative paths among the
  harness arguments resolve there too.
- **Trust.** Codex resolves a linked worktree's trust to its main checkout, so a
  launch from a trusted repository keeps that trust in its checkout
  ([research-codex.md](research-codex.md#--worktree-with-a-remote-terminal)). Claude
  Code keeps trust under the canonical git root, which for a linked worktree is its main
  checkout too — read in its source, not checked live
  ([research-worktree.md](research-worktree.md#continuing-a-conversation-and-trust)).
- **The record** says which repository (its shared Git directory and the checkout the
  launch came from), which commit, which branch, where, when, which rewake process made
  it, what `.worktreeinclude` copied — written as soon as the copies are made, under the
  lock that made the checkout — and, once the session's name is claimed, which session:
  name, room, run and the room's state directory. Between the checkout being made and
  the session being registered only the process that made it says the checkout is in
  use: while it runs and no session is claimed, `ls` shows the worktree as launching and
  rm keeps it.
- **A new conversation only.** A continuation is refused with `--worktree`: it continues
  a conversation in the directory it was started in. For Codex that is `resume` and
  `fork`. The terminal's `thread/resume` carries a cwd only from its own `-C`/`--cd`,
  which rewake takes out, and the server then restores the saved one, while the session's
  workspace roots are already the checkout's: the model would work in one place with its
  rights named for another (seen on 0.155.1 in a container with the real binaries,
  September 26, 2026; fork read in the source, not run). Moving the conversation into the
  checkout — handing the terminal the checkout as its `-C` — is not built: what that does
  to a continued conversation's permissions
  ([continuation-permissions.md](continuation-permissions.md)) was not checked. It is
  asked of the words, not of Codex's grammar: `resume` or `fork` as a whole argument
  anywhere before `--` refuses. The grammar lets a prompt come before the subcommand and
  ends an image's joined value (`--image=foo`, `-ifoo`) in its own argument where a
  separated one runs on, and a parser that followed it missed six forms the 0.155.1
  terminal takes as a continuation. Neither 0.155.1 nor 0.157.1 has another spelling: no
  alias, and a bare `--last` is refused by the parser. For Claude Code it is
  `--continue`/`-c`, `--resume`/`-r`, `--from-pr`, `--teleport` and `--fork-session`,
  a cluster of short flags in which `c` or `r` acts as a flag, and the subcommands
  `attach` and `respawn`, which open a background session where it was started: a
  conversation is kept with its directory, so in a new checkout `--continue` finds none
  and `--resume` of one begun elsewhere leads back out (read in its source, not run;
  [research-worktree.md](research-worktree.md#continuing-a-conversation-and-trust)). In
  a cluster a letter that takes a value — `-d`, `-n`, `-r`, `-w` — ends the flags, so
  `-dcache`, a debug filter, and `-ncircle`, a name, are not refused, while `-pc` and
  `-rID` are. `--session-id` is no continuation: it names the id of a new conversation,
  and Claude Code refuses an id already in use. `--cloud` is refused as well, new or
  attached, and `--environment`, which makes a cloud session on that environment: a
  cloud session works in none of this machine's checkouts. A prompt or an
  option's value that is the bare word is refused too; the refusal says such a prompt
  goes after `--`, where it is text (decided September 26, 2026). The refusal spells out
  both ways on: a new conversation with `--worktree`, or the continuation where the
  conversation was started — for one begun in a rewake worktree, `rewake worktree ls`
  names its path, and `rewake codex resume` or `rewake claude --continue` run there
  without `--worktree`.
- **How git runs.** Every git call rewake makes runs with `GIT_TERMINAL_PROMPT=0` and in
  a session of its own, with no terminal to open, so nothing prompts. Inherited `GIT_*`
  variables are cleared — a `GIT_DIR` from a hook would point the call at another
  repository, a `GIT_CONFIG_PARAMETERS` would carry a parent's `-c` into it — except
  those choosing the configuration files and the identity. The filesystem monitor is
  off, and so are the repository's hooks: a `post-checkout` hook would run the
  repository's code in the new checkout with no terminal before the session starts,
  which is the preparation rewake does not do. Both settings go in the environment of
  the one call; nothing is written to the repository's configuration, which its
  checkouts share. Clean and smudge filters stay on, or a file a large-file filter keeps
  would be checked out as its pointer. `land` is the exception for hooks: it moves the
  person's branch in their checkout, and the hooks the repository keeps for a merge or a
  ref update run as they would for the person's own `git merge --ff-only`, waited for
  at most a minute ([landing and finishing](#landing-and-finishing)). The same
  choices were weighed against Codex's own worktree, which turns filters off too
  ([research-codex.md](research-codex.md#--worktree-with-a-remote-terminal)), and
  against Claude Code's, which sets `core.hooksPath` in the shared configuration and
  resets an existing branch with `-B`
  ([research-worktree.md](research-worktree.md#what-else-claude-codes-worktree-does)); rewake does neither.
- **Refusals** exit 2 and nothing prompts. The session of its own keeps Ctrl-C from
  reaching git: a long `git worktree add` interrupted so
  runs to its end after rewake has gone, and its checkout is left with a record and no
  owner, which `ls` shows and rm removes (kept on purpose, September 26, 2026). Most
  come before anything is made: the flag given twice or as `--worktree=`, a
  continuation, `-w`, `--tmux`, `--cloud` or `--environment` beside it, Codex's `--remote`, a profile or
  `--oss`/`--local-provider`, a launch directory that does not resolve, a name out of
  shape, a launch directory outside any working tree, a repository with no commit yet, a
  worktree directory inside the repository. By then the worktree directory itself may
  have been created. A name taken by a checkout, by a directory in its place or by a
  branch is found once the repository's directory under it is made, which is removed
  again when empty; the refusal gives the next step: another name, `rewake worktree ls`
  and `rm`, or `cd` into the existing one. A refusal of the registration — a session name
  or a main already taken — comes after the checkout was made: it is taken back, and the
  line saying where the session works is printed only once the name is claimed.
- **A name is claimed before git runs.** The record is published under the name with
  an exclusive link, so of two launches asking for one name only one goes on, and a
  directory already standing where the checkout would go refuses the name. The branch
  is made in the same claim, with `git update-ref` against an empty old value, which
  creates it only when there is none: a look for the branch followed by
  `git worktree add -b` left a moment in which a branch somebody made would fail the
  add, and taking the add back would have deleted their branch. `git worktree add` that
  fails removes the checkout it could not finish, but the branch the claim made stays;
  rewake drops it while it is still at the commit and checked out nowhere, and removes
  the directory when it is empty and the record.
- **After the session** the checkout and its branch stay, as Codex's own would, and a
  line on stderr says where, and how to land, finish or remove it. A launch that fails
  after the checkout was made, or whose harness exits with an error — a resume of a
  conversation that does not exist, say — takes it back when it is still at its commit
  and rm without `--force` would remove it, and drops the branch the same way rm does.
  One it cannot take back stays, and the line says why — the reasons rm would give —
  where it is, that the launch command runs there without `--worktree` to go on, and how
  to land, finish or remove it.

## Launches at once

Several launches with `--worktree` in one repository make their checkouts one after
another, not side by side. With git 2.43 a `git worktree add` running beside another
can read the other's new entry in the Git directory with its `commondir` still empty,
and dies, leaving the launch to exit 1
([research-worktree.md](research-worktree.md#git-worktree-add-beside-another)). The
workflow suite's three `--worktree` launches at once met it in one run of 48.

Main decided on September 27, 2026 how: a lock per repository, held by the launch from
before its branch is made until the checkout, its copies and its record are in place.
The lock is an `flock` on `<repository>-<hash>.lock` beside the repository's directory
under the worktree root: not in the repository's Git directory, since rewake writes
nothing of its own into another program's metadata, and not inside the repository's own
directory, which goes with its last checkout. The file stays, one per repository, and
locks nothing once its holder has let go or died. `rewake worktree rm` takes the same
lock for its checks and the removal together, and finish for its checks, its landing
and the removal, and each reads the record again under it: `git worktree remove` reads
the other entries too, and a look taken before the lock could see a name a launch had
claimed and not yet checked out — missing, nothing to keep — and the removal after the
wait would take the checkout that launch had just made (found September 28, 2026). A
failed launch taking its checkout back looks and removes under the lock the same way.

The wait is bounded: after a minute the launch is refused with exit 1, naming the lock
and the process that took it last; nothing is made. A holder is that slow when it hangs,
or when it copies large ignored directories `.worktreeinclude` names: the copies stay
under the lock, so a parallel rm cannot take a checkout still being filled (main's
decision of September 27, 2026). The refusal says both, and to wait and run the command
again. A removal whose worktree root is gone takes no lock and does not make the root
again: nothing is made there without a launch making the root first.

A `git worktree add` rewake does not run — the person's, or Claude Code's own `-w` — is
outside the lock. An add of rewake's that meets such an entry half-written, which git
reports as `failed to read <common dir>/worktrees/<entry>/commondir`, is tried once
more after a fifth of a second: git has removed what it began of this checkout, and the
other entry is whole long before then. A second failure is reported as it is. The match
is on the path in git's message, not its words, which git translates. The other worktree
commands — land, finish, the checks of ls and rm — are not serialized with such an add,
and can fail the same way with git's message; they are then run again.

## Files git ignores: `.worktreeinclude`

A checkout holds what the commit holds, and a repository often needs a file git ignores
to run at all — a `.env`, a local configuration. A `.worktreeinclude` at the top of the
repository names such files, and after making the checkout rewake copies them in from
the checkout the launch came from. The format and the choice are Claude Code's
([research-worktree.md](research-worktree.md#worktreeinclude)), so one file serves a Claude
Code `-w` worktree and a rewake worktree of either harness alike:

- `.gitignore` syntax: `#` comments, `!` negation, a trailing `/` for directories, a
  leading or inner `/` anchoring at the top, `*`, `?`, `[...]` with git's twelve named
  classes inside — `[:alpha:]`, `[:digit:]` and the rest — and `**`. As in git, a
  bracket never matches a slash, so neither `a[/]b` nor `a[.-0]b` matches `a/b`. The
  file is the repository's content, a stranger's too, so a line that makes no pattern —
  a range running backwards such as `[z-a]`, a class git does not know such as
  `[:word:]` — is named on stderr and left out, and the other lines apply;
- only files git ignores are copied: a tracked file the pattern names is the commit's,
  and an untracked one git does not ignore is not copied either;
- a directory git ignores as a whole is not walked unless a pattern opens it — names it,
  names a path inside it, or has a literal part before its first wildcard that leads into
  it; `**/x` and patterns without a slash look only where git already lists files one by
  one;
- a regular file is copied, with its permissions, never linked; a symbolic link is
  skipped, as the 2.1.280 binary does, though the reference source would follow it; a
  file already in the checkout, or whose directory leads out of it, is not written; a
  file that cannot be copied is named on stderr and the launch goes on;
- without the file nothing is copied.

What was copied is in the record and named on the launch line. rm and finish do not
count those copies as work while they equal the source's files — compared by size, then
a block at a time, and a copy or source that is no longer a regular file is not equal; a
copy changed in the checkout, or a new ignored file beside them, counts as before. A
record whose branch is not its name, or that lists a copy outside its checkout, is no
record rewake wrote and is not listed.

The copy is full: listing a heavy directory such as `node_modules` copies every file in
it, which is slow and costly in space. A copy that skips links does not reproduce the
`node_modules` pnpm makes, which is links to its store; a Python virtual environment does
not carry over at all, since its scripts hold absolute paths. Owner decision, September
27, 2026: rewake neither carries heavy dependencies into a checkout nor installs them.
The agent installs them, or the preparation the repository itself describes, best with
a package manager that keeps a shared cache. rewake builds no preparation command, no
link to the source's dependencies and no shared directory keyed by a lock file's hash.
`.worktreeinclude` carries only what it lists, and without it nothing is copied. Codex's
own worktree carries nothing beyond the commit
([research-codex.md](research-codex.md#--worktree-with-a-remote-terminal)).

## Landing and finishing

`rewake worktree land <name>` fast-forwards a branch of the source to the worktree's
branch, and nothing else: the commits keep their hashes — no rebase, squash or merge
commit — and the worktree and its session are not touched, so it runs as often as there
is accepted work to take. The target is the branch checked out in the checkout the
worktree was made from, or `--into <branch>`; a source on a detached HEAD needs `--into`.
Checked out in the source, the target is moved there with `git merge --ff-only`, so the
files in that checkout follow and git refuses changes the merge would overwrite; its
reason is passed on with the advice to commit or stash them. Checked out nowhere, its
ref is moved with `git update-ref` against the value it was read at. Checked out in any
other checkout — the source compared with symbolic links resolved — it is refused with
the merge to run there: that checkout may be a worker's, the worktree's own among them
when its worker switched to the target, and a merge would change its files and HEAD
under it (decided September 27, 2026, after an acceptance run saw exactly that on Codex
0.155.1 and 0.157.1). A target a rebase or a bisect in any checkout is working on is
refused too, as git itself refuses moving such a branch: git lists that checkout as
detached, and the rebase's last step would fail. So is a target a rebase with
`--update-refs` (or `rebase.updateRefs`) will move at its end, which it lists in
`rebase-merge/update-refs`: moved under it, the rebase fails to update it. A target that moved on is refused —
rewake does not rewrite history — with the rebase to run in the worktree,
`git -C <worktree> rebase <target>`. Nothing new to take is not a refusal: it says so
and exits 0. The answer names how many commits moved and the target's old and new
commit; `--json` gives the same model.

`git merge` moves whatever branch its checkout has out when it runs, so a source that
switches branches while land runs would have that branch fast-forwarded instead. The
source is asked again right before the merge, and a source no longer on the target is
refused with nothing merged; after the merge the target must hold the worktree's tip,
and when it does not, land is refused naming the branch the source switched to, which
git moved, and its reflog to see where it was (found September 28, 2026).

The hooks land runs are the repository's code, and land waits for a git call running
them at most a minute — the time a launch waits for the lock finish holds through its
landing. Past it land is refused naming the hook still running, or the command git
started, and git is not stopped: a merge cut short between moving the files and moving
the branch would leave the checkout changed under a branch that did not move. git goes
on in a session of its own, writing its output to files rather than to pipes that
would close when rewake exits, and what it does may still land; land run again once it
has ended says what did.

A refusal over the state of the worktree or its repository exits 1: a branch,
repository or source gone, a source on a detached HEAD or on the worktree's own branch,
a target the repository lacks, another checkout holds or has moved on, a source that
switched branches or a hook still running. A wrong call exits 2: a worktree name that
names none, a branch name `git check-ref-format` refuses, `--into` the worktree's own
branch, `--into=` with no branch, which would otherwise land into the default target
(decided September 27, 2026; the source on the worktree's branch and `--into=` on
September 28).

`rewake worktree finish <name>` lands once more, then removes the worktree and its
branch. Everything that would stop it is asked first, under the repository's lock held
until the removal, and a refused finish lands and removes nothing: a rewake session
running in the worktree or a launch still starting one, changes, files git ignores
other than unchanged copies, commits only a detached HEAD holds, a worktree no longer
on its branch, a fast-forward that is not possible, and what `git worktree remove`
itself refuses — a worktree locked with `git worktree lock`, or one holding a submodule
checked out (until September 28, 2026 those two let finish land and then fail to
remove). Each exits 1 with its reason. finish has no `--force`: removing without those
checks is `rm --force`.

## Listing and removing

A command names a worktree by its name or by `<repository>/<name>`, as ls prints it.
Since a name may hold a slash, `rewake-3f9a1c/feat/login` could be read either way; the
full form is tried first — a repository's directory and a name in it that match the word
— and the bare name only when none does. The repository part is a directory rewake
names with a hash, so a word that matches one in full is meant that way, and every
worktree's full form names it alone, even when another worktree's name spells it.

`rewake worktree ls` lists the checkouts with their branch, their owner, the rewake
sessions still running in each, and whether removing one would lose anything; `--json`
gives the same model. A record whose path is not the directory of its own name beside it,
with `+` for its slashes, is not listed: rewake did not make that path, and rm would
remove it.
`rewake worktree rm <name>` — or `<repository>/<name>` when a name is in two
repositories — removes one through `git worktree remove`, so the repository forgets it
too. Its branch goes along only when another branch, tag or remote-tracking ref holds
the branch's commits and no checkout has it out; a branch with commits only it holds
stays, with or without `--force`, and rm says how to take them (`git merge --ff-only`)
or drop them (`git branch -D`). A branch another checkout has out stays too, and rm says
where; one already deleted, or gone with its repository, is named as gone, with no
advice to merge it. Without `--force` it never loses work, and when it cannot tell, it
refuses. The checks and the removal hold the repository's lock together
([launches at once](#launches-at-once)). It refuses, naming each reason:

- changes `git status` shows;
- files git ignores — a `.env`, a local build — which `git worktree remove` deletes
  without a word, forced or not (checked on git 2.43, September 28, 2026); unchanged
  copies `.worktreeinclude` made are not counted;
- a HEAD no branch, tag or remote-tracking ref holds, asked every time: the worktree's
  own branch holds its commits while HEAD is on it, but a checkout switched to a
  detached HEAD, or whose branch was deleted, holds commits nothing else does;
- a launch that made it and has not yet registered its session, while that rewake
  process runs;
- a lock from `git worktree lock`, named with its reason, and `git worktree unlock` to
  lift it; a submodule checked out in it. `git worktree remove` refuses both;
- a rewake session still running in it: the one it was made for, or any whose working
  directory is in the checkout — started there by hand after the first ended, say — in
  any room of the current state directory and of the one the owner registered in. A
  process rewake did not start is not seen, and neither is a rewake session started
  from another directory with `-C <checkout>`: the working directory it registers is
  the one its wrapper started in, not the target of `-C`. Its work is not lost —
  `git worktree remove` refuses a checkout with changes — but a clean checkout goes from
  under it. Recording the directory a session actually works in is queued
  ([work-queue.md](work-queue.md#also-queued-not-scheduled));
- a directory gone while the repository still lists it: it may have been moved with its
  work, and `git worktree repair <new path>` run in the repository reconnects it. With
  `--force` its own entry is removed with `git worktree remove`, never `git worktree
  prune`, which would take every other missing checkout of the repository along. One
  the repository no longer lists either leaves only the record, which rm removes;
- a directory whose repository is gone: git has nothing left to say of what it holds.
  With `--force` the directory and its record go. One whose directory is gone as well
  leaves only the record, which rm removes.

`--force` removes it anyway, a locked one too: git needs `--force` twice for that, and
rm passes it twice. Looking never stands in a session's way: git runs with
`GIT_OPTIONAL_LOCKS=0`, so `git status` does not take `index.lock` from under a commit
the session is making. Nothing is removed automatically. rm's refusals exit 1, as
finish's and land's over the same state do; a worktree name that names none exits 2.
Until September 27, 2026 they exited 2, fixed before finish existed; the owner decided
that day that a refusal over state exits 1 in every worktree command.

A main's `--grant-git` reaches such a checkout as it does a worktree made by hand: the
grant reads the conversation's cwd, which is in the checkout, and resolves its private
and common Git directories
([research-permissions.md](research-permissions.md#managed-worktrees-and-continuation-permissions)).
