import { expect, test } from "claude-code/testing";
import {
  briefCommentToolName,
  briefReadToolName,
  briefTickToolName,
  briefTimeoutMs,
} from "../hooks/briefs.ts";
import { epoch, socket, start, world } from "./world.ts";

const env = {
  REWAKE_MOD_SOCKET: socket,
  REWAKE_EPOCH: epoch,
  REWAKE_EXECUTABLE: "/opt/rewake/bin/rewake",
  REWAKE_DIR: "/state/v2/rooms/team",
  REWAKE_ROOM: "team",
  REWAKE_SESSION: "api-claude",
  REWAKE_ROLE: "write",
};
const shown =
  "brief 3 · Login · edition 2 · revision 5 · 1 of 2 done · 0 comments\n\n- [x] #1 move it";

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

const call = ($: any, tool: string, input: object) => $.tool.call({ tool, ...input } as any);
const ok = (stdout: string) => () => ({ value: { exitCode: 0, stdout, stderr: "" } });

test("a worker is offered the three brief tools, main none", async ($, on) => {
  const seen = stage(on, ok(""));
  await start($);
  expect(
    seen.registered.map((one) => one.name).filter((name) => name.startsWith("brief_")),
  ).toEqual(["brief_read", "brief_tick", "brief_comment"]);
});

test("main is offered no brief tool", async ($, on) => {
  const seen = stage(on, ok(""), { ...env, REWAKE_ROLE: "main" });
  await start($);
  expect(
    seen.registered.map((one) => one.name).filter((name) => name.startsWith("brief_")),
  ).toEqual([]);
});

test("each tool runs its rewake brief command by argv with the run's variables, and answers its lines", async ($, on) => {
  const seen = stage(on, ok(shown + "\n"));
  await start($);
  const read: any = await call($, briefReadToolName, { id: 3 });
  expect(read.isError).toBeUndefined();
  expect(read.result).toBe(shown);
  await call($, briefTickToolName, { id: 3, item: 1, edition: 2 });
  await call($, briefTickToolName, { id: 3, item: 1, edition: 2, undo: true });
  await call($, briefCommentToolName, { id: 3, text: "--force was not needed" });
  expect(seen.runs.map((run) => run.argv)).toEqual([
    ["/opt/rewake/bin/rewake", "brief", "show", "3"],
    ["/opt/rewake/bin/rewake", "brief", "tick", "3", "1", "--edition=2"],
    ["/opt/rewake/bin/rewake", "brief", "tick", "3", "1", "--edition=2", "--undo"],
    ["/opt/rewake/bin/rewake", "brief", "comment", "3", "--", "--force was not needed"],
  ]);
  for (const run of seen.runs) {
    expect(run.init.timeoutMs).toBe(briefTimeoutMs);
    expect(run.init.cwd).toBe("/work/tree");
    expect(run.init.env).toEqual({
      REWAKE_DIR: env.REWAKE_DIR,
      REWAKE_ROOM: env.REWAKE_ROOM,
      REWAKE_SESSION: env.REWAKE_SESSION,
      REWAKE_EPOCH: env.REWAKE_EPOCH,
    });
  }
});

test("an input that is no positive integer or an empty text runs nothing", async ($, on) => {
  const seen = stage(on, ok(""));
  await start($);
  const refused = [
    await call($, briefReadToolName, { id: 0 }),
    await call($, briefReadToolName, { id: "3" }),
    await call($, briefTickToolName, { id: 3, item: 1.5, edition: 2 }),
    await call($, briefTickToolName, { id: 3, item: 1 }),
    await call($, briefTickToolName, { id: 3, item: 1, edition: 2, undo: "yes" }),
    await call($, briefCommentToolName, { id: 3, text: "  " }),
  ];
  for (const answer of refused as any[]) expect(answer.isError).toBe(true);
  expect(seen.runs).toEqual([]);
});

test("a refusal of the command is an error naming it", async ($, on) => {
  stage(on, () => ({
    value: {
      exitCode: 1,
      stdout: "",
      stderr: "brief 3 is at edition 3, not 2: main edited it since you read it",
    },
  }));
  await start($);
  const refused: any = await call($, briefTickToolName, { id: 3, item: 1, edition: 2 });
  expect(refused.isError).toBe(true);
  expect(refused.result).toContain("rewake brief tick failed (exit 1)");
  expect(refused.result).toContain("edition 3, not 2");
});
