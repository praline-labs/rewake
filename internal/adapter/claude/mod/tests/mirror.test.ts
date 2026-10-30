import { expect, test } from "claude-code/testing";
import { MirrorOutbox } from "../hooks/mirror.ts";
import { Permits, reasonOf, type Verdict } from "../hooks/permits.ts";
import { fit, jsonSize, pieces, utf8Size } from "../hooks/sizes.ts";
import { bounds } from "../protocol.gen.ts";
import type { Reply } from "./world.ts";
import { holdMs, start, world } from "./world.ts";

const drawn = { status: 200, text: '{"role":"write","generation":1,"mirror":true}' };

const mirrored = (run: ReturnType<typeof world>) =>
  run.sent.filter((one) => one.path === "/v1/mirror/events").flatMap((one) => one.body.events);

// A host that never answers: every batch stays in flight, as a stalled
// wrapper's would.
function stalled() {
  const posted: any[] = [];
  const host: any = {
    post: (_path: string, body: any) => {
      posted.push(body);
      return new Promise(() => {});
    },
    after: () => {},
  };
  return { host, posted };
}

test("a text is cut to its budget with its head and tail, and split into pieces within it", () => {
  const text = "HEAD" + '\u0001"é😀'.repeat(20000) + "TAIL";
  const kept = fit(text, 4096);
  expect(jsonSize(kept) <= 4096).toBe(true);
  expect(kept.startsWith("HEAD") && kept.endsWith("TAIL")).toBe(true);
  const parts = pieces(text, 1000);
  expect(parts.join("")).toBe(text);
  expect(parts.every((part) => jsonSize(part) <= 1000)).toBe(true);
});

test("an outbox past its bound drops deltas first, then settles to lost markers, and never passes the bound", () => {
  const { host } = stalled();
  const out = new MirrorOutbox(host, () => undefined);
  const delta = "d".repeat(30000);
  for (let i = 0; i < 20; i++) out.push({ kind: "delta", id: "b" + i, text: delta });
  expect(out.bytes <= bounds.maxMirrorOutboxBytes).toBe(true);
  for (let i = 0; i < 20; i++) {
    out.push({ kind: "settle", id: "s" + i, part: 0, of: 1, text: delta });
    expect(out.bytes <= bounds.maxMirrorOutboxBytes).toBe(true);
  }
  const queue: any[] = (out as any).queue;
  expect(queue.some((q) => q.event.kind === "delta")).toBe(false);
  expect(queue.filter((q) => q.event.kind === "lost").map((q) => q.event.id)).toContain("s0");
  expect(queue.at(-1).event.id).toBe("s19");
});

test("markers past their reserve collapse into one resync that counts what went", () => {
  const { host } = stalled();
  const out = new MirrorOutbox(host, () => undefined);
  for (let i = 0; i < 1000; i++)
    out.push({ kind: "turn", turn: "t" + i, state: i % 2 ? "idle" : "working" });
  expect(out.bytes <= bounds.mirrorMarkerBytes).toBe(true);
  const queue: any[] = (out as any).queue;
  expect(queue[0].event.kind).toBe("resync");
  expect(queue[0].event.lost > 0).toBe(true);
  expect(queue.filter((q) => q.event.kind === "resync").length).toBe(1);
});

test("a batch that does not answer is dropped after 5 s and stands as lost behind a resync", async ($, on) => {
  const batches: any[] = [];
  const run = world(on, {
    replies: {
      "/v1/hello": [drawn],
      "/v1/mirror/events": [
        (body: any) => {
          batches.push(body);
          return new Promise(() => {});
        },
        async (body: any) => {
          batches.push(body);
          return { status: 200, text: "{}" };
        },
      ],
    },
  });
  await start($);
  await run.clock.settle();
  await $.turn.start({ turnId: "a" } as any);
  await run.clock.advance(50);
  expect(batches.length).toBe(1);
  await $.turn.start({ turnId: "b" } as any);
  await run.clock.advance(1000);
  expect(batches.length).toBe(1);
  await run.clock.advance(5000);
  await run.clock.advance(50);
  expect(batches.length).toBe(2);
  expect(batches[1].events).toEqual([
    { kind: "turn", turn: "b", state: "working" },
    { kind: "resync", lost: 1 },
  ]);
});

