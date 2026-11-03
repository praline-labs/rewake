import { expect, mock, test } from "claude-code/testing";
import type { Reply } from "./world.ts";
import { complete, holdMs, start, world } from "./world.ts";

function command(action: string, fields: Record<string, unknown> = {}) {
  return {
    id: "request-1",
    action,
    from: "main",
    focus: "keep the remaining plan",
    conversation: "conversation-1",
    ...fields,
  };
}

function orders(run: any, value: object): Reply[] {
  return [
    { status: 200, text: JSON.stringify({ control: value }) },
    async () => {
      await run().clock.sleep(holdMs);
      return { status: 200, text: "{}" };
    },
  ];
}

const messages = [{ role: "user", text: "kept summary", toolUses: [] }];

const results = (run: any) =>
  run.sent.filter((one: any) => one.path === "/v1/mod/control").map((one: any) => one.body);
const states = (run: any) =>
  run.sent.filter((one: any) => one.path === "/v1/mod/state").map((one: any) => one.body);

test("what the session read is sampled initially, on measurement and after the main turn, as read", async ($, on) => {
  let usage: any = { window: 200000, tokens: 50000, percent: 25 };
  const run = world(on, { usage: () => usage });
  on("session.measure", () => ({ changed: ["context"] }));
  await start($);
  await run.clock.settle();
  expect(states(run).at(-1)).toMatchObject({
    native: {
      usage: { context: { tokens: 50000, window: 200000, percent: 25 }, rateLimits: [] },
      model: "test-model",
      step: {},
    },
    activity: "idle",
  });
  usage = { window: 200000 };
  await $.session.measure({ context: usage, rateLimits: [], changed: ["context"] } as any);
  await run.clock.settle();
  expect(states(run).at(-1).native.usage.context).toEqual({ window: 200000 });
  await $.turn.start({ turnId: "t" } as any);
  await run.clock.settle();
  expect(states(run).at(-1).activity).toBe("working");
  await complete($, "t");
  await run.clock.settle();
  expect(states(run).at(-1).activity).toBe("idle");
});

// Brief 60: a sample names where the session runs now, which a worktree
// entry moves, and the account's windows; the host converts them (brief 94).
test("a sample carries the session's directory and the account's rate limits as read", async ($, on) => {
  let cwd = "/work";
  let limits: unknown[] = [];
  const run = world(on, { cwd: () => cwd, limits: () => limits });
  await start($);
  await run.clock.settle();
  expect(states(run).at(-1).native.cwd).toBe("/work");
  expect(states(run).at(-1).native.usage.rateLimits).toEqual([]);
  cwd = "/work/.claude/worktrees/wt";
  limits = [
    { kind: "five_hour", percentUsed: 37.4, resetsAt: "2026-10-31T13:00:00Z" },
    { kind: "seven_day", percentUsed: 9 },
  ];
  await $.turn.start({ turnId: "t" } as any);
  await run.clock.settle();
  expect(states(run).at(-1).native).toMatchObject({
    cwd: "/work/.claude/worktrees/wt",
    usage: { rateLimits: limits },
  });
  expect(Object.keys(states(run).at(-1))).not.toContain("rateLimits");
});

test("remote compact carries focus, returns requested while the heartbeat continues, then counts completion once", async ($, on) => {
  const run = world(on, { replies: { "/v1/mod/alive": orders(() => run, command("compact")) } });
  const calls: any[] = [];
  on("session.compact", async (_: any, e: any) => {
    calls.push(e);
    await run.clock.sleep(45000);
    return { messages, tokensBefore: 25000, tokensAfter: 1000 };
  });
  await start($);
  await run.clock.settle();
  expect(calls.length).toBe(1);
  expect(calls[0].instructions).toBe("keep the remaining plan");
  expect(results(run).map((one: any) => one.outcome)).toEqual(["requested"]);
  await run.clock.advance(30000);
  expect(run.polls().length).toBe(4);
  expect(states(run).at(-1).compacting).toBe(true);
  await run.clock.advance(15000);
  expect(results(run).map((one: any) => one.outcome)).toEqual(["requested", "done"]);
  expect(results(run).at(-1).native).toEqual({ tokensBefore: 25000, tokensAfter: 1000 });
  expect(states(run).at(-1)).toMatchObject({
    compactions: 1,
    compacting: false,
    request: "request-1",
    from: "main",
  });
});

