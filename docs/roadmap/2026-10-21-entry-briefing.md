# A worker that may enter a checkout is told it starts in main's

October 21, 2026. **Done.** A worker of a template with `enter_worktree = true` never called
`worktree_enter`, ran `git switch -c` in main's checkout, and main's merge and push went to the
wrong branch: nothing told it whose checkout it sat in. A run that may enter now gets the
briefing section YOUR CHECKOUT (`entryBrief`, Claude Code's adapter) — no change to main's
checkout, `worktree_enter` before the first change, main told if it fails — and the tool's text
says the same; no guard on the edit tools, as the live case was a shell command. Live on the
cheapest model: an edit task and a "create branch feature-x" task each entered a worktree first
and committed there, main's checkout clean on its branch. **Open.** Nothing enforces it.