test("a step of a drawn run yields every chunk unchanged and sends its deltas, its settle and its tool", async ($, on) => {
  const run = world(on, {
    replies: { "/v1/hello": [drawn], "/v1/mirror/events": [{ status: 200, text: "{}" }] },
  });
  const chunks = [
    { kind: "text", index: 0, text: "Hel" },
    { kind: "text", index: 0, text: "lo" },
    { kind: "tool", index: 1, id: "call-1", name: "Bash" },
    { kind: "input", index: 1, json: '{"command":' },
    { kind: "input", index: 1, json: '"ls"}' },
    { kind: "stop", stopReason: "tool_use", usage: null },
  ];
  on("turn.step", async function* (_: any, e: any) {
    for (const chunk of chunks) yield chunk;
    return {
      turnId: e.turnId,
      index: e.index,
      answer: "",
      toolUses: [],
      stopReason: "tool_use",
      usage: null,
    };
  });
  on("tool.call", () => ({ result: "a\nb", text: "a\nb" }));
  await start($);
  await run.clock.settle();
  await $.turn.start({ turnId: "t" } as any);
  const stream = $.turn.step({
    turnId: "t",
    index: 0,
    model: "test-model",
    messageCount: 1,
  } as any);
  const seen: any[] = [];
  for (let next = await stream.next(); next.done !== true; next = await stream.next())
    seen.push(next.value);
  expect(seen).toEqual(chunks);
  await $.tool.call({ tool: "Bash", command: "ls", tool_use_id: "call-1" } as any);
  await run.clock.advance(100);
  const events = mirrored(run);
  expect(events.map((e: any) => e.kind)).toEqual([
    "turn",
    "delta",
    "delta",
    "tool",
    "settle",
    "input",
    "result",
  ]);
  expect(events[4]).toMatchObject({ id: "t.0.0", part: 0, of: 1, text: "Hello" });
  expect(events[5]).toMatchObject({ id: "call-1", text: '{"command":"ls"}' });
  expect(events[6]).toMatchObject({ status: "done", text: "a\nb" });
});

test("a run the face does not draw sends nothing of its conversation", async ($, on) => {
  const run = world(on);
  on("turn.step", async function* (_: any, e: any) {
    yield { kind: "text", index: 0, text: "Hello" };
    return {
      turnId: e.turnId,
      index: e.index,
      answer: "",
      toolUses: [],
      stopReason: "end_turn",
      usage: null,
    };
  });
  await start($);
  await run.clock.settle();
  const stream = $.turn.step({
    turnId: "t",
    index: 0,
    model: "test-model",
    messageCount: 1,
  } as any);
  for (let next = await stream.next(); next.done !== true; next = await stream.next()) {}
  await run.clock.advance(100);
  expect(mirrored(run)).toEqual([]);
});

// The face's prompt, as main's commands come, on the heartbeat's answer.
function prompting(run: () => any, text: string): Reply[] {
  const command = {
    id: "face-1",
    action: "prompt",
    from: "face",
    focus: "",
    conversation: "conversation-1",
    text,
  };
  return [
    { status: 200, text: JSON.stringify({ control: command }) },
    async () => {
      await run().clock.sleep(holdMs);
      return { status: 200, text: "{}" };
    },
  ];
}

for (const drop of [undefined, "a Stop stands"]) {
  test(`a prompt of the face is mirrored once the engine ${drop ? "drops it, as refused" : "takes it in"}`, async ($, on) => {
    const run: any = world(on, {
      replies: {
        "/v1/hello": [drawn],
        "/v1/mod/alive": prompting(() => run, "go on"),
        "/v1/mirror/events": [{ status: 200, text: "{}" }],
      },
    });
    on("prompt.submit", (_: any, e: any) => (drop ? { drop } : { text: e.text }));
    await start($);
    await run.clock.settle();
    await run.clock.advance(100);
    const prompts = mirrored(run).filter((e: any) => e.kind === "prompt");
    expect(prompts.length).toBe(1);
    expect(prompts[0].origin).toBe("face");
    expect(prompts[0].status).toBe(drop ? "refused" : undefined);
    expect(prompts[0].text.startsWith("go on")).toBe(true);
    if (drop) expect(prompts[0].text).toContain(drop);
  });
}

