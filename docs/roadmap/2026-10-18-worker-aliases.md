# Worker aliases and pi worktrees

Owner decision, October 18, 2026: launch aliases also describe main's worker choices.
Added optional `main`, one-line `description`, and `launch` model/effort/context/apply fields.
`workers` is a view with marked mains under `--all`; `space open --alias` uses the wrapper,
role, args and applied settings, but never a main alias; space identity stays main's.
Fixed pi's fresh `--session-id` being mistaken for continuation beside `--worktree`.
Build, vet, lint, focused tests, full-tree race/shuffle and the alias-wrapper workflow pass.
Open: one review and main's live wrapper/worker-settings and pi-worktree acceptance.