test("compact refuses a running turn without calling native compact", async ($, on) => {
  const run = world(on, { replies: { "/v1/mod/alive": orders(() => run, command("compact")) } });
  let calls = 0;
  on("session.compact", () => {
    calls++;
    return { messages };
  });
  await start($);
  await $.turn.start({ turnId: "t" } as any);
  await run.clock.settle();
  expect(calls).toBe(0);
  expect(results(run).at(-1)).toMatchObject({ outcome: "refused", code: "turn" });
});

test("native compaction counts only a main conversation completion, and a skip counts nothing", async ($, on) => {
  const run = world(on);
  let skip = false;
  on("session.compact", () =>
    skip ? { skip: "nothing to compact" } : { messages, tokensBefore: 25000, tokensAfter: 1000 },
  );
  await start($);
  await $.session.compact({ trigger: "manual", messages } as any);
  await run.clock.settle();
  expect(states(run).at(-1).compactions).toBe(1);
  await $.session.compact({ trigger: "auto", messages, agentId: "child" } as any);
  skip = true;
  await $.session.compact({ trigger: "manual", messages } as any);
  await run.clock.settle();
  expect(states(run).at(-1)).toMatchObject({
    compactions: 1,
    compacting: false,
    request: "",
    from: "",
  });
});

// The mod aborts the turn the host names and answers requested; the host
// matches the end it hears on the outbox and sends the actor back as a trace.
test("interrupt aborts the turn the host names, and a trace is appended to native history", async ($, on) => {
  const run = world(on, {
    replies: {
      "/v1/mod/alive": [
        ...orders(() => run, command("interrupt", { turn: "t" })),
        ...orders(
          () => run,
          command("trace", {
            id: "trace-1",
            text: "main interrupted this turn with rewake interrupt",
          }),
        ),
      ],
    },
  });
  const history = mock.session(on);
  const aborts: string[] = [];
  on("turn.abort", (_: any, e: any) => {
    aborts.push(e.turnId);
    return { value: undefined };
  });
  await start($);
  await $.turn.start({ turnId: "t" } as any);
  await run.clock.settle();
  expect(aborts).toEqual(["t"]);
  expect(results(run)).toEqual([
    expect.objectContaining({ id: "request-1", outcome: "requested" }),
  ]);
  expect(states(run).at(-1).turn).toBe("t");
  await complete($, "t", { reason: "aborted", answer: "" });
  expect(run.ends().at(-1)!.body).toMatchObject({ turnId: "t", reason: "aborted" });
  expect(states(run).at(-1).turn).toBeUndefined();
  await run.clock.advance(holdMs);
  expect(results(run).at(-1)).toMatchObject({ id: "trace-1", outcome: "done" });
  expect(JSON.stringify(history.appended())).toContain(
    "main interrupted this turn with rewake interrupt",
  );
});

test("an interrupt naming a turn that is not the running one aborts nothing", async ($, on) => {
  const run = world(on, {
    replies: { "/v1/mod/alive": orders(() => run, command("interrupt", { turn: "old" })) },
  });
  let calls = 0;
  on("turn.abort", () => {
    calls++;
    return { value: undefined };
  });
  await start($);
  await $.turn.start({ turnId: "t" } as any);
  await run.clock.settle();
  expect(calls).toBe(0);
  expect(results(run).at(-1)).toMatchObject({ outcome: "refused", code: "turn" });
});

for (const action of ["interrupt", "compact"]) {
  test(`${action} refuses a different selected conversation`, async ($, on) => {
    const run = world(on, {
      replies: { "/v1/mod/alive": orders(() => run, command(action, { conversation: "other" })) },
    });
    await start($);
    await run.clock.settle();
    expect(results(run).at(-1)).toMatchObject({ outcome: "refused", code: "conversation" });
  });
}

test("interrupt without a main turn is refused without aborting", async ($, on) => {
  const run = world(on, { replies: { "/v1/mod/alive": orders(() => run, command("interrupt")) } });
  let calls = 0;
  on("turn.abort", () => {
    calls++;
    return { value: undefined };
  });
  await start($);
  await run.clock.settle();
  expect(calls).toBe(0);
  expect(results(run).at(-1)).toMatchObject({ outcome: "refused", code: "idle" });
});

test("an interrupt whose abort is refused answers what it said, and frees the guard", async ($, on) => {
  const run = world(on, {
    replies: {
      "/v1/mod/alive": [
        ...orders(() => run, command("interrupt")),
        ...orders(() => run, command("interrupt", { id: "request-2" })),
      ],
    },
  });
  let refuse = true;
  on("turn.abort", () => (refuse ? { deny: "no such turn" } : { value: undefined }));
  await start($);
  await $.turn.start({ turnId: "t" } as any);
  await run.clock.settle();
  expect(results(run).at(-1)).toMatchObject({
    outcome: "failed",
    native: { error: expect.stringContaining("no such turn") },
  });
  refuse = false;
  await run.clock.advance(holdMs);
  expect(results(run).at(-1)).toMatchObject({ id: "request-2", outcome: "requested" });
});

