# The close drops the branch the worker switched to

October 21, 2026. **Done.** `worktree.Release` also drops the branch the checkout was on at
removal when it is not the record's `wt-*` branch (`git switch -c spacehost-nits`), by
`DropBranch` itself: deleted when another ref holds its tip and no checkout has it out. The
checkout's branch is read in the same look under the lock (`closeReasons` now returns the
check). A switched branch with unmerged work stays and the checkout still goes: its HEAD is held
by that branch, a local branch other than the record's. `rm`/`finish` are other paths and are
unchanged. **Open:** nothing.
