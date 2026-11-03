// A worker's entry into a checkout of its own (docs/worktree-tools.md): in a
// run whose template says enter_worktree = true (REWAKE_WORKTREE_ENTER),
// rewake's tool worktree_enter (tools.ts) has rewake make and claim the
// worker's one checkout, then moves the session into it with Claude Code's
// EnterWorktree {path}, so the space's close finds and releases it as a
// launch's own. The model's own EnterWorktree is refused there: it would make
// a checkout rewake does not know. Main's runs, and workers without entry,
// are not touched. These are the mod's checks at the call, so they stay here.
import type { Register } from "claude-code";

export const guardRefusal =
  "use the rewake tool worktree_enter instead: rewake makes this worker's worktree, records it and removes it at the close; one Claude Code makes itself is unknown to rewake.";

// guardEnterWorktree guards EnterWorktree and lets rewake's own move pass.
export function guardEnterWorktree(on: Parameters<Register>[0]) {
  // Only rewake's own call passes in a run that may enter, and a guard that
  // fails refuses before the tool ran: fail closed. Elsewhere — main, a
  // worker whose deny list hides the tool — the call goes on untouched.
  on("tool.call", { tool: "EnterWorktree" }, async ($, e, next) => {
    if ((await $.env.get("REWAKE_WORKTREE_ENTER")) !== "1" || next.origin.plugin === "rewake")
      return next(e);
    return { deny: guardRefusal };
  }).catch(($, e, next) => (next.called ? next(e) : { deny: guardRefusal }));

  // Claude Code asks a person before it moves a session into a checkout
  // outside .claude/worktrees, whatever the allow rules say (seen live on
  // 2.1.302); nobody may be at a worker's screen. rewake made this one for
  // this worker a moment ago, so that ask of the engine's own is answered
  // for rewake's call. Every other verdict stands: a deny, and an ask a
  // PreToolUse hook, a rule, a setting or another plugin made (2.1.304 marks
  // that with `plugin`), are the person's policy; the
  // model's call, and every call elsewhere, keep the engine's verdict whole.
  on("tool.check", { tool: "EnterWorktree" }, async ($, e, next) => {
    const verdict = await next(e);
    // `plugin` is not in 2.1.302's verdict type, so it is read through a widened one.
    const { plugin } = verdict as { plugin?: string };
    const own =
      verdict.hook === undefined &&
      verdict.rule === undefined &&
      verdict.setting === undefined &&
      plugin === undefined;
    if (verdict.decision !== "ask" || !own || next.origin.plugin !== "rewake") return verdict;
    if ((await $.env.get("REWAKE_WORKTREE_ENTER")) !== "1") return verdict;
    return { decision: "allow", reason: entryAllowed };
  });
}

export const entryAllowed = "rewake made this worktree for this worker";
