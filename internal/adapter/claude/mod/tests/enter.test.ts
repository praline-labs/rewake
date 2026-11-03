import { expect, test } from "claude-code/testing";
import { guardRefusal } from "../hooks/enter.ts";
import { epoch, socket, start, world } from "./world.ts";

// The mod's own checks at the call: the model's EnterWorktree is refused in
// a run that may enter, and the engine's verdict on it stands. The move
// worktree_enter makes is tools.test.ts's.
const entering = { REWAKE_MOD_SOCKET: socket, REWAKE_EPOCH: epoch, REWAKE_WORKTREE_ENTER: "1" };
const path = "/trees/repo-1a2b/fix";

// Stands up the run and records what reaches Claude Code's EnterWorktree.
function stage(on: any, given: { env?: Record<string, string> } = {}) {
  world(on, { env: given.env ?? entering });
  const seen = { entries: [] as any[] };
  on("tool.call", (_: any, e: any) => {
    seen.entries.push(e);
    return { result: { worktreePath: path }, text: "Switched to " + path };
  });
  return seen;
}

test("the model's own EnterWorktree is refused in a run that may enter", async ($, on) => {
  const seen = stage(on);
  await start($);
  const answer: any = await $.tool.call({ tool: "EnterWorktree", name: "mine" } as any);
  expect(answer.deny).toBe(guardRefusal);
  expect(seen.entries).toEqual([]);
});

test("elsewhere — main, a worker without entry — EnterWorktree goes on untouched", async ($, on) => {
  const main: Record<string, string> = { ...entering, REWAKE_ROLE: "main" };
  delete main.REWAKE_WORKTREE_ENTER;
  const seen = stage(on, { env: main });
  await start($);
  const answer: any = await $.tool.call({ tool: "EnterWorktree", name: "mine" } as any);
  expect(answer.deny).toBeUndefined();
  expect(seen.entries).toHaveLength(1);
});

test("the permission check of the model's own EnterWorktree keeps the engine's verdict", async ($, on) => {
  stage(on);
  on("tool.check", () => ({ decision: "ask" }));
  await start($);
  const verdict: any = await $.tool.check({
    tool: "EnterWorktree",
    input: { path },
    tool_use_id: "t1",
  } as any);
  expect(verdict.decision).toBe("ask");
});
