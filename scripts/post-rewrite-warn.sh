#!/bin/sh
# The post-rewrite job of lefthook.yml: git cannot refuse a rewrite after it is done,
# so this tells the agent that it broke the owner's rule. $1 is "amend" or "rebase".
cat >&2 <<TEXT
!!! WARNING: a commit was rewritten ($1). !!!
The owner forbids any rewrite of a branch, pushed or not: no amend, no rebase, no reset
that drops a commit. Report it to main now; fix a mistake in a new commit.
TEXT
