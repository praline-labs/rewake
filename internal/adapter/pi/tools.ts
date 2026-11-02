// The session tools, served through pi: the worktree tools (worktree_status,
// every run; worktree_list and worktree_land, main's) and the brief tools
// (brief_read, brief_tick, brief_comment, every run but main's). Each is
// defined once, in sessiontools.ts, a copy of the module Claude Code's mod
// serves the same tools from; what stays here is pi's bridge. A tool runs its
// `rewake …` command by argv through the rewake that started the run
// (REWAKE_EXECUTABLE): pi's exec spawns without a shell, and its child
// inherits this process's environment, the run's REWAKE_DIR, room, session
// and epoch among it (docs/worktree-tools.md, docs/briefs.md).
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import {
  briefCommentTool,
  briefReadTool,
  briefTickTool,
  type CommandTool,
  didNotEnd,
  type Outcome,
  outcome,
  worktreeLandTool,
  worktreeListTool,
  worktreeStatusTool,
} from "./sessiontools.ts";

type Ran = { stdout: string; stderr: string; code: number; killed: boolean };

// answer is the tool's result from what pi says of the command. pi reports a
// child it killed at the timeout with code 0, so killed is read first.
export function answer(ran: Ran, tool: CommandTool): Outcome {
  if (ran.killed) {
    return didNotEnd(tool.what, `stopped after ${tool.timeoutMs} ms or by an abort`);
  }
  return outcome(tool.what, ran.code, ran.stdout, ran.stderr);
}

// serve registers one tool, answered by its command in the session's directory.
function serve(pi: ExtensionAPI, executable: string, tool: CommandTool) {
  pi.registerTool({
    name: tool.name,
    label: tool.label,
    description: tool.description,
    parameters: tool.inputSchema as never,
    async execute(_id, params, signal, _onUpdate, ctx) {
      let result: Outcome;
      const args = tool.args(params as Record<string, unknown>);
      if (typeof args === "string") {
        result = { isError: true, text: args };
      } else {
        try {
          const ran = await pi.exec(executable, args, {
            cwd: ctx.cwd,
            signal,
            timeout: tool.timeoutMs,
          });
          result = answer(ran, tool);
        } catch (error) {
          result = didNotEnd(tool.what, error);
        }
      }
      return {
        content: [{ type: "text", text: result.text }],
        details: undefined,
        isError: result.isError,
      };
    },
  });
}

// worktreeTools registers the worktree tools a run is offered, in a run whose
// rewake names itself; a pi started otherwise has no rewake to ask. Main's two
// choose what a session is shown, not who may run the commands behind them.
export function worktreeTools(
  pi: ExtensionAPI,
  executable = process.env.REWAKE_EXECUTABLE,
  role = process.env.REWAKE_ROLE,
) {
  if (!executable) return;
  const offered =
    role === "main"
      ? [worktreeStatusTool, worktreeListTool, worktreeLandTool]
      : [worktreeStatusTool];
  for (const tool of offered) serve(pi, executable, tool);
}

// briefTools registers the brief tools for every run but main's, which uses
// the commands. This chooses what a session is shown, not what it may do.
export function briefTools(
  pi: ExtensionAPI,
  executable = process.env.REWAKE_EXECUTABLE,
  role = process.env.REWAKE_ROLE,
) {
  if (!executable || role === "main") return;
  for (const tool of [briefReadTool, briefTickTool, briefCommentTool]) serve(pi, executable, tool);
}
