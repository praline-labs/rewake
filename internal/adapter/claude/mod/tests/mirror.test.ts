import { expect, test } from "claude-code/testing";
import { MirrorOutbox } from "../hooks/mirror.ts";
import { fit, jsonSize, pieces } from "../hooks/sizes.ts";
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