// Each chunk of a tool's input ends with its number, so the tail of the
// input the face gets says how much of it the mod held.
const chunk = (char: string, n: number) => `${char.repeat(1024)}<${n}>`;

function heldOf(char: string, text: string): number {
  const last = /<(\d+)>$/.exec(text);
  let held = 0;
  for (let n = 0; last && n <= Number(last[1]); n++) held += utf8Size(chunk(char, n));
  return held;
}

for (const char of ["x", "界"]) {
  test(`a step's open tool inputs count against its bound with its blocks, in bytes (${char})`, async ($, on) => {
    const run = world(on, {
      replies: { "/v1/hello": [drawn], "/v1/mirror/events": [{ status: 200, text: "{}" }] },
    });
    on("turn.step", async function* (_: any, e: any) {
      for (let i = 0; i < 10; i++) yield { kind: "tool", index: i, id: `call-${i}`, name: "Write" };
      for (let i = 0; i < 10; i++)
        for (let n = 0; n < 256; n++) yield { kind: "input", index: i, json: chunk(char, n) };
      yield { kind: "stop", stopReason: "tool_use", usage: null };
      return {
        turnId: e.turnId,
        index: e.index,
        answer: "",
        toolUses: [],
        stopReason: "tool_use",
        usage: null,
      };
    });
    await start($);
    await run.clock.settle();
    const stream = $.turn.step({
      turnId: "t",
      index: 0,
      model: "test-model",
      messageCount: 1,
    } as any);
    for (let next = await stream.next(); next.done !== true; next = await stream.next()) {}
    for (let i = 0; i < 20; i++) await run.clock.advance(50);
    const inputs = mirrored(run).filter((e: any) => e.kind === "input");
    const held = inputs.map((e: any) => heldOf(char, e.text));
    // Each input holds at most 256 KiB, and all of them the step's 1 MiB.
    expect(held.every((bytes: number) => bytes <= bounds.maxMirrorBlockBytes)).toBe(true);
    const total = held.reduce((a: number, b: number) => a + b, 0);
    expect(total <= 4 * bounds.maxMirrorBlockBytes).toBe(true);
    expect(total > 3 * bounds.maxMirrorBlockBytes).toBe(true);
    expect(inputs.length).toBe(10);
  });
}

