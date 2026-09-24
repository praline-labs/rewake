# A Claude Code session's context window, as the person set it — September 24, 2026

The owner saw workers that run under `CLAUDE_CODE_AUTO_COMPACT_WINDOW=300000`, set in
the `env` block of their settings, listed by `rewake list` as `… / 1000K`, with the percent
of a million. The harness compacts those sessions against 300K; the listing showed the
model's window, which they never reach.

**The research.** review-claude read the 2.1.280 binary and probed it live against a
stand-in API the same morning
([research.md](../research.md#telemetry-sources-the-status-line-and-hooks)):

- The harness takes the window from the variable, then the `--autocompact` flag, then
  the `autoCompactWindow` settings key, then remote configuration, an experiment and the
  model's default, and reads it again for every request.
- The settings `env` block wins over the process environment; an edit of the file
  mid-session takes effect on the next turn.
- Neither the status line nor a plugin's `session.measure` carries the result: both
  report the model's window and a percent of it.
- A function-hooks module's `$.env.get` returns exactly what the harness uses, and
  `$.settings.read()` returns the key; the flag is visible to no plugin.
- Compaction fires at `window − min(maxOutput, 20000) − 13000`: 267K, 89% of a 300K
  window, seen live between 266K and 268K.
- A module that uses `$` inside a logical expression does not load
  ([research-claude-control.md](../research-claude-control.md#what-a-function-hooks-plugin-hears)).

**What was built** (`de44dda`, [claude-telemetry.md](../claude-telemetry.md)):

- rewake's plugin reads the variable and the key at the session's start and at every
  measure of the context, and sends the two raw values with `plugin.ready` and
  `session.measure`; nothing else of the environment or the settings leaves the module.
- The wrapper takes the last `--autocompact` from the launch's own arguments.
- The collector reads each value by the harness's rules and in its order, and where the
  result is below the model's window the snapshot shows it as the window, with the
  percent computed against it. The tokens stay the status line's; the newest report
  decides.
- Without any of the three, and for a session without the plugin, the numbers are the
  status line's, as before.
- Tests: parsing, bounds and order in unit tests; the module under node with a strict
  host, secrets in its environment and settings that must not be sent; the
  `claude-telemetry` workflow case now runs the plugin with a 150K window against a 200K
  status line and must read 33% of 150K, and a fourth mutant, `model-window`, puts the
  model's window back and breaks exactly the listing and the header
  ([testing.md](../testing.md#claude-code-telemetry-budgets)).

**The review.** review-claude ran the built tree against the real 2.1.280 in a private
HOME with a stand-in API: a settings `env` of 300000 gave `17% / 300K` in a main's
`rewake list`, and eleven variants — the variable in the settings env, none, the key,
the variable with the flag, the key with the flag, the flag's `auto` over the key,
`300k`, the process environment alone and under a settings env, 2000000, and a zero
variable over the key — all matched the harness's own window. The module loaded, and
the measure handler took 14–20 ms. Its findings, fixed before the commit:

- The count parser was looser than the harness's: `0.5`, `.5` and `1.5e-1` read as
  100000 where the harness ignores them, `1.234567e5` as 123456, `300 000` with a space
  or a no-break space as 100000, `3,00000` as 300000 and `300,000_000` as 1000000. It is
  now a one-to-one port of the harness's functions, with those cases in the unit tests,
  and was checked against the harness's own code on 200064 inputs with no difference.
  The flag's `150000.5` now reads as 150000, as the harness's `parseInt` does.
- A read that threw was sent as "no variable". Now a failed read sends no limit at all,
  and the last one heard stands; a node test fails each read in turn.
- The telemetry case now needs node, so the plugin-less path lost its end-to-end
  coverage; accepted, and named in testing.md.
- Wording in four documents: what `$.settings.read()` was seen returning, the sources
  after the key, the compaction point as a design choice, and HF-11's live evidence.

**The owner's decisions**, September 24, 2026:

- The listing shows the person's limit as the window, with the percent of it.
- The value is read from the harness's own sources, never assumed.
- The owner does not change the value in the middle of a conversation; reading it at
  every measure costs nothing and follows an edit anyway.

Not showing the compaction point was main's design choice in the brief, not the owner's:
it depends on the model's output limit, which rewake does not know, and computing it
would copy the harness's arithmetic.

**What stays open.**

- No workflow case plays a session without the plugin end to end: its model window is
  covered by unit tests only, and its `interruptions unheard` header by unit tests and
  claude-interrupted's `unobserved` listing.
- A percent above 100 is shown as it is, not clamped.
- The compaction point is not shown; the 89% fact is in research.md only.
