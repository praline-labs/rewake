# A worker's checkout removed at its close when nothing would be lost

October 20, 2026, by the owner's decision of that day (a room held 18 checkouts of merged work).
**Done.** After the worker's process group is seen gone, every close of a space — `rewake space
close`, main's end, the worker's own exit — removes its checkout and branch under the
repository's lock when rm's checks find nothing and every commit at its HEAD and branch tip is on
another local branch (`worktree.Release`, SP7). A kept one adds why to the end's reason; the close's
answer and `space list` show it; an assume-unchanged or skip-worktree mark keeps one, for rm too;
ignored files no longer keep one at the close, rm still does (the owner: 14 of 17 kept only for
them). Live with haiku workers: untouched and merged removed, unmerged and uncommitted kept. **Open.** The owner's existing checkouts (main, with `rewake worktree rm`).
