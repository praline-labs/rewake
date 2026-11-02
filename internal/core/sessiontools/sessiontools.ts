// The session tools rewake offers through every harness: the brief tools
// (docs/briefs.md) and the worktree tools (docs/worktree-tools.md), each
// defined once here — its name, description, input schema, the check of its
// input into a rewake command's arguments or a refusal, its bound and the
// text of its result. Each tool is a thin front end of one `rewake …`
// command: the operation and the right to it are the command's, so a tool and
// a command the model types answer alike.
//
// The file names no harness and imports nothing. Each adapter embeds files of
// its own folder alone, so `go run ./tools/protogen` copies it into pi's
// extension and Claude Code's mod, as it copies the mirror's feed; an adapter
// keeps only its bridge: which roles see a tool, how it runs the command, the
// harness's own result shape and the failures only its runner has.

// A CommandTool is a tool answered by one rewake command, run by argv through
// the rewake that started the run.
export interface CommandTool {
  // name is the tool's name as registered; a harness may prefix it.
  name: string;
  // label is a short title, for a harness that shows one.
  label: string;
  description: string;
  inputSchema: {
    type: "object";
    properties: Record<string, unknown>;
    required?: string[];
    additionalProperties: false;
  };
  // what names the command in an error result.
  what: string;
  // timeoutMs bounds the command; past it the call is reported as failed.
  timeoutMs: number;
  // args is the command's arguments after the executable, or why the input
  // cannot make them: nothing runs for a refusal.
  args: (input: Record<string, unknown>) => string[] | string;
}

// An Outcome is a tool's result before the harness shapes it.
export interface Outcome {
  text: string;
  isError: boolean;
}

// outcome is the result of a command that ran to its end: its trimmed output
// when it succeeded, otherwise an error that says what went wrong.
export function outcome(
  what: string,
  code: number | null,
  stdout: string,
  stderr: string,
): Outcome {
  if (code !== 0) {
    const said = stderr.trim() || stdout.trim() || "no output";
    return { isError: true, text: `${what} failed (exit ${code}): ${said}` };
  }
  return { isError: false, text: stdout.trim() };
}

// didNotEnd is the error of a command that did not run to its end: it could
// not start, ran past its bound or was aborted.
export function didNotEnd(what: string, why: unknown): Outcome {
  const said = why instanceof Error ? why.message : String(why);
  return { isError: true, text: `${what} did not run to its end: ${said}` };
}

// The worktree tools: the model asks which checkout its session works in
// (worktree_status), and main lists its room's checkouts and lands one
// (worktree_list, worktree_land). Nothing from the model's input reaches the
// command but a checkout's name and a branch's, each one argument that is no
// flag, and the session is known by its run's variables, never by its
// directory.

// statusTimeoutMs bounds a view over records and git, seconds at most.
export const statusTimeoutMs = 60_000;
// landTimeoutMs outlasts land's own wait of a minute for a hook: a shorter
// bound would kill a land that is only waiting.
export const landTimeoutMs = 120_000;

export const statusArgs = ["worktree", "status", "--json"];
export const listArgs = ["worktree", "ls", "--this-room", "--json"];

