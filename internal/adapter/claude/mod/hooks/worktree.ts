// The worktree_status tool: the model asks which checkout its session works
// in and what its space's close would do with it. The answer is rewake's own,
// `rewake worktree status --json`, run by argv through the rewake that started
// the run (REWAKE_EXECUTABLE): no shell, nothing from the model's input on the
// command line, and the session known by its run's variables, never by its
// directory (docs/worktree-tools.md). rewake's pi extension serves
// the same tool with the same contract.
import type { Register } from "claude-code";

// The tool as the model sees it: mcp__rewake__worktree_status.
export const worktreeTool = {
  name: "worktree_status",
  description:
    "Say which git worktree rewake made for this session, if any — its path, branch and the commit it was made at — and what this worker's space close would do with it if it closed now: remove it (nothing would be lost; files git ignores go too), keep it with the reasons, or none (main, or no worker of a space). A look, not a promise. Takes no input.",
  inputSchema: { type: "object", properties: {}, additionalProperties: false },
};
export const worktreeToolName = "mcp__rewake__" + worktreeTool.name;

// statusTimeoutMs bounds the command: a view over records and git, seconds at
// most; past this it is reported as failed rather than waited on.
export const statusTimeoutMs = 60_000;

// statusArgv is the command, by argv.
export function statusArgv(executable: string): string[] {
  return [executable, "worktree", "status", "--json"];
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
// a shorter answer.
export function answer(ran: Ran): Answer {
  if (ran.exitCode !== 0) {
    const said = ran.stderr.trim() || ran.stdout.trim() || "no output";
    return {
      isError: true,
      result: `rewake worktree status failed (exit ${ran.exitCode}): ${said}`,
    };
  }
  if (ran.isStdoutTruncated) {
    return {
      isError: true,
      result: "rewake worktree status answered more than the harness keeps; its answer was cut",
    };
  }
  return { result: ran.stdout.trim() };
}

// unrun is the error of a command that did not run to its end: it could not
// start, or ran past the bound.
export function unrun(error: unknown): Answer {
  const why = error instanceof Error ? error.message : String(error);
  return { isError: true, result: "rewake worktree status did not run to its end: " + why };
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

// serveWorktreeStatus answers the tool's calls. The command's environment is
// the run's own variables, set over the harness's environment, so the
// command answers for this run whatever the harness passes its children.
// The variables are named one by one: the engine lists what a module reads.
export function serveWorktreeStatus(on: Parameters<Register>[0]) {
  on("tool.call", { tool: "mcp__rewake__worktree_status" }, async ($) => {
    const executable = await $.env.get("REWAKE_EXECUTABLE");
    if (!executable) {
      return {
        isError: true,
        result: "this session was not started by a rewake that sets REWAKE_EXECUTABLE",
      };
    }
    const env = runEnv({
      REWAKE_DIR: await $.env.get("REWAKE_DIR"),
      REWAKE_ROOM: await $.env.get("REWAKE_ROOM"),
      REWAKE_SESSION: await $.env.get("REWAKE_SESSION"),
      REWAKE_EPOCH: await $.env.get("REWAKE_EPOCH"),
    });
    try {
      const ran = await $.process.run(statusArgv(executable), {
        cwd: await $.session.cwd(),
        env,
        timeoutMs: statusTimeoutMs,
      });
      return answer(ran);
    } catch (error) {
      return unrun(error);
    }
  }).catch((_$, _e, next) => ({
    isError: true,
    result: "worktree_status failed: " + next.error.message,
  }));
}
