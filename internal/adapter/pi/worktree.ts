// The worktree tools, with the names and contracts of the ones Claude Code's
// mod serves: the model asks which checkout its session works in
// (worktree_status, every session), and main lists its room's checkouts and
// lands one (worktree_list, worktree_land). rewake answers with a
// `rewake worktree …` command, run by argv through the rewake that started
// the run (REWAKE_EXECUTABLE). pi's exec spawns without a shell, and its child
// inherits this process's environment, the run's REWAKE_DIR, room, session and
// epoch among it (docs/worktree-tools.md).
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

export const statusTimeoutMs = 60_000;
// landTimeoutMs outlasts land's own wait of a minute for a hook.
export const landTimeoutMs = 120_000;

export const description =
  "Say which git worktree rewake made for this session, if any — its path, branch and the commit it was made at — and what this worker's space close would do with it if it closed now: remove it (nothing would be lost; files git ignores go too), keep it with the reasons, or none (main, or no worker of a space). A look, not a promise. Takes no input.";
export const listDescription =
  "List the git worktrees rewake made for the sessions of this room, each with its path, branch, the commit it was made at, the session it was made for and whether one still runs there, and what a removal would lose (changes, ignored files, commits that no branch, tag or remote-tracking ref holds, the worktree's own branch included). A listing, not a close forecast: worktree_status says what this worker's close would do. A look, not a promise. Takes no input.";
export const landDescription =
  "Fast-forward a branch to the work of one of this room's worktrees, as rewake worktree land does, and nothing else: the worktree and its session stay, hashes are kept, and it can be repeated. name is the worktree's name from worktree_list, or <repository>/<name>; into is the branch to move, by default the one checked out where the worktree was made from. Refused, with the reason and what to do: a target that moved on (rebase the branch in the worktree, then land again), one checked out elsewhere, changes in the target's checkout. Landing into the integration branch is a decision of the owner or a review: ask first.";

export const statusArgs = ["worktree", "status", "--json"];
export const listArgs = ["worktree", "ls", "--this-room", "--json"];

// landArgs is the land command's arguments, or why the input cannot make
// them: a name that is empty or begins with a dash would be read as a flag.
export function landArgs(input: { name?: unknown; into?: unknown }): string[] | string {
  const { name, into } = input;
  if (typeof name !== "string" || name === "" || name.startsWith("-")) {
    return "worktree_land needs name, the worktree's name from worktree_list";
  }
  // An empty into is refused, not read as "leave out": it would land into the default.
  if (into !== undefined && (typeof into !== "string" || into === "" || into.startsWith("-"))) {
    return "into must be a branch name; leave it out for the default";
  }
  return ["worktree", "land", name, ...(into !== undefined ? ["--into=" + into] : []), "--json"];
}

type Ran = { stdout: string; stderr: string; code: number; killed: boolean };
type Answer = { text: string; isError: boolean };

// answer is the tool's result: the command's JSON when it ran to its end and
// succeeded, otherwise an error that says what went wrong. pi reports a child
// it killed at the timeout with code 0, so killed is read first. what names
// the command in the error; bound is the timeout it was given.
export function answer(ran: Ran, what = "rewake worktree status", bound = statusTimeoutMs): Answer {
  if (ran.killed) {
    return {
      isError: true,
      text: `${what} did not run to its end: stopped after ${bound} ms or by an abort`,
    };
  }
  if (ran.code !== 0) {
    const said = ran.stderr.trim() || ran.stdout.trim() || "no output";
    return { isError: true, text: `${what} failed (exit ${ran.code}): ${said}` };
  }
  return { isError: false, text: ran.stdout.trim() };
}

type Tool = {
  name: string;
  label: string;
  description: string;
  what: string;
  bound: number;
  properties: Record<string, unknown>;
  required?: string[];
  args: (input: any) => string[] | string;
};

const tools: Record<string, Tool> = {
  status: {
    name: "worktree_status",
    label: "Worktree status",
    description,
    what: "rewake worktree status",
    bound: statusTimeoutMs,
    properties: {},
    args: () => statusArgs,
  },
  list: {
    name: "worktree_list",
    label: "Worktree list",
    description: listDescription,
    what: "rewake worktree ls",
    bound: statusTimeoutMs,
    properties: {},
    args: () => listArgs,
  },
  land: {
    name: "worktree_land",
    label: "Worktree land",
    description: landDescription,
    what: "rewake worktree land",
    bound: landTimeoutMs,
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
    args: landArgs,
  },
};

// worktreeTools registers the tools a run is offered, in a run whose rewake
// names itself; a pi started otherwise has no rewake to ask. Main's two
// choose what a session is shown, not who may run the commands behind them.
export function worktreeTools(
  pi: ExtensionAPI,
  executable = process.env.REWAKE_EXECUTABLE,
  role = process.env.REWAKE_ROLE,
) {
  if (!executable) return;
  for (const tool of role === "main" ? [tools.status, tools.list, tools.land] : [tools.status]) {
    pi.registerTool({
      name: tool.name,
      label: tool.label,
      description: tool.description,
      parameters: {
        type: "object",
        properties: tool.properties,
        ...(tool.required ? { required: tool.required } : {}),
        additionalProperties: false,
      } as never,
      async execute(_id, params, signal, _onUpdate, ctx) {
        let result: Answer;
        const args = tool.args(params);
        if (typeof args === "string") {
          result = { isError: true, text: args };
        } else {
          try {
            const ran = await pi.exec(executable, args, {
              cwd: ctx.cwd,
              signal,
              timeout: tool.bound,
            });
            result = answer(ran, tool.what, tool.bound);
          } catch (error) {
            const why = error instanceof Error ? error.message : String(error);
            result = { isError: true, text: `${tool.what} did not run to its end: ${why}` };
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
}
