// The brief tools, with the names and contracts of the ones Claude Code's mod
// serves: a run main gave a brief reads it, ticks its items and comments
// (brief_read, brief_tick, brief_comment). Each runs a `rewake brief …`
// command by argv through the rewake that started the run, as the worktree
// tools do; the child inherits the run's REWAKE_DIR, room, session and epoch,
// and the right to a brief is the command's check (docs/briefs.md).
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { answer } from "./worktree.ts";

export const briefTimeoutMs = 60_000;

export const readDescription =
  "Read a brief main gave this run with a task: a header naming its edition, the items done and the comments, then its markdown text with each item's number after its box (- [ ] #3 ...), then the comments. id is the brief's number, as the letter names it. Only a run the brief was sent to may read it, while it is open.";
export const tickDescription =
  "Mark an item of a brief done, or not done with undo: item is its number as brief_read shows it after the box, edition the edition brief_read's header names. Refused once main has edited the brief since that edition: read it again and tick by the new numbers. Your own ticks leave the edition as it is. Only a run the brief was sent to as work may tick; a run given it to review is refused, and says in a comment what it finds done.";
export const commentDescription =
  "Add a comment to a brief, at most 1000 characters, for main and the person to read under its text, shown with your name and whether you work on the brief or review it: what was found, what blocks, what was decided. Write markdown: a one-line point first, then short bullets, one idea per line, no wall of text; its lines are kept as written. It does not replace the report: that is still your final reply. Only a run the brief was sent to may comment.";

const positive = (value: unknown) =>
  typeof value === "number" && Number.isInteger(value) && value > 0;

export function readArgs(input: { id?: unknown }): string[] | string {
  if (!positive(input.id)) return "brief_read needs id, the brief's number from 1";
  return ["brief", "show", String(input.id)];
}

export function tickArgs(input: {
  id?: unknown;
  item?: unknown;
  edition?: unknown;
  undo?: unknown;
}): string[] | string {
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
export function commentArgs(input: { id?: unknown; text?: unknown }): string[] | string {
  const { id, text } = input;
  if (!positive(id)) return "brief_comment needs id, the brief's number from 1";
  if (typeof text !== "string" || text.trim() === "")
    return "brief_comment needs text, the comment";
  return ["brief", "comment", String(id), "--", text];
}

const id = { type: "integer", minimum: 1, description: "The brief's number." };

const tools = [
  {
    name: "brief_read",
    label: "Brief read",
    description: readDescription,
    what: "rewake brief show",
    properties: { id },
    required: ["id"],
    args: readArgs,
  },
  {
    name: "brief_tick",
    label: "Brief tick",
    description: tickDescription,
    what: "rewake brief tick",
    properties: {
      id,
      item: { type: "integer", minimum: 1, description: "The item's number, #n in brief_read." },
      edition: { type: "integer", minimum: 1, description: "The edition the number was read in." },
      undo: { type: "boolean", description: "Mark the item not done." },
    },
    required: ["id", "item", "edition"],
    args: tickArgs,
  },
  {
    name: "brief_comment",
    label: "Brief comment",
    description: commentDescription,
    what: "rewake brief comment",
    properties: {
      id,
      text: { type: "string", description: "The comment, at most 1000 characters." },
    },
    required: ["id", "text"],
    args: commentArgs,
  },
];

// briefTools registers the tools for every run but main's, which uses the
// commands, in a run whose rewake names itself. This chooses what a session
// is shown, not what it may do.
export function briefTools(
  pi: ExtensionAPI,
  executable = process.env.REWAKE_EXECUTABLE,
  role = process.env.REWAKE_ROLE,
) {
  if (!executable || role === "main") return;
  for (const tool of tools) {
    pi.registerTool({
      name: tool.name,
      label: tool.label,
      description: tool.description,
      parameters: {
        type: "object",
        properties: tool.properties,
        required: tool.required,
        additionalProperties: false,
      } as never,
      async execute(_id, params, signal, _onUpdate, ctx) {
        let result: { text: string; isError: boolean };
        const args = tool.args(params as never);
        if (typeof args === "string") {
          result = { isError: true, text: args };
        } else {
          try {
            const ran = await pi.exec(executable, args, {
              cwd: ctx.cwd,
              signal,
              timeout: briefTimeoutMs,
            });
            result = answer(ran, tool.what, briefTimeoutMs);
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
