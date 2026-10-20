import { expect, test } from "claude-code/testing";
import { enterTimeoutMs, guardRefusal, worktreeEnterToolName } from "../hooks/enter.ts";
import { epoch, socket, start, world } from "./world.ts";

const entering = {
  REWAKE_MOD_SOCKET: socket,
  REWAKE_EPOCH: epoch,
  REWAKE_EXECUTABLE: "/opt/rewake/bin/rewake",
  REWAKE_DIR: "/state/v2/rooms/team",
  REWAKE_ROOM: "team",
  REWAKE_SESSION: "api-claude",
  REWAKE_WORKTREE_ENTER: "1",
};
const path = "/trees/repo-1a2b/fix";
const created = JSON.stringify({
  outcome: "created",
  checkout: { ref: "fix", path, branch: "fix" },
});

// Stands up the run: what is registered, run and entered, the CLI answering
// with ran and Claude Code's EnterWorktree with entered; the session's root
// is root.
function stage(
  on: any,
  given: {
    env?: Record<string, string>;
    ran?: () => any;
    entered?: (e: any) => any;
    root?: string;
  } = {},
) {
  world(on, { env: given.env ?? entering });
  const seen = { registered: [] as any[], runs: [] as any[], entries: [] as any[] };
  on("tool.register", (_: any, e: any) => {
    seen.registered.push(e.name);
    return { value: { tool: "mcp__rewake__" + e.name } };
  });
  on("session.cwd", () => ({ value: "/work/repo" }));
  on("session.root", () => ({ value: given.root ?? "/work/repo" }));
  on("process.run", (_: any, e: any) => {
    seen.runs.push(e);
    return (
      given.ran ?? (() => ({ value: { exitCode: 0, stdout: created + "\n", stderr: "" } }))
    )();
  });
  on("tool.call", (_: any, e: any) => {
    seen.entries.push(e);
    return (
      given.entered ?? (() => ({ result: { worktreePath: path }, text: "Switched to " + path }))
    )(e);
  });
  return seen;
}

test("a run that may enter is offered worktree_enter; one that may not is not", async ($, on) => {
  const seen = stage(on);
  await start($);
  expect(seen.registered).toContain("worktree_enter");
});

test("a run without entry registers no worktree_enter", async ($, on) => {
  const without: Record<string, string> = { ...entering };
  delete without.REWAKE_WORKTREE_ENTER;
  const seen = stage(on, { env: without });
  await start($);
  expect(seen.registered).not.toContain("worktree_enter");
});

test("worktree_enter has rewake make the checkout, then enters it by path", async ($, on) => {
  const seen = stage(on);
  await start($);
  const answer: any = await $.tool.call({ tool: worktreeEnterToolName, name: "fix" } as any);
  expect(answer.isError).toBeUndefined();
  expect(answer.result).toContain(created);
  expect(answer.result).toContain("Switched to " + path);
  expect(seen.runs).toHaveLength(1);
  expect(seen.runs[0].argv).toEqual([
    "/opt/rewake/bin/rewake",
    "worktree",
    "enter",
    "fix",
    "--json",
  ]);
  expect(seen.runs[0].init.cwd).toBe("/work/repo");
  expect(seen.runs[0].init.timeoutMs).toBe(enterTimeoutMs);
  expect(seen.runs[0].init.env.REWAKE_WORKTREE_ENTER).toBe("1");
  expect(seen.runs[0].init.env.REWAKE_EPOCH).toBe(epoch);
  const native = seen.entries.filter((e) => e.tool === "EnterWorktree");
  expect(native).toHaveLength(1);
  expect(native[0].path).toBe(path);
  expect(native[0].name).toBeUndefined();
});

test("a session already in its checkout is not moved again", async ($, on) => {
  const seen = stage(on, { root: path + "/src" });
  await start($);
  const answer: any = await $.tool.call({ tool: worktreeEnterToolName } as any);
  expect(answer.isError).toBeUndefined();
  expect(answer.result).toContain("already works in it");
  expect(seen.runs[0].argv).toEqual(["/opt/rewake/bin/rewake", "worktree", "enter", "--json"]);
  expect(seen.entries.filter((e) => e.tool === "EnterWorktree")).toEqual([]);
});

test("a refusal of rewake's enters nothing, and a refused entry says the checkout stays", async ($, on) => {
  const results = [
    { value: { exitCode: 2, stdout: "", stderr: "rewake: one checkout per worker: yours is fix" } },
    { value: { exitCode: 0, stdout: created, stderr: "" } },
  ];
  const seen = stage(on, {
    ran: () => results.shift(),
    entered: () => ({ result: { message: "" }, text: "not a git worktree", isError: true }),
  });
  await start($);
  const refused: any = await $.tool.call({ tool: worktreeEnterToolName, name: "other" } as any);
  expect(refused.isError).toBe(true);
  expect(refused.result).toContain("one checkout per worker");
  expect(seen.entries.filter((e) => e.tool === "EnterWorktree")).toEqual([]);
  const failed: any = await $.tool.call({ tool: worktreeEnterToolName } as any);
  expect(failed.isError).toBe(true);
  expect(failed.result).toContain("not a git worktree");
  expect(failed.result).toContain("stays");
});

test("a name read as a flag runs nothing", async ($, on) => {
  const seen = stage(on);
  await start($);
  for (const name of ["", "--force", 3]) {
    const answer: any = await $.tool.call({ tool: worktreeEnterToolName, name } as any);
    expect(answer.isError).toBe(true);
  }
  expect(seen.runs).toEqual([]);
});

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
