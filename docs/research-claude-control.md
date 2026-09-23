# Research: steering a running Claude Code session from outside

Split from [research.md](research.md) by subject on September 23, 2026: what reaches a
running Claude Code session besides a message, and what it lets rewake hear back — an
interruption, the session's own commands, text put into its input box. The Codex side of
the same questions is in [research-protocol.md](research-protocol.md#compaction-and-the-terminals-commands-over-the-protocol).

Tags, with the version in each section: **[live]** — observed in a private HOME against
a fake API on a local port, keys typed through tmux; **[source]** — read in the installed
binary's bundled source; **[docs]** — the official pages. An older source tree of the
harness served only as a map of where to look in the binary; no fact below rests on it.

## Interrupting a turn and changing the conversation

Probed on September 23, 2026, Claude Code 2.1.280, in a private HOME against a fake API
on a local port, with Esc and Ctrl+C typed through tmux **[live]** and the abort path
read in the binary **[source]**.

An interrupted turn is marked by nothing rewake can hear (HF-06):

- Esc while the request hangs, while text streams, and during a Bash tool: no `Stop`,
  `StopFailure`, `Notification` or `PostToolUseFailure`. No hook, no status-line field
  and no key of a later `UserPromptSubmit` says the turn was interrupted.
- `idle_prompt` did not come within 80 seconds after an Esc; after an ordinary `Stop`
  it came 61.6 seconds later.
- The only marker, `[Request interrupted by user]`, is in the next request's body — the
  transcript, which rewake does not read.
- Ctrl+C once or twice during a turn behaves like Esc. Twice while idle ends the
  session: `SessionEnd` with reason `prompt_input_exit`, and the wrapper exits 0.
- In the binary the abort branches return `aborted_streaming` or `aborted_tools` and
  never call the Stop hook, whose callers are `blockable_turn_end`,
  `turn_end_reactions` and `loop_tick`. A `terminal_reason` of `aborted_*` exists only
  in the SDK's stream-json output. **[source]**
- The hooks reference says the same of `Stop`: "Does not run if the stoppage occurred
  due to a user interrupt." A hook for an interruption is an open feature request,
  anthropics/claude-code#9516. **[docs]**

Changing the conversation (HF-19):

- `/clear`: `SessionEnd` with reason `clear` and the old `session_id`, then
  `SessionStart` with source `clear` and a new one, without a `model` key; a status
  line with the new id about 300 ms later.
- `/resume <id>`: `SessionEnd` with reason `resume`, then `SessionStart` with source
  `resume` and the resumed conversation's original id, with extra keys —
  `context_tokens`, `seconds_since_last_response`, `prompt_cache_likely_expired`,
  `estimated_cache_write_usd`, `model`, `prompt_id`. The picker behaves the same.
- The wrapper's socket survives both, and a delivery and its `Stop` work after each.
- Not checked: `--resume` or `--continue` at launch through rewake, and resuming a
  conversation from another project or one started outside rewake.

### A second probe: signals and hooks around an interruption

Probed on September 23, 2026, Claude Code 2.1.280, same setup **[live]**. The question
was whether anything short of the transcript reaches the wrapper or a hook process at
the moment of an interruption.

- **The wrapper gets no SIGINT.** Ctrl+C in the running terminal UI, once or twice,
  during a turn or idle, never reached the wrapper as a signal. `stty` showed `-isig`
  in both states: the UI holds the terminal raw, so Ctrl+C arrives as a key, not as a
  signal to the foreground process group. Proven with a logging build of the wrapper
  whose SIGINT-dropping goroutine wrote a line per signal; `kill -INT` from outside
  produced the line, Ctrl+C never did. Esc likewise reaches only the UI.
- **An async `UserPromptSubmit` hook outlives everything.** Its process survived `Stop`,
  Esc, Ctrl+C and the end of the session itself: seven instances, all exiting normally
  after their full 150 seconds. Nothing is sent to it, so its fate says nothing about
  the turn.
- **A sync `UserPromptSubmit` hook sees an interruption, but only of its own phase.**
  It blocks the turn — the UI shows `running UserPromptSubmit hooks…` and the request
  is not sent until the hook ends — and an interruption during it sends it SIGTERM,
  72 ms after Esc and 14 ms after Ctrl+C. Once it has returned, the request and the
  tools that follow are outside its reach, and blocking every turn behind a hook is a
  cost of its own.

So no immediate interruption signal is reachable by rewake within its boundaries.
Catching Esc in the wrapper would mean reading the keyboard between the person and the
harness — a pseudo-terminal proxy, which the project's boundaries exclude
([AGENTS.md](../AGENTS.md#boundaries)).

## Commands from outside

Probed on September 23, 2026, Claude Code 2.1.280, same setup **[live]**, with the
queueing path read in the binary **[source]**.

- **The inbound socket never runs a slash command.** Every line of type `user` is
  queued with `skipSlashCommands`, `isMeta` and `skipAttachments` set, whatever its
  priority and whatever the permission mode **[source]**. `/compact`, `/clear`,
  `/model`, `/effort`, `/status`, `/cost`, `/context`, `/config`, a custom command and
  a skill each reached the model as plain text, both between turns and mid-turn
  **[live]**.
- **The socket's control actions** are `rename`, `peer_message_status`,
  `notify_when_idle`, `peer_idle_notice` and three actions for replies to artifacts;
  none of them compacts, clears, switches a model or runs a command **[source]**.
- **A typed `/compact`**, for comparison, gives `PreCompact` with trigger `manual`,
  `SessionStart` with source `compact`, then `PostCompact` **[live]** — the same
  sequence as in [research.md](research.md#telemetry-sources-the-status-line-and-hooks).

## Putting text into the input box

Probed on September 23, 2026, Claude Code 2.1.280 **[live]** and in the binary
**[source]**. The question was whether rewake can prepare a command — `/compact` with
instructions, say — for the person to confirm with Enter, since it cannot run one.

- **At launch**: the flag `--prefill`, see
  [research-launch.md](research-launch.md#a-prompt-draft-at-launch). First prompt only.
- **A function-hooks plugin.** Enabled by `CLAUDE_CODE_ENABLE_FUNCTION_HOOKS=1`, loaded
  from `--plugin-dir` or `CLAUDE_CODE_PLUGIN_DIRS`, only in a trusted workspace and not
  under `--bare` or `disableAllHooks`; the API is marked early access **[source]**. A
  handler on `session.receive` can call:
  - `$.prompt.fill` — puts text into the input box without submitting. Verified: `/compact
    keep only the plan` appeared in the input, no turn ran, and Enter gave a real manual
    compaction with those instructions **[live]**. Mode `replace` silently overwrote text
    the person had typed, so a handler reads the box first or appends.
  - `$.prompt.suggest` — a grey suggestion, shown only when the input is empty and the
    session idle; Tab takes it, Enter sends it **[live]**.
  - `$.session.compact` and `$.command.run` exist in the declarations and would act
    without an Enter. Not tested live: the probe was refused by the auto-mode
    classifier **[source]**.
- **Not usable for a draft**: the socket's envelope and control frames; a command hook's
  output — `SessionStart`'s `initialUserMessage` submits at once, and only in print or
  SDK mode; the built-in prompt suggestions, which nothing outside feeds; Remote
  Control; deep links, which open a new session **[source]**.