test("a skipped remote compaction has a final refusal and no completion count", async ($, on) => {
  const run = world(on, { replies: { "/v1/mod/alive": orders(() => run, command("compact")) } });
  on("session.compact", () => ({ skip: "nothing to compact" }));
  await start($);
  await run.clock.settle();
  expect(results(run).map((one: any) => one.outcome)).toEqual(["requested", "done"]);
  expect(results(run).at(-1).native).toEqual({ skip: "nothing to compact" });
  expect(states(run).at(-1)).toMatchObject({ compactions: 0, compacting: false });
});

test("effort is the main loop's latest request's, and each main-loop step is sampled after its response; a subagent's is not the session's", async ($, on) => {
  const run = world(on);
  on("turn.step", async function* (_: any, e: any) {
    yield { kind: "text", index: 0, text: "" };
    return {
      turnId: e.turnId,
      index: e.index,
      answer: "",
      toolUses: [],
      stopReason: "end_turn",
      usage: null,
    };
  });
  const step = async (fields: object) => {
    const stream = $.turn.step({
      turnId: "t",
      index: 0,
      model: "test-model",
      messageCount: 1,
      ...fields,
    } as any);
    let next = await stream.next();
    while (next.done !== true) next = await stream.next();
  };
  await start($);
  await run.clock.settle();
  expect(states(run).at(-1).native.step.effort).toBeUndefined();
  const before = states(run).length;
  await step({ effort: "max", agentId: "child" });
  await run.clock.settle();
  expect(states(run).length).toBe(before);
  await step({ effort: "low" });
  await run.clock.settle();
  const count = states(run).length;
  expect(states(run).at(-1).native.step.effort).toBe("low");
  await step({ effort: "low", index: 1 });
  await run.clock.settle();
  expect(states(run).length).toBe(count + 1);
  expect(states(run).at(-1).native.step.effort).toBe("low");
  await step({ effort: "high", index: 2 });
  await run.clock.settle();
  expect(states(run).at(-1).native.step.effort).toBe("high");
});

test("a sample carries the last main-loop step's usage as reported; a subagent's leaves it", async ($, on) => {
  const run = world(on);
  let usage: any = null;
  on("turn.step", async function* (_: any, e: any) {
    yield { kind: "text", index: 0, text: "" };
    return {
      turnId: e.turnId,
      index: e.index,
      answer: "",
      toolUses: [],
      stopReason: "end_turn",
      usage,
    };
  });
  const step = async (fields: object) => {
    const stream = $.turn.step({
      turnId: "t",
      index: 0,
      model: "test-model",
      messageCount: 1,
      ...fields,
    } as any);
    let next = await stream.next();
    while (next.done !== true) next = await stream.next();
  };
  await start($);
  await run.clock.settle();
  expect(states(run).at(-1).native.step.usage).toBeUndefined();
  usage = {
    model: "m",
    input_tokens: 50,
    output_tokens: 9,
    cache_read_input_tokens: 900,
    cache_creation_input_tokens: 50,
  };
  const reported = usage;
  await step({});
  await run.clock.settle();
  expect(states(run).at(-1).native.step.usage).toEqual(reported);
  usage = { ...usage, cache_read_input_tokens: 1 };
  await step({ agentId: "child" });
  await run.clock.settle();
  expect(states(run).at(-1).native.step.usage).toEqual(reported);
  usage = null;
  await step({ index: 1 });
  await run.clock.settle();
  expect(states(run).at(-1).native.step.usage).toBeNull();
});

test("a prompt of rewake's face is submitted as the person's from the clock and answered done", async ($, on) => {
  const run = world(on, {
    replies: {
      "/v1/mod/alive": orders(
        () => run,
        command("prompt", { from: "face", focus: "", text: "go on" }),
      ),
    },
  });
  const submitted: any[] = [];
  on("prompt.submit", (_: any, e: any) => {
    submitted.push(e);
    return { text: e.text };
  });
  await start($);
  await run.clock.settle();
  expect(submitted.map((e) => [e.text, e.origin.asUser])).toEqual([["go on", true]]);
  expect(results(run).at(-1)).toMatchObject({ id: "request-1", outcome: "done" });
});
