import { expect, test } from "claude-code/testing";
import { fit, unanswered } from "../hooks/tools.ts";
import { bounds } from "../protocol.gen.ts";
import { epoch, holdMs, type Reply, socket, start, world } from "./world.ts";

const env = { REWAKE_MOD_SOCKET: socket, REWAKE_EPOCH: epoch };
const schema = { type: "object", properties: {}, additionalProperties: false };
const status = { name: "worktree_status", description: "Say which worktree.", inputSchema: schema };
const tick = {
  name: "brief_tick",
  description: "Tick an item.",
  inputSchema: {
    type: "object",
    properties: { id: { type: "integer" } },
    additionalProperties: false,
  },
};
const hello = (tools: object[], digest: string) => ({
  status: 200,
  text: JSON.stringify({ role: "write", generation: 1, tools, toolsDigest: digest }),
});
const ok = (body: object): Reply => ({ status: 200, text: JSON.stringify(body) });
const plan = ok({
  argv: ["/r/rewake", "worktree", "status", "--json"],
  env: [{ name: "REWAKE_SESSION", value: "api" }],
  timeoutMs: 60_000,
});

// Stands up the run with the endpoint's replies; records what is
// registered, run, entered and invalidated, the harness's command ending
// with ran and its EnterWorktree with entered.
function stage(
  on: any,
  given: {
    env?: Record<string, string>;
    replies?: Record<string, Reply[]>;
    ran?: () => any;
    entered?: () => any;
    root?: string;
  } = {},
) {
  const run = world(on, {
    env: given.env ?? env,
    replies: { "/v1/hello": [hello([status], "a")], "/v1/tool/plan": [plan], ...given.replies },
  });
  const seen = {
    registered: [] as any[],
    runs: [] as any[],
    entries: [] as any[],
    invalidated: [] as string[],
  };
  on("tool.register", (_: any, e: any) => {
    seen.registered.push(e);
    return { value: { tool: "mcp__rewake__" + e.name } };
  });
  on("ui.invalidate", (_: any, e: any) => {
    seen.invalidated.push(e.event ?? e);
    return { value: undefined };
  });
  on("session.cwd", () => ({ value: "/work/tree" }));
  on("session.root", () => ({ value: given.root ?? "/work/tree" }));
  on("process.run", (_: any, e: any) => {
    seen.runs.push(e);
    return (given.ran ?? (() => ({ value: { exitCode: 0, stdout: "{}\n", stderr: "" } })))();
  });
  on("tool.call", (_: any, e: any) => {
    seen.entries.push(e);
    return (given.entered ?? (() => ({ result: {}, text: "Switched." })))();
  });
  const bodies = (path: string) =>
    run.sent.filter((one) => one.path === path).map((one) => one.body);
  return { run, seen, bodies };
}

const call = ($: any, tool = "mcp__rewake__worktree_status", input: object = {}) =>
  $.tool.call({ tool, ...input } as any);

test("a hello names the protocol and the run's entry, and its table is registered by turn one", async ($, on) => {
  const { seen, run } = stage(on, {
    env: { ...env, REWAKE_WORKTREE_ENTER: "1" },
    replies: { "/v1/hello": [hello([status, tick], "a")] },
  });
  await start($);
  expect(run.hellos()[0].body.protocol).toBe(bounds.protocol);
  expect(run.hellos()[0].body.enter).toBe(true);
  expect(seen.registered).toEqual([status, tick]);
});

test("a run without entry says none", async ($, on) => {
  const { run } = stage(on);
  await start($);
  expect(run.hellos()[0].body.enter).toBeUndefined();
});

test("a call is planned by the wrapper, run natively as planned, and its raw end worded by the wrapper", async ($, on) => {
  const { seen, bodies } = stage(on, {
    replies: { "/v1/tool/result": [ok({ text: "worded", isError: false })] },
  });
  await start($);
  const answer: any = await call($, "mcp__rewake__brief_tick", { id: 3, item: 1 });
  expect(bodies("/v1/tool/plan")[0].tool).toBe("brief_tick");
  expect(bodies("/v1/tool/plan")[0].arguments).toEqual({ id: 3, item: 1 });
  expect(seen.runs).toHaveLength(1);
  expect(seen.runs[0].argv).toEqual(["/r/rewake", "worktree", "status", "--json"]);
  expect(seen.runs[0].init).toEqual({
    cwd: "/work/tree",
    env: { REWAKE_SESSION: "api" },
    timeoutMs: 60_000,
  });
  expect(bodies("/v1/tool/result")[0].ran).toEqual({ stdout: "{}\n", stderr: "", exitCode: 0 });
  expect(answer).toEqual({ result: "worded" });
});

test("a refused plan runs nothing and answers the wrapper's refusal as an error", async ($, on) => {
  const { seen } = stage(on, {
    replies: { "/v1/tool/plan": [ok({ refused: "brief_tick needs id" })] },
  });
  await start($);
  const answer: any = await call($, "mcp__rewake__brief_tick");
  expect(answer).toEqual({ isError: true, result: "brief_tick needs id" });
  expect(seen.runs).toEqual([]);
});

