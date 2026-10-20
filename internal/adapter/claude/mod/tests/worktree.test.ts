import { expect, test } from "claude-code/testing";
import {
  landTimeoutMs,
  statusTimeoutMs,
  worktreeLandToolName,
  worktreeListToolName,
  worktreeToolName,
} from "../hooks/worktree.ts";
import { epoch, socket, start, world } from "./world.ts";

const env = {
  REWAKE_MOD_SOCKET: socket,
  REWAKE_EPOCH: epoch,
  REWAKE_EXECUTABLE: "/opt/rewake/bin/rewake",
  REWAKE_DIR: "/state/v2/rooms/team",
  REWAKE_ROOM: "team",
  REWAKE_SESSION: "api-claude",
};
const status = '{"session":"api-claude","checkout":null,"close":{"outcome":"none"}}';

// Stands up the run, records what is registered and run, and answers each run
// with ran.
function stage(on: any, ran: () => any, given: Record<string, string> = env) {
  world(on, { env: given });
  const registered: any[] = [];
  const runs: any[] = [];
  on("tool.register", (_: any, e: any) => {
    registered.push(e);
    return { value: { tool: "mcp__rewake__" + e.name } };
  });
  on("session.cwd", () => ({ value: "/work/tree" }));
  on("process.run", (_: any, e: any) => {
    runs.push(e);
    return ran();
  });
  return { registered, runs };
}

const call = ($: any, tool = worktreeToolName, input: object = {}) =>
  $.tool.call({ tool, ...input } as any);

test("the tool runs rewake's status by argv with the run's variables, and answers its JSON", async ($, on) => {
  const seen = stage(on, () => ({ value: { exitCode: 0, stdout: status + "\n", stderr: "" } }));
  await start($);
  expect(seen.registered.map((one) => one.name)).toEqual(["worktree_status"]);
  const answer: any = await call($);
  expect(answer.isError).toBeUndefined();
  expect(answer.result).toBe(status);
  expect(seen.runs).toHaveLength(1);
  const [run] = seen.runs;
  expect(run.argv).toEqual(["/opt/rewake/bin/rewake", "worktree", "status", "--json"]);
  expect(run.init.timeoutMs).toBe(statusTimeoutMs);
  expect(run.init.cwd).toBe("/work/tree");
  expect(run.init.env).toEqual({
    REWAKE_DIR: env.REWAKE_DIR,
    REWAKE_ROOM: env.REWAKE_ROOM,
    REWAKE_SESSION: env.REWAKE_SESSION,
    REWAKE_EPOCH: env.REWAKE_EPOCH,
  });
});

test("a refusal, a cut answer and a command that did not run are errors", async ($, on) => {
  const results = [
    {
      value: {
        exitCode: 2,
        stdout: "",
        stderr: "rewake: status answers for the session it runs in",
      },
    },
    { value: { exitCode: 0, stdout: status.slice(0, 10), stderr: "", isStdoutTruncated: true } },
    { deny: "the command is still running after 60000 ms" },
  ];
  stage(on, () => results.shift());
  await start($);
  const refused: any = await call($);
  expect(refused.isError).toBe(true);
  expect(refused.result).toContain("exit 2");
  expect(refused.result).toContain("status answers for the session it runs in");
  const cut: any = await call($);
  expect(cut.isError).toBe(true);
  expect(cut.result).toContain("cut");
  const unrun: any = await call($);
  expect(unrun.isError).toBe(true);
  expect(unrun.result).toContain("did not run to its end");
});

test("a run whose rewake sets no executable registers no tool", async ($, on) => {
  const without: Record<string, string> = { ...env };
  delete without.REWAKE_EXECUTABLE;
  const seen = stage(on, () => ({ value: { exitCode: 0, stdout: status, stderr: "" } }), without);
  await start($);
  expect(seen.registered).toEqual([]);
});

const asMain = { ...env, REWAKE_ROLE: "main" };

test("main is offered the list and the land tools beside status, a worker only status", async ($, on) => {
  const main = stage(on, () => ({ value: { exitCode: 0, stdout: "{}", stderr: "" } }), asMain);
  await start($);
  expect(main.registered.map((one) => one.name)).toEqual([
    "worktree_status",
    "worktree_list",
    "worktree_land",
  ]);
});

test("a write session is offered status alone", async ($, on) => {
  const seen = stage(on, () => ({ value: { exitCode: 0, stdout: "{}", stderr: "" } }), {
    ...env,
    REWAKE_ROLE: "write",
  });
  await start($);
  expect(seen.registered.map((one) => one.name)).toEqual(["worktree_status"]);
});

test("list asks for this room's checkouts, land runs once with the name and the branch as single arguments", async ($, on) => {
  const seen = stage(
    on,
    () => ({ value: { exitCode: 0, stdout: '{"worktrees":[]}', stderr: "" } }),
    asMain,
  );
  await start($);
  await call($, worktreeListToolName);
  await call($, worktreeLandToolName, { name: "fix-login", into: "release" });
  await call($, worktreeLandToolName, { name: "fix-login" });
  expect(seen.runs.map((run) => run.argv)).toEqual([
    ["/opt/rewake/bin/rewake", "worktree", "ls", "--this-room", "--json"],
    ["/opt/rewake/bin/rewake", "worktree", "land", "fix-login", "--into=release", "--json"],
    ["/opt/rewake/bin/rewake", "worktree", "land", "fix-login", "--json"],
  ]);
  expect(seen.runs[0].init.timeoutMs).toBe(statusTimeoutMs);
  // land waits up to a minute for a hook, so its bound is longer than that.
  expect(seen.runs[1].init.timeoutMs).toBe(landTimeoutMs);
  expect(landTimeoutMs).toBeGreaterThan(60_000);
});

test("land refuses a name or a branch that would be read as a flag, without running anything", async ($, on) => {
  const seen = stage(on, () => ({ value: { exitCode: 0, stdout: "{}", stderr: "" } }), asMain);
  await start($);
  for (const input of [
    {},
    { name: "" },
    { name: "--force" },
    { name: "x", into: "-f" },
    { name: "x", into: "" },
    { name: 3 },
  ]) {
    const answer: any = await call($, worktreeLandToolName, input);
    expect(answer.isError).toBe(true);
  }
  expect(seen.runs).toEqual([]);
});

test("a land the command refuses answers its reason as an error", async ($, on) => {
  stage(
    on,
    () => ({
      value: { exitCode: 1, stdout: "", stderr: "rewake: main moved on; rebase the branch" },
    }),
    asMain,
  );
  await start($);
  const answer: any = await call($, worktreeLandToolName, { name: "x" });
  expect(answer.isError).toBe(true);
  expect(answer.result).toContain("rewake worktree land failed (exit 1)");
  expect(answer.result).toContain("rebase the branch");
});
