// The worktree tools: the model asks which checkout its session works in
// (worktree_status, every session), and main lists its room's checkouts and
// lands one (worktree_list, worktree_land). Each is defined once, in
// sessiontools.ts, a copy of the module rewake's pi extension serves the same
// tools from; what stays here is the mod's bridge, which the brief tools and
// worktree_enter use too: the answer is a `rewake …` command run by argv
// through the rewake that started the run (REWAKE_EXECUTABLE), no shell, the
// run's variables passed explicitly, and a cut answer an error
// (docs/worktree-tools.md).
import type { Register } from "claude-code";
import {
  type CommandTool,
  didNotEnd,
  outcome,
  worktreeLandTool,
  worktreeListTool,
  worktreeStatusTool,
} from "./sessiontools.ts";

// toolName is a tool's name as the model sees it: mcp__rewake__worktree_status
// and so on.
export const toolName = (tool: CommandTool) => "mcp__rewake__" + tool.name;
export const worktreeToolName = toolName(worktreeStatusTool);
export const worktreeListToolName = toolName(worktreeListTool);
export const worktreeLandToolName = toolName(worktreeLandTool);

// registration is what $.tool.register takes of a tool.
export function registration(tool: CommandTool) {
  return { name: tool.name, description: tool.description, inputSchema: tool.inputSchema };
}

// The tools a role is offered. Main's two change nothing about who may run the
// commands behind them: this chooses what a session is shown, not what it may do.
export function worktreeToolsFor(role: string | undefined) {
  const offered =
    role === "main"
      ? [worktreeStatusTool, worktreeListTool, worktreeLandTool]
      : [worktreeStatusTool];
  return offered.map(registration);
}

// Ran is what the harness says of a finished command.
export interface Ran {
  exitCode: number | null;
  stdout: string;
  stderr: string;
  isStdoutTruncated?: boolean;
  isStderrTruncated?: boolean;
}

export type Answer = { result: string } | { isError: true; result: string };

// answer is the tool's result: the command's JSON when it succeeded whole,
// otherwise an error that says what went wrong. A cut answer is an error, not
// a shorter answer. what names the command in the error.
export function answer(ran: Ran, what = "rewake worktree status"): Answer {
  const ended = outcome(what, ran.exitCode, ran.stdout, ran.stderr);
  if (ended.isError) return { isError: true, result: ended.text };
  if (ran.isStdoutTruncated) {
    return {
      isError: true,
      result: `${what} answered more than the harness keeps; its answer was cut`,
    };
  }
  return { result: ended.text };
}

// unrun is the error of a command that did not run to its end: it could not
// start, or ran past the bound.
export function unrun(error: unknown, what = "rewake worktree status"): Answer {
  return { isError: true, result: didNotEnd(what, error).text };
}

// runEnv is the run's variables that are set, passed on as they are: the
// state directory, the room, and the session and run the command answers for.
export function runEnv(values: Record<string, string | undefined>): Record<string, string> {
  const env: Record<string, string> = {};
  for (const [name, value] of Object.entries(values)) {
    if (value) env[name] = value;
  }
  return env;
}

// serveCommand answers one tool's calls with its rewake command. The command's
// environment is the run's own variables, set over the harness's environment,
// so the command answers for this run whatever the harness passes its
// children. The variables are named one by one: the engine lists what a
// module reads. A call of a tool the session was not offered cannot arrive:
// no tool, no call. The brief tools are served the same way (briefs.ts).
export function serveCommand(on: Parameters<Register>[0], tool: CommandTool) {
  const name = toolName(tool);
  on("tool.call", { tool: name }, async ($, e) => {
    const executable = await $.env.get("REWAKE_EXECUTABLE");
    if (!executable) {
      return {
        isError: true,
        result: "this session was not started by a rewake that sets REWAKE_EXECUTABLE",
      };
    }
    const args = tool.args(e as unknown as Record<string, unknown>);
    if (typeof args === "string") return { isError: true, result: args };
    const env = runEnv({
      REWAKE_DIR: await $.env.get("REWAKE_DIR"),
      REWAKE_ROOM: await $.env.get("REWAKE_ROOM"),
      REWAKE_SESSION: await $.env.get("REWAKE_SESSION"),
      REWAKE_EPOCH: await $.env.get("REWAKE_EPOCH"),
    });
    try {
      const ran = await $.process.run([executable, ...args], {
        cwd: await $.session.cwd(),
        env,
        timeoutMs: tool.timeoutMs,
      });
      return answer(ran, tool.what);
    } catch (error) {
      return unrun(error, tool.what);
    }
  }).catch((_$, _e, next) => ({
    isError: true,
    result: `${name} failed: ${next.error.message}`,
  }));
}

// serveWorktreeStatus answers the worktree tools' calls.
export function serveWorktreeStatus(on: Parameters<Register>[0]) {
  for (const tool of [worktreeStatusTool, worktreeListTool, worktreeLandTool]) {
    serveCommand(on, tool);
  }
}
