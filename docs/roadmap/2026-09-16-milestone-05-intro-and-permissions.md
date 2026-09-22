# Milestone 5. Intro and permissions — done, September 16, 2026

- The intro for both harnesses, plus the `--no-intro` flag.
- `--allowedTools "Bash(rewake:*)"` for Claude Code.
- Verify that the flag adds to the user's rules rather than replacing them; if
  it replaces them, drop the flag and have the overview name the rule to add
  once.

**Live criterion:** an agent launched through the wrapper, asked "who are you
in rewake", answers with its own name, and runs `rewake send` without
confirmation. For Codex, the same question in one cheap turn.

Met on September 16, 2026, as a side effect of the milestone 3 and 4 runs: both
agents answered through `rewake send` without being told how and without a
confirmation prompt, the Codex one from inside its sandbox.
