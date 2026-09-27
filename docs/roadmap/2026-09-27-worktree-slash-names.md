# Worktree names with a slash — September 27, 2026

The owner decided on September 27, 2026 that a worktree's name may hold a slash, as a
branch's does: `--worktree=feat/super-feature`. Until then a name was a letter or digit
and up to 39 letters, digits, `-` or `_`, one path element by construction.

## What was built

- **The name** is the branch's, exactly: `feat/super-feature` makes the branch
  `feat/super-feature`. It is a name `git check-ref-format --branch` takes, written in
  ASCII letters, digits, `.`, `_`, `-` and `/`; not `HEAD` or forty hex digits, as
  before; not ending in `.json`; at most 250 bytes
  ([worktree.md](../worktree.md#the-launch)).
- **The characters** are Claude Code's for its own worktree names
  ([research-worktree.md](../research-worktree.md#names-with-a-slash), read in the
  reference source) rather than all git takes. The brief proposed git's rules with `+`
  forbidden; git also takes `;`, `$`, quotes, `!`, `&` and non-ASCII letters, and a
  refusal prints the name inside a command to copy — `rewake worktree rm <name>
  --force` — which a shell would read otherwise. The narrower set leaves `+` out as
  well.
- **The length** is no longer 40: 250 bytes is the longest name whose record,
  `<name>.json`, and git's `.lock` beside a branch's last element fit the 255 bytes a
  file name may take.
- **The directory and the record** write each slash as `+`, as Claude Code does:
  `feat+super-feature/` and `feat+super-feature.json`, every checkout one directory
  beside the others. Nested directories were the alternative and were not taken: the
  checkout `feat/login` would lie inside the checkout `feat`, whose removal deletes it,
  and its record among `feat`'s files. No name holds `+`, so two names never share a
  directory, and a name ending in `.json` is refused, since its directory would be
  another name's record. Records from before carry names with no slash, whose
  directory is the name itself: nothing to tell apart, no legacy mark.
- **Naming a worktree.** land, finish, rm and ls take and print the name with its
  slashes. `<repository>/<name>` is now ambiguous — `rewake-3f9a1c/feat/login` — and
  is read as the full form first, as a bare name only when no repository's directory
  and name match it. The repository part is a directory rewake names with a hash, so a
  word matching one in full is meant that way, and every worktree's full form names it
  alone even when another's name spells it. Rejected: refusing names whose first
  element looks like a repository's directory, which forbids what only matters when
  it collides.
- **A branch in the way, both directions.** `fix` beside `fix/login` was already
  refused as a taken branch naming `fix/login`; `feat/login` beside a branch `feat` now
  is too, naming `feat`, where git's raw `cannot lock ref` came out before.
- The record's invariants stand: its branch is its name, each included path lies within
  the checkout, and a record whose checkout is not its name's directory beside it is
  not listed — the name's directory now being the one with `+`.

## Tests

In `internal/worktree`: `git check-ref-format --branch` as the judge of `ValidName`
over a table and 300 random names of the characters its rules are about; the names
rewake refuses beyond git and the ones the old rule refused that pass now; a slash name
made, found both ways, on its branch and landed; the refusal in both directions; the
full form winning over a name that spells it, and a record elsewhere than its flattened
name left out. In `internal/cli`, a launch with `--worktree=feat/super-feature`, then
ls, land, a launch of a name below its branch refused, and rm by the full form. Each
fails without its part of the change: the flattening, the refusal above, the order of
reading, the characters, `.json` in any case, the `.lock` rule and the directory check
were each undone in turn.

## What stays open

Nothing.
