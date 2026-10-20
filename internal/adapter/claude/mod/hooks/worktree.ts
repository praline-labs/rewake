// The worktree tools: the model asks which checkout its session works in
// (worktree_status, every session), and main lists its room's checkouts and
// lands one (worktree_list, worktree_land). The answer is rewake's own, a
// `rewake worktree …` command run by argv through the rewake that started the
// run (REWAKE_EXECUTABLE): no shell, nothing from the model's input but a
// checkout's name and a branch's, each one argument that is no flag, and the
// session known by its run's variables, never by its directory
// (docs/worktree-tools.md). rewake's pi extension serves the same tools with
// the same contracts.
import type { Register } from "claude-code";

// The tools as the model sees them: mcp__rewake__worktree_status and so on.
export const worktreeTool = {
  name: "worktree_status",
  description:
    "Say which git worktree rewake made for this session, if any — its path, branch and the commit it was made at — and what this worker's space close would do with it if it closed now: remove it (nothing would be lost; files git ignores go too), keep it with the reasons, or none (main, or no worker of a space). A look, not a promise. Takes no input.",
  inputSchema: { type: "object", properties: {}, additionalProperties: false },
};
export const worktreeListTool = {
  name: "worktree_list",
  description:
    "List the git worktrees rewake made for the sessions of this room, each with its path, branch, the commit it was made at, the session it was made for and whether one still runs there, and what a removal would lose (changes, ignored files, commits on no other branch). A look, not a promise. Takes no input.",
  inputSchema: { type: "object", properties: {}, additionalProperties: false },
};
export const worktreeLandTool = {
  name: "worktree_land",
  description:
    "Fast-forward a branch to the work of one of this room's worktrees, as rewake worktree land does, and nothing else: the worktree and its session stay, hashes are kept, and it can be repeated. name is the worktree's name from worktree_list, or <repository>/<name>; into is the branch to move, by default the one checked out where the worktree was made from. Refused, with the reason and what to do: a target that moved on (rebase the branch in the worktree, then land again), one checked out elsewhere, changes in the target's checkout. Landing into the integration branch is a decision of the owner or a review: ask first.",
  inputSchema: {
    type: "object",
    properties: {
      name: {
        type: "string",
        description: "The worktree to land: its name, or <repository>/<name>.",
      },
      into: {
        type: "string",
        description: "The branch to fast-forward; leave out for the default.",
      },
    },
    required: ["name"],
    additionalProperties: false,
  },
};
export const worktreeToolName = "mcp__rewake__" + worktreeTool.name;
export const worktreeListToolName = "mcp__rewake__" + worktreeListTool.name;
export const worktreeLandToolName = "mcp__rewake__" + worktreeLandTool.name;

// The tools a role is offered. Main's two change nothing about who may run the
// commands behind them: this chooses what a session is shown, not what it may do.
export function worktreeToolsFor(role: string | undefined) {
  return role === "main" ? [worktreeTool, worktreeListTool, worktreeLandTool] : [worktreeTool];
}

// statusTimeoutMs bounds a view over records and git, seconds at most; past
// this it is reported as failed rather than waited on.
export const statusTimeoutMs = 60_000;
// landTimeoutMs outlasts land's own wait of a minute for a hook: the engine's
// default of 30 s would kill a land that is only waiting.
export const landTimeoutMs = 120_000;

// statusArgv is the command, by argv.
export function statusArgv(executable: string): string[] {
  return [executable, "worktree", "status", "--json"];
}
export function listArgv(executable: string): string[] {
  return [executable, "worktree", "ls", "--this-room", "--json"];
}

// landArgv is the land command, or why the input cannot make one: a name that
// is empty or begins with a dash would be read as a flag.
export function landArgv(executable: string, input: Record<string, unknown>): string[] | string {
  const { name, into } = input;
  if (typeof name !== "string" || name === "" || name.startsWith("-")) {
    return "worktree_land needs name, the worktree's name from worktree_list";
  }
  // An empty into is refused, not read as "leave out": it would land into the default.
  if (into !== undefined && (typeof into !== "string" || into === "" || into.startsWith("-"))) {
    return "into must be a branch name; leave it out for the default";
  }
  const argv = [executable, "worktree", "land", name];
  if (into !== undefined) argv.push("--into=" + into);
  return [...argv, "--json"];
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
  if (ran.exitCode !== 0) {
    const said = ran.stderr.trim() || ran.stdout.trim() || "no output";
    return {
      isError: true,
      result: `${what} failed (exit ${ran.exitCode}): ${said}`,
    };
  }
  if (ran.isStdoutTruncated) {
    return {
      isError: true,
      result: `${what} answered more than the harness keeps; its answer was cut`,
    };
  }
  return { result: ran.stdout.trim() };
}

// unrun is the error of a command that did not run to its end: it could not
// start, or ran past the bound.
export function unrun(error: unknown, what = "rewake worktree status"): Answer {
  const why = error instanceof Error ? error.message : String(error);
  return { isError: true, result: `${what} did not run to its end: ${why}` };
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

// serveWorktreeStatus answers the tools' calls. The command's environment is
// the run's own variables, set over the harness's environment, so the
// command answers for this run whatever the harness passes its children.
// The variables are named one by one: the engine lists what a module reads.
// A call of a tool the session was not offered cannot arrive: no tool, no call.
export function serveWorktreeStatus(on: Parameters<Register>[0]) {
  const serve = (
    tool: string,
    what: string,
    timeoutMs: number,
    argv: (executable: string, input: Record<string, unknown>) => string[] | string,
  ) => {
    on("tool.call", { tool }, async ($, e) => {
      const executable = await $.env.get("REWAKE_EXECUTABLE");
      if (!executable) {
        return {
          isError: true,
          result: "this session was not started by a rewake that sets REWAKE_EXECUTABLE",
        };
      }
      const command = argv(executable, e as unknown as Record<string, unknown>);
      if (typeof command === "string") return { isError: true, result: command };
      const env = runEnv({
        REWAKE_DIR: await $.env.get("REWAKE_DIR"),
        REWAKE_ROOM: await $.env.get("REWAKE_ROOM"),
        REWAKE_SESSION: await $.env.get("REWAKE_SESSION"),
        REWAKE_EPOCH: await $.env.get("REWAKE_EPOCH"),
      });
      try {
        const ran = await $.process.run(command, {
          cwd: await $.session.cwd(),
          env,
          timeoutMs,
        });
        return answer(ran, what);
      } catch (error) {
        return unrun(error, what);
      }
    }).catch((_$, _e, next) => ({
      isError: true,
      result: `${tool} failed: ${next.error.message}`,
    }));
  };
  serve(worktreeToolName, "rewake worktree status", statusTimeoutMs, statusArgv);
  serve(worktreeListToolName, "rewake worktree ls", statusTimeoutMs, listArgv);
  serve(worktreeLandToolName, "rewake worktree land", landTimeoutMs, landArgv);
}
