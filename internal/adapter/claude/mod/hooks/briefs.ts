// The brief tools: a run main gave a brief reads it, ticks its items and
// comments (brief_read, brief_tick, brief_comment). Each is a thin front end
// of a `rewake brief …` command, served as the worktree tools are: by argv
// through the rewake that started the run, the run's variables set, bound,
// and every failure an error result. The right to a brief is the command's
// check, so a tool and a command the model types answer alike
// (docs/roadmap/briefs.md). rewake's pi extension serves the same tools with
// the same contracts.
import type { Register } from "claude-code";
import { serveCommand } from "./worktree.ts";

export const briefReadTool = {
  name: "brief_read",
  description:
    "Read a brief main gave this run with a task: a header naming its edition, the items done and the comments, then its markdown text with each item's number after its box (- [ ] #3 ...), then the comments. id is the brief's number, as the letter names it. Only a run the brief was sent to may read it, while it is open.",
  inputSchema: {
    type: "object",
    properties: { id: { type: "integer", minimum: 1, description: "The brief's number." } },
    required: ["id"],
    additionalProperties: false,
  },
};
export const briefTickTool = {
  name: "brief_tick",
  description:
    "Mark an item of a brief done, or not done with undo: item is its number as brief_read shows it after the box, edition the edition brief_read's header names. Refused once main has edited the brief since that edition: read it again and tick by the new numbers. Your own ticks leave the edition as it is. Only a run the brief was sent to may tick.",
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
};
export const briefCommentTool = {
  name: "brief_comment",
  description:
    "Add a comment to a brief, at most 1000 characters, for main and the person to read under its text: what was found, what blocks, what was decided. It does not replace the report: that is still your final reply. Only a run the brief was sent to may comment.",
  inputSchema: {
    type: "object",
    properties: {
      id: { type: "integer", minimum: 1, description: "The brief's number." },
      text: { type: "string", description: "The comment, at most 1000 characters." },
    },
    required: ["id", "text"],
    additionalProperties: false,
  },
};
export const briefReadToolName = "mcp__rewake__" + briefReadTool.name;
export const briefTickToolName = "mcp__rewake__" + briefTickTool.name;
export const briefCommentToolName = "mcp__rewake__" + briefCommentTool.name;

// The tools a role is offered: every run but main, which uses the commands.
// This chooses what a session is shown, not what it may do.
export function briefToolsFor(role: string | undefined) {
  return role === "main" ? [] : [briefReadTool, briefTickTool, briefCommentTool];
}

// briefTimeoutMs bounds a command over a brief's records.
export const briefTimeoutMs = 60_000;

const positive = (value: unknown) =>
  typeof value === "number" && Number.isInteger(value) && value > 0;

export function readArgv(executable: string, input: Record<string, unknown>): string[] | string {
  if (!positive(input.id)) return "brief_read needs id, the brief's number from 1";
  return [executable, "brief", "show", String(input.id)];
}

export function tickArgv(executable: string, input: Record<string, unknown>): string[] | string {
  const { id, item, edition, undo } = input;
  if (!positive(id)) return "brief_tick needs id, the brief's number from 1";
  if (!positive(item))
    return "brief_tick needs item, the item's number from 1 as brief_read shows it";
  if (!positive(edition)) return "brief_tick needs edition, the edition brief_read named";
  if (undo !== undefined && typeof undo !== "boolean") return "undo is true or false";
  const argv = [
    executable,
    "brief",
    "tick",
    String(id),
    String(item),
    "--edition=" + String(edition),
  ];
  return undo ? [...argv, "--undo"] : argv;
}

// commentArgv puts the text after --, so one that begins with a dash is no flag.
export function commentArgv(executable: string, input: Record<string, unknown>): string[] | string {
  const { id, text } = input;
  if (!positive(id)) return "brief_comment needs id, the brief's number from 1";
  if (typeof text !== "string" || text.trim() === "")
    return "brief_comment needs text, the comment";
  return [executable, "brief", "comment", String(id), "--", text];
}

export function serveBriefs(on: Parameters<Register>[0]) {
  serveCommand(on, briefReadToolName, "rewake brief show", briefTimeoutMs, readArgv);
  serveCommand(on, briefTickToolName, "rewake brief tick", briefTimeoutMs, tickArgv);
  serveCommand(on, briefCommentToolName, "rewake brief comment", briefTimeoutMs, commentArgv);
}