test("a command that could not start, or whose output was cut, is posted unworded", async ($, on) => {
  const ends = [
    { deny: "spawn ENOENT" },
    { value: { exitCode: 0, stdout: "{", stderr: "", isStdoutTruncated: true } },
  ];
  const { bodies } = stage(on, {
    ran: () => ends.shift(),
    replies: { "/v1/tool/result": [ok({ text: "worded", isError: true })] },
  });
  await start($);
  expect(await call($)).toEqual({ isError: true, result: "worded" });
  await call($);
  const [unstarted, cut] = bodies("/v1/tool/result").map((body) => body.ran);
  expect(unstarted.stdout).toBe("");
  expect(unstarted.error).toContain("spawn ENOENT");
  expect(cut).toEqual({ stdout: "{", stderr: "", exitCode: 0, stdoutCut: true });
});

test("an output past the body's bound is cut at a whole character and marked cut", () => {
  const long = "é".repeat(bounds.maxEndBodyBytes);
  const kept = fit({ loadId: "l", tool: "t", ran: { stdout: long, stderr: "x".repeat(10_000) } });
  expect(new TextEncoder().encode(JSON.stringify(kept)).length).toBeLessThanOrEqual(
    bounds.maxEndBodyBytes,
  );
  expect(kept.ran.stdoutCut).toBe(true);
  expect(kept.ran.stdout.length).toBeGreaterThan(400_000);
  const small = { loadId: "l", tool: "t", ran: { stdout: "ok", stderr: "" } };
  expect(fit(small)).toBe(small);
});

test("a plan refused in a switch's freeze is sent again, and a wrapper that never answers gives the one fixed text", async ($, on) => {
  const { run, seen } = stage(on, {
    replies: {
      "/v1/tool/plan": [{ status: 503, text: '{"error":"handing over"}' }, plan, "throw"],
      "/v1/tool/result": [ok({ text: "worded", isError: false })],
    },
  });
  await start($);
  const first = call($);
  await run.clock.advance(1000);
  expect(await first).toEqual({ result: "worded" });
  expect(seen.runs).toHaveLength(1);
  const second = call($);
  await run.clock.advance(10_000);
  const failed: any = await second;
  expect(failed.isError).toBe(true);
  expect(failed.result.startsWith(unanswered(""))).toBe(true);
  expect(failed.result).toContain("the socket is gone");
  expect(seen.runs).toHaveLength(1);
});

test("a poll's new table reaches the next turn: a changed description is described again, a changed schema registered again", async ($, on) => {
  const changed = { ...status, description: "Say which worktree, now." };
  const reshaped = { ...tick, inputSchema: schema };
  const { run, seen } = stage(on, {
    replies: {
      "/v1/hello": [hello([status, tick], "a")],
      "/v1/mod/alive": [
        ok({ tools: [changed, reshaped], toolsDigest: "b" }),
        async () => {
          await run.clock.sleep(holdMs);
          return ok({});
        },
      ],
    },
  });
  on("tool.describe", (_: any, e: any) => ({ description: e.description }));
  await start($);
  await run.clock.settle();
  expect(run.polls()[0].body.toolsDigest).toBe("a");
  expect(run.polls()[1].body.toolsDigest).toBe("b");
  expect(seen.registered.map((one) => one.name)).toEqual([
    "worktree_status",
    "brief_tick",
    "brief_tick",
  ]);
  expect(seen.registered[2].inputSchema).toEqual(schema);
  expect(seen.invalidated).toContain("tool.describe");
  const described: any = await $.tool.describe({
    tool: "mcp__rewake__worktree_status",
    description: "Say which worktree.",
    provider: { plugin: "rewake", tier: "user" },
  } as any);
  expect(described.description).toBe(changed.description);
});

const entered = (path: string) =>
  ok({ text: '{"checkout":{"path":"' + path + '"}}', isError: false, enterWorktree: path });

test("worktree_enter's result moves the session by Claude Code's own EnterWorktree, and the wrapper words what came of it", async ($, on) => {
  const path = "/trees/fix";
  const { seen, bodies } = stage(on, {
    replies: {
      "/v1/tool/result": [entered(path)],
      "/v1/tool/native": [ok({ text: "entered, worded", isError: false })],
    },
  });
  await start($);
  const answer: any = await call($, "mcp__rewake__worktree_enter");
  expect(seen.entries.map((e) => [e.tool, e.path])).toEqual([["EnterWorktree", path]]);
  const [native] = bodies("/v1/tool/native");
  expect(native).toMatchObject({
    tool: "worktree_enter",
    path,
    inside: false,
    moved: { text: "Switched." },
  });
  expect(native.made).toContain(path);
  expect(answer).toEqual({ result: "entered, worded" });
});

test("a session already in the checkout is not moved again, and a refused move is posted as the refusal", async ($, on) => {
  const { seen, bodies } = stage(on, {
    root: "/trees/fix/sub",
    replies: {
      "/v1/tool/result": [entered("/trees/fix"), entered("/trees/other")],
      "/v1/tool/native": [ok({ text: "worded", isError: false })],
    },
    entered: () => ({ deny: "policy refuses" }),
  });
  await start($);
  await call($, "mcp__rewake__worktree_enter");
  await call($, "mcp__rewake__worktree_enter");
  const [inside, refused] = bodies("/v1/tool/native");
  expect(inside.inside).toBe(true);
  expect(inside.moved).toBeUndefined();
  expect(refused.moved).toEqual({ deny: "policy refuses" });
  expect(seen.entries).toHaveLength(1);
});
