# The close drops the branch the worker switched to

October 21, 2026. **Done.** `worktree.Release` also drops the branch the checkout was on at
removal when it is not the record's `wt-*` branch (`git switch -c spacehost-nits`), by
`DropBranch` itself: deleted when another ref holds its tip and no checkout has it out. A
switched branch with unmerged work stays and the checkout still goes (its HEAD is held by that
branch); `Release` then answers a note, which the space host adds to the end's reason ("; its
branch X stays: its commits are on no other branch"), so `space close` and `space list` show it.
`rm`/`finish` are other paths, unchanged. **Open:** nothing.
