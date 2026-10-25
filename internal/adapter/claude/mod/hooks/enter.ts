// A worker's entry into a checkout of its own (docs/worktree-tools.md): in a
// run whose template says enter_worktree = true (REWAKE_WORKTREE_ENTER), the
// tool worktree_enter has rewake make and claim the worker's one checkout,
// then moves the session into it with Claude Code's EnterWorktree {path}, so
// the space's close finds and releases it as a launch's own. The model's own
// EnterWorktree is refused there: it would make a checkout rewake does not
// know. Main's runs, and workers without entry, are not touched.
import type { Register } from "claude-code";
import { type Answer, answer, runEnv, unrun } from "./worktree.ts";

export const worktreeEnterTool = {
  name: "worktree_enter",
  description:
    "Move this session into a git worktree of its own, which rewake makes from the commit the session is at, on a new branch, and removes at this worker's close when nothing in it would be lost (landed or unchanged); otherwise the close keeps it and says why. This session starts in main's own checkout, which must not be changed: call this before your first change to any file of the repository, and before any commit or branch change, whether or not main's task mentions a worktree. One worktree per worker: called again it answers the same one, and there is no way back to the launch directory. name is optional, the worktree's and its branch's name; leave it out for a generated one.",
  inputSchema: {
    type: "object",
    properties: {
      name: {
        type: "string",
        description: "The worktree's name, also its branch's; leave out for a generated one.",
      },
    },
    additionalProperties: false,
  },
};
export const worktreeEnterToolName = "mcp__rewake__" + worktreeEnterTool.name;

// enterTimeoutMs bounds git worktree add and the copies .worktreeinclude asks
// for; past it the call is reported as failed.
export const enterTimeoutMs = 120_000;

// enterArgv is the command, or why the input cannot make one: a name that is
// empty or begins with a dash would be read as a flag.
export function enterArgv(executable: string, input: Record<string, unknown>): string[] | string {
  const { name } = input;
  if (name === undefined) return [executable, "worktree", "enter", "--json"];
  if (typeof name !== "string" || name === "" || name.startsWith("-")) {
    return "name must be a worktree name; leave it out for a generated one";
  }
  return [executable, "worktree", "enter", name, "--json"];
}

// within says whether dir is path or lies in it.
export function within(dir: string, path: string): boolean {
  const base = path.replace(/\/+$/, "");
  return dir === base || dir.startsWith(base + "/");
}

export const guardRefusal =
  "use the rewake tool worktree_enter instead: rewake makes this worker's worktree, records it and removes it at the close; one Claude Code makes itself is unknown to rewake.";

// serveWorktreeEnter answers worktree_enter and guards EnterWorktree.
export function serveWorktreeEnter(on: Parameters<Register>[0]) {
  on("tool.call", { tool: worktreeEnterToolName }, async ($, e) => {
    const executable = await $.env.get("REWAKE_EXECUTABLE");
    if (!executable) {
      return {
        isError: true,
        result: "this session was not started by a rewake that sets REWAKE_EXECUTABLE",
      };
    }
    const command = enterArgv(executable, e as unknown as Record<string, unknown>);
    if (typeof command === "string") return { isError: true, result: command };
    const env = runEnv({
      REWAKE_DIR: await $.env.get("REWAKE_DIR"),
      REWAKE_ROOM: await $.env.get("REWAKE_ROOM"),
      REWAKE_SESSION: await $.env.get("REWAKE_SESSION"),
      REWAKE_EPOCH: await $.env.get("REWAKE_EPOCH"),
      REWAKE_WORKTREE_ENTER: await $.env.get("REWAKE_WORKTREE_ENTER"),
    });
    let made: Answer;
    try {
      const ran = await $.process.run(command, {
        cwd: await $.session.cwd(),
        env,
        timeoutMs: enterTimeoutMs,
      });
      made = answer(ran, "rewake worktree enter");
    } catch (error) {
      return unrun(error, "rewake worktree enter");
    }
    if ("isError" in made) return made;
    let path: string;
    try {
      const entry = JSON.parse(made.result);
      path = entry.checkout.path;
      if (typeof path !== "string" || path === "") throw new Error("no checkout path");
    } catch (error) {
      return unrun(error, "reading rewake worktree enter's answer");
    }
    if (within(await $.session.root(), path)) {
      return { result: made.result + "\nThis session already works in it." };
    }
    // The checkout is this run's from here: a failed entry leaves it claimed,
    // a later call answers the same one, and the close releases it.
    const stays = `The worktree at ${path} is this worker's and stays; call worktree_enter again to enter it.`;
    try {
      const moved = await $.tool.call({ tool: "EnterWorktree", path });
      if (moved.deny !== undefined) {
        return {
          isError: true,
          result: `Claude Code refused to enter it: ${moved.deny}. ${stays}`,
        };
      }
      if (moved.isError) {
        return {
          isError: true,
          result: `Claude Code could not enter it: ${moved.text ?? ""}. ${stays}`,
        };
      }
      return { result: made.result + "\n" + (moved.text ?? "Entered " + path) };
    } catch (error) {
      return unrun(error, `entering ${path} (${stays})`);
    }
  }).catch((_$, _e, next) => ({
    isError: true,
    result: `worktree_enter failed: ${next.error.message}`,
  }));

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
