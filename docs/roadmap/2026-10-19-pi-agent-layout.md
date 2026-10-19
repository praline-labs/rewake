# The bundled pi's agent folders

Owner decision, October 19, 2026: make it visible where pi plugins and skills go.

- A pi launch makes `extensions/` and `skills/` (0700) in `~/.config/rewake/pi/agent` when
  missing and touches nothing in them (`agentDir`, `internal/adapter/pi/ship.go`).
- `docs/launch.md` and `docs/research-pi.md` say how an extension and a skill are added.
- Open: not run live with a pi; the unit test covers creation and no-overwrite.
