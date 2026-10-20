// The worktree_status tool, with the name and contract of the one Claude
// Code's mod serves: the model asks which checkout its session works in and
// what its space's close would do with it, and rewake answers with
// `rewake worktree status --json`, run by argv through the rewake that
// started the run (REWAKE_EXECUTABLE). pi's exec spawns without a shell, and
// its child inherits this process's environment, the run's REWAKE_DIR, room,
// session and epoch among it (docs/worktree-tools.md).
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

export const statusTimeoutMs = 60_000;

export const description =
  "Say which git worktree rewake made for this session, if any — its path, branch and the commit it was made at — and what this worker's space close would do with it if it closed now: remove it (nothing would be lost; files git ignores go too), keep it with the reasons, or none (main, or no worker of a space). A look, not a promise. Takes no input.";

export const statusArgs = ["worktree", "status", "--json"];

type Ran = { stdout: string; stderr: string; code: number; killed: boolean };
type Answer = { text: string; isError: boolean };

// answer is the tool's result: the command's JSON when it ran to its end and
// succeeded, otherwise an error that says what went wrong. pi reports a child
// it killed at the timeout with code 0, so killed is read first.
export function answer(ran: Ran): Answer {
  if (ran.killed) {
    return {
      isError: true,
      text: `rewake worktree status did not run to its end: stopped after ${statusTimeoutMs} ms or by an abort`,
    };
  }
  if (ran.code !== 0) {
    const said = ran.stderr.trim() || ran.stdout.trim() || "no output";
    return { isError: true, text: `rewake worktree status failed (exit ${ran.code}): ${said}` };
  }
  return { isError: false, text: ran.stdout.trim() };
}

// worktreeStatus registers the tool in a run whose rewake names itself; a pi
// started otherwise has no rewake to ask.
export function worktreeStatus(pi: ExtensionAPI, executable = process.env.REWAKE_EXECUTABLE) {
  if (!executable) return;
  pi.registerTool({
    name: "worktree_status",
    label: "Worktree status",
    description,
    parameters: { type: "object", properties: {}, additionalProperties: false } as never,
    async execute(_id, _params, signal, _onUpdate, ctx) {
      let result: Answer;
      try {
        const ran = await pi.exec(executable, statusArgs, {
          cwd: ctx.cwd,
          signal,
          timeout: statusTimeoutMs,
        });
        result = answer(ran);
      } catch (error) {
        const why = error instanceof Error ? error.message : String(error);
        result = { isError: true, text: "rewake worktree status did not run to its end: " + why };
      }
      return {
        content: [{ type: "text", text: result.text }],
        details: undefined,
        isError: result.isError,
      };
    },
  });
}
