// The brief tools: a run main gave a brief reads it, ticks its items and
// comments (brief_read, brief_tick, brief_comment). Each is defined once, in
// sessiontools.ts, a copy of the module rewake's pi extension serves the same
// tools from, and served as the worktree tools are, through their
// serveCommand: by argv through the rewake that started the run, the run's
// variables set, bound, and every failure an error result. The right to a
// brief is the command's check, so a tool and a command the model types
// answer alike (docs/briefs.md).
import type { Register } from "claude-code";
import { briefCommentTool, briefReadTool, briefTickTool } from "./sessiontools.ts";
import { registration, serveCommand, toolName } from "./worktree.ts";

export const briefReadToolName = toolName(briefReadTool);
export const briefTickToolName = toolName(briefTickTool);
export const briefCommentToolName = toolName(briefCommentTool);

// The tools a role is offered: every run but main, which uses the commands.
// This chooses what a session is shown, not what it may do.
export function briefToolsFor(role: string | undefined) {
  return role === "main" ? [] : [briefReadTool, briefTickTool, briefCommentTool].map(registration);
}

export function serveBriefs(on: Parameters<Register>[0]) {
  for (const tool of [briefReadTool, briefTickTool, briefCommentTool]) serveCommand(on, tool);
}
