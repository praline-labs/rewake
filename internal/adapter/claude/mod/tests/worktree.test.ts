import { expect, test } from "claude-code/testing";
import { statusTimeoutMs, worktreeToolName } from "../hooks/worktree.ts";
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

const call = ($: any) => $.tool.call({ tool: worktreeToolName } as any);

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