// A call's result carries how it was let run or refused, and the mode the
// harness reports reaches the face once, when the hello's answer is in.
test("a permit tells who let a call run or refused it, as the terminal program tells it", () => {
  const permits = new Permits();
  const check = (tool: string, input: object, id: string, verdict: Verdict, cwd?: string) =>
    permits.checked({ tool, input, tool_use_id: id } as any, verdict, cwd);
  const ask = { decision: "ask" };
  check("Read", { file_path: "/work/a" }, "r", { decision: "allow" });
  check("Glob", { pattern: "*" }, "g", { decision: "deny", rule: "Glob" });
  check("Bash", { command: "pre" }, "h", { decision: "allow", hook: "pre" });
  check("Bash", { command: "make" }, "b", ask, "/work");
  check("Edit", { file_path: "/work/a" }, "e", ask, "/work");
  check("Edit", { file_path: "/workshop/a" }, "o", ask, "/work");
  check("WebFetch", { url: "u" }, "w", ask, "/work");
  check("WebFetch", { url: "v" }, "d", ask, "/work");
  check("Bash", { command: "rm" }, "p", ask, "/work");
  check("Bash", { command: "rm -r" }, "q", ask, "/work");
  check("Bash", { command: "ls" }, "x", ask, "/work");
  check("Bash", { command: "ls" }, "y", ask, "/work");
  permits.denied("d", "out of scope");
  permits.dialog({ tool_name: "Bash", tool_input: { command: "rm" } } as any, "person");
  permits.answered("x", "rewake", true);
  permits.answered("y", "rewake", false);
  const refusal =
    "Permission for this action was denied by the Claude Code auto mode classifier. Reason: [Scope]. If you have other tasks, go on.";
  const take = (id: string, ended: object = {}, mode = "auto") => permits.take(id, ended, mode);
  expect(take("r")).toEqual({ by: "mode", allowed: true });
  expect(take("g", { refused: "no" })).toEqual({ by: "rule", allowed: false });
  expect(take("h")).toEqual({ by: "hook", allowed: true });
  expect(take("b")).toEqual({ by: "classifier", allowed: true });
  expect(take("e")).toEqual({ by: "mode", allowed: true });
  expect(take("o")).toEqual({ by: "classifier", allowed: true });
  expect(take("w", { failed: refusal })).toEqual({
    by: "classifier",
    allowed: false,
    reason: "[Scope]",
  });
  expect(take("d", { refused: "denied" })).toEqual({
    by: "classifier",
    allowed: false,
    reason: "out of scope",
  });
  expect(take("p")).toEqual({ by: "person", allowed: true });
  expect(
    take("q", {
      failed: "The user doesn't want to proceed with this tool use. The tool use was rejected.",
    }),
  ).toEqual({
    by: "person",
    allowed: false,
  });
  expect(take("x")).toEqual({ by: "rewake", allowed: true });
  expect(take("y", { refused: "refused" })).toEqual({ by: "rewake", allowed: false });
  expect(take("r")).toBeUndefined();
  check("Bash", { command: "make" }, "m", ask, "/work");
  expect(take("m", {}, "default")).toEqual({ by: "mode", allowed: true });
  permits.checked({ tool: "Bash", input: {}, tool_use_id: "s", agentId: "a1" } as any, ask);
  expect(take("s")).toBeUndefined();
  expect(reasonOf("Permission for this action has been denied. Reason: x")).toEqual({});
  expect(reasonOf("ok")).toBeUndefined();
});

test("a result carries its permit, and the mode goes to the face once as it changes", async ($, on) => {
  const run = world(on, {
    replies: { "/v1/hello": [drawn], "/v1/mirror/events": [{ status: 200, text: "{}" }] },
  });
  on("tool.check", (_: any, e: any) =>
    e.tool === "Read"
      ? { decision: "allow" }
      : e.tool === "Glob"
        ? { decision: "deny", rule: "Glob" }
        : { decision: "ask" },
  );
  on("session.cwd", () => ({ value: "/work" }));
  on("classic.SessionStart", () => ({}));
  on("classic.PostToolUse", () => ({}));
  // The test's dialog stands for the person, allowing.
  on("classic.PermissionRequest", () => ({ decision: { behavior: "allow" } }));
  on("tool.call", () => ({ result: "ok", text: "ok" }));
  await $.classic.SessionStart({ source: "startup", permission_mode: "auto" } as any);
  await start($);
  await run.clock.settle();
  await $.tool.call({ tool: "Read", file_path: "/work/a", tool_use_id: "r" } as any);
  await $.tool.call({ tool: "Glob", pattern: "*", tool_use_id: "g" } as any);
  await $.tool.call({ tool: "Bash", command: "make", tool_use_id: "b" } as any);
  await $.classic.PostToolUse({
    tool_name: "Bash",
    tool_use_id: "b",
    permission_mode: "auto",
  } as any);
  await $.classic.PostToolUse({
    tool_name: "Bash",
    tool_use_id: "z",
    permission_mode: "plan",
  } as any);
  await run.clock.advance(100);
  const events = mirrored(run);
  expect(events.filter((e: any) => e.kind === "mode")).toEqual([
    { kind: "mode", mode: "auto" },
    { kind: "mode", mode: "plan" },
  ]);
  const permits = Object.fromEntries(
    events.filter((e: any) => e.kind === "result").map((e: any) => [e.id, e.permit]),
  );
  expect(permits).toEqual({
    r: { by: "mode", allowed: true },
    g: { by: "rule", allowed: false },
    b: { by: "person", allowed: true },
  });
});