// landArgs is the land command's arguments, or why the input cannot make
// them: a name that is empty or begins with a dash would be read as a flag.
export function landArgs(input: Record<string, unknown>): string[] | string {
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

export const worktreeStatusTool: CommandTool = {
  name: "worktree_status",
  label: "Worktree status",
  description:
    "Say which git worktree rewake made for this session, if any — its path, branch and the commit it was made at — and what this worker's space close would do with it if it closed now: remove it (nothing would be lost; files git ignores go too), keep it with the reasons, or none (main, or no worker of a space). A look, not a promise. Takes no input.",
  inputSchema: { type: "object", properties: {}, additionalProperties: false },
  what: "rewake worktree status",
  timeoutMs: statusTimeoutMs,
  args: () => statusArgs,
};

export const worktreeListTool: CommandTool = {
  name: "worktree_list",
  label: "Worktree list",
  description:
    "List the git worktrees rewake made for the sessions of this room, each with its path, branch, the commit it was made at, the session it was made for and whether one still runs there, and what a removal would lose (changes, ignored files, commits that no branch, tag or remote-tracking ref holds, the worktree's own branch included). A listing, not a close forecast: worktree_status says what this worker's close would do. A look, not a promise. Takes no input.",
  inputSchema: { type: "object", properties: {}, additionalProperties: false },
  what: "rewake worktree ls",
  timeoutMs: statusTimeoutMs,
  args: () => listArgs,
};

export const worktreeLandTool: CommandTool = {
  name: "worktree_land",
  label: "Worktree land",
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
  what: "rewake worktree land",
  timeoutMs: landTimeoutMs,
  args: landArgs,
};

// The brief tools: a run main gave a brief reads it, ticks its items and
// comments.

// briefTimeoutMs bounds a command over a brief's records.
export const briefTimeoutMs = 60_000;

const positive = (value: unknown) =>
  typeof value === "number" && Number.isInteger(value) && value > 0;

export function readArgs(input: Record<string, unknown>): string[] | string {
  if (!positive(input.id)) return "brief_read needs id, the brief's number from 1";
  return ["brief", "show", String(input.id)];
}

export function tickArgs(input: Record<string, unknown>): string[] | string {
  const { id, item, edition, undo } = input;
  if (!positive(id)) return "brief_tick needs id, the brief's number from 1";
  if (!positive(item))
    return "brief_tick needs item, the item's number from 1 as brief_read shows it";
  if (!positive(edition)) return "brief_tick needs edition, the edition brief_read named";
  if (undo !== undefined && typeof undo !== "boolean") return "undo is true or false";
  const args = ["brief", "tick", String(id), String(item), "--edition=" + String(edition)];
  return undo ? [...args, "--undo"] : args;
}

// commentArgs puts the text after --, so one that begins with a dash is no flag.
export function commentArgs(input: Record<string, unknown>): string[] | string {
  const { id, text } = input;
  if (!positive(id)) return "brief_comment needs id, the brief's number from 1";
  if (typeof text !== "string" || text.trim() === "")
    return "brief_comment needs text, the comment";
  return ["brief", "comment", String(id), "--", text];
}

export const briefReadTool: CommandTool = {
  name: "brief_read",
  label: "Brief read",
  description:
    "Read a brief main gave this run with a task: a header naming its edition, the items done and the comments, then its markdown text with each item's number after its box (- [ ] #3 ...), then the comments. id is the brief's number, as the letter names it. Only a run the brief was sent to may read it, while it is open.",
  inputSchema: {
    type: "object",
    properties: { id: { type: "integer", minimum: 1, description: "The brief's number." } },
    required: ["id"],
    additionalProperties: false,
  },
  what: "rewake brief show",
  timeoutMs: briefTimeoutMs,
  args: readArgs,
};

export const briefTickTool: CommandTool = {
  name: "brief_tick",
  label: "Brief tick",
  description:
    "Mark an item of a brief done, or not done with undo: item is its number as brief_read shows it after the box, edition the edition brief_read's header names. Refused once main has edited the brief since that edition: read it again and tick by the new numbers. Your own ticks leave the edition as it is. Only a run the brief was sent to as work may tick; a run given it to review is refused, and says in a comment what it finds done.",
  inputSchema: {
    type: "object",
    properties: {
      id: { type: "integer", minimum: 1, description: "The brief's number." },
      item: { type: "integer", minimum: 1, description: "The item's number, #n in brief_read." },
      edition: {
        type: "integer",
        minimum: 1,
        description: "The edition the number was read in.",
      },
      undo: { type: "boolean", description: "Mark the item not done." },
    },
    required: ["id", "item", "edition"],
    additionalProperties: false,
  },
  what: "rewake brief tick",
  timeoutMs: briefTimeoutMs,
  args: tickArgs,
};

export const briefCommentTool: CommandTool = {
  name: "brief_comment",
  label: "Brief comment",
  description:
    "Add a comment to a brief, at most 1000 characters, for main and the person to read under its text, shown with your name and whether you work on the brief or review it: what was found, what blocks, what was decided. Write markdown: a one-line point first, then short bullets, one idea per line, no wall of text; its lines are kept as written. It does not replace the report: that is still your final reply. Only a run the brief was sent to may comment.",
  inputSchema: {
    type: "object",
    properties: {
      id: { type: "integer", minimum: 1, description: "The brief's number." },
      text: { type: "string", description: "The comment, at most 1000 characters." },
    },
    required: ["id", "text"],
    additionalProperties: false,
  },
  what: "rewake brief comment",
  timeoutMs: briefTimeoutMs,
  args: commentArgs,
};
