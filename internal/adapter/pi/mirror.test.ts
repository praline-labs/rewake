// The face's mirror of a pi session, run by go test (worktree_tool_test.go).
import { expect, test } from "bun:test";
import { facePrompt, Mirror, serveMirror } from "./mirror.ts";
import { bounds, type Carrier, type MirrorEvent, Outbox } from "./outbox.ts";
import { facedTool, resultText } from "./toolmap.ts";

test("pi's tools are named and given as their Claude Code kin, another as it is", () => {
  expect(facedTool("bash", { command: "make", timeout: 5 })).toEqual({
    name: "Bash",
    input: { command: "make" },
  });
  expect(facedTool("read", { path: "a.go", offset: 2 })).toEqual({
    name: "Read",
    input: { file_path: "a.go", offset: 2 },
  });
  expect(facedTool("write", { path: "a", content: "x" })).toEqual({
    name: "Write",
    input: { file_path: "a", content: "x" },
  });
  expect(facedTool("edit", { path: "a", edits: [{ oldText: "x", newText: "y" }] })).toEqual({
    name: "Edit",
    input: { file_path: "a", old_string: "x", new_string: "y" },
  });
  expect(
    facedTool("edit", {
      path: "a",
      edits: [
        { oldText: "x", newText: "y" },
        { oldText: "p", newText: "q" },
      ],
    }),
  ).toEqual({
    name: "MultiEdit",
    input: {
      file_path: "a",
      edits: [
        { old_string: "x", new_string: "y" },
        { old_string: "p", new_string: "q" },
      ],
    },
  });
  expect(facedTool("grep", { pattern: "x", path: "src", ignoreCase: true })).toEqual({
    name: "Grep",
    input: { pattern: "x", path: "src", "-i": true },
  });
  expect(facedTool("find", { pattern: "*.ts", path: "src", limit: 3 })).toEqual({
    name: "Glob",
    input: { pattern: "*.ts", path: "src" },
  });
  expect(facedTool("ls", { path: "src" })).toEqual({ name: "Bash", input: { command: "ls src" } });
  expect(facedTool("ls", { path: "my dir" })).toEqual({
    name: "Bash",
    input: { command: "ls 'my dir'" },
  });
  expect(facedTool("ls", {})).toEqual({ name: "Bash", input: { command: "ls" } });
  expect(facedTool("brief_tick", { n: 1 })).toEqual({ name: "brief_tick", input: { n: 1 } });
  // A call's input while it streams is whatever parsed so far.
  expect(facedTool("edit", undefined)).toEqual({ name: "MultiEdit", input: { edits: [] } });
  expect(
    resultText({
      content: [{ type: "text", text: "a" }, { type: "image" }, { type: "text", text: "b" }],
    }),
  ).toBe("a\n[image]\nb");
});

// fakePi keeps the handlers an extension registers and runs them as pi does.
function fakePi() {
  const handlers = new Map<string, ((e: unknown, ctx: unknown) => unknown)[]>();
  const sent: { text: string; options: unknown }[] = [];
  const pi = {
    on: (name: string, handle: (e: unknown, ctx: unknown) => unknown) =>
      handlers.set(name, [...(handlers.get(name) ?? []), handle]),
    sendUserMessage: (text: string, options: unknown) => sent.push({ text, options }),
  };
  const emit = (name: string, e: unknown, ctx: unknown = { isIdle: () => true }) =>
    (handlers.get(name) ?? []).map((handle) => handle(e, ctx));
  return { pi: pi as never, emit, sent };
}

function mirrored() {
  const events: MirrorEvent[] = [];
  let now = 0;
  const mirror = new Mirror({ push: (e) => events.push(e) }, () => now);
  mirror.on = true;
  const { pi, emit, sent } = fakePi();
  serveMirror(pi, mirror);
  return { mirror, events, emit, pi, sent, tick: (ms: number) => (now += ms) };
}

const update = (assistantMessageEvent: unknown) => ({ assistantMessageEvent });

test("a turn streams its prompt, thinking, reply and calls in the face's words", () => {
  const { events, emit, tick } = mirrored();
  emit("input", { text: "look", source: "interactive" });
  emit("agent_start", {});
  emit("turn_start", { turnIndex: 1 });
  emit("message_update", update({ type: "thinking_start", contentIndex: 0 }));
  emit("message_update", update({ type: "thinking_end", contentIndex: 0, content: "" }));
  emit("message_update", update({ type: "text_delta", contentIndex: 1, delta: "Hi" }));
  emit("message_update", update({ type: "text_end", contentIndex: 1, content: "Hi there" }));
  const call = { type: "toolCall", id: "c1", name: "read", arguments: { path: "a" } };
  emit(
    "message_update",
    update({ type: "toolcall_start", contentIndex: 2, partial: { content: [{}, {}, call] } }),
  );
  tick(200);
  emit(
    "message_update",
    update({ type: "toolcall_delta", contentIndex: 2, partial: { content: [{}, {}, call] } }),
  );
  // Within the interval a delta sends nothing more.
  emit(
    "message_update",
    update({ type: "toolcall_delta", contentIndex: 2, partial: { content: [{}, {}, call] } }),
  );
  emit("message_update", update({ type: "toolcall_end", contentIndex: 2, toolCall: call }));
  emit("tool_execution_start", { toolCallId: "c1", toolName: "read", args: { path: "a" } });
  emit("tool_call", { toolCallId: "c1" });
  emit("tool_execution_end", {
    toolCallId: "c1",
    result: { content: [{ type: "text", text: "body" }] },
    isError: false,
    durationMs: 3,
  });
  emit("tool_execution_start", {
    toolCallId: "n1",
    toolName: "bash",
    args: {},
    parentToolCallId: "c1",
  });
  emit("agent_settled", { aborted: false });
  const run = events[1].turn;
  expect(events.map((e) => [e.kind, e.id, e.text ?? e.state ?? e.name])).toEqual([
    ["prompt", events[0].id, "look"],
    ["turn", undefined, "working"],
    ["delta", `${run}.0.1.0`, ""],
    ["settle", `${run}.0.1.0`, ""],
    ["delta", `${run}.0.1.1`, "Hi"],
    ["settle", `${run}.0.1.1`, "Hi there"],
    ["tool", "c1", "Read"],
    ["input", "c1", '{"file_path":"a"}'],
    ["input", "c1", '{"file_path":"a"}'],
    ["tool", "c1", "Read"],
    ["input", "c1", '{"file_path":"a"}'],
    ["result", "c1", "body"],
    ["turn", undefined, "idle"],
  ]);
  expect(events[0].origin).toBe("person");
  expect(events[0].turn).toBeUndefined();
  expect(events[3]).toMatchObject({ block: "thinking", part: 0, of: 1, turn: run });
  expect(events[11].status).toBe("done");
  expect(events.at(-1)?.turn).toBe(run);
});

test("a call a hook blocked ends refused, one pi's checks refused or an abort ends an error", () => {
  const { events, emit } = mirrored();
  emit("agent_start", {});
  const failed = { content: [{ type: "text", text: "not allowed" }] };
  emit("tool_call", { toolCallId: "blocked" });
  emit("tool_execution_end", { toolCallId: "blocked", result: failed, isError: true });
  emit("tool_execution_end", { toolCallId: "unknown", result: failed, isError: true });
  emit("tool_call", { toolCallId: "cut" });
  emit(
    "tool_execution_end",
    { toolCallId: "cut", result: failed, isError: true },
    { signal: { aborted: true } },
  );
  emit("tool_call", { toolCallId: "ran" });
  emit("tool_execution_end", { toolCallId: "ran", result: failed, isError: true, durationMs: 4 });
  const results = events.filter((e) => e.kind === "result");
  expect(results.map((e) => [e.id, e.status])).toEqual([
    ["blocked", "refused"],
    ["unknown", "error"],
    ["cut", "error"],
    ["ran", "error"],
  ]);
  expect(results[0].permit).toEqual({ by: "hook", allowed: false, reason: "not allowed" });
});

test("the face's prompt is pi's input from the face, steered while pi works", () => {
  const { mirror, events, emit, pi, sent } = mirrored();
  expect(facePrompt(pi, mirror, true, false, "go")).toEqual({ ok: true, steered: false });
  emit("input", { text: "go", source: "extension" });
  emit("agent_start", {});
  expect(facePrompt(pi, mirror, false, false, "more")).toEqual({ ok: true, steered: true });
  emit("input", { text: "more", source: "extension", streamingBehavior: "steer" });
  emit("input", { text: "other", source: "extension" });
  expect(sent).toEqual([
    { text: "go", options: undefined },
    { text: "more", options: { deliverAs: "steer" } },
  ]);
  const prompts = events.filter((e) => e.kind === "prompt");
  expect(prompts.map((e) => [e.text, e.origin, e.turn === undefined])).toEqual([
    ["go", "face", true],
    ["more", "face", false],
    ["other", "extension", true],
  ]);
  expect(facePrompt(pi, mirror, true, true, "x").error).toContain("compacting");
  expect(facePrompt(pi, mirror, true, false, " ").error).toBeDefined();
  mirror.on = false;
  expect(facePrompt(pi, mirror, true, false, "x").error).toContain("does not draw");
});

test("nothing is mirrored before the hello's answer says mirror, and a failure of its own is dropped", () => {
  const events: MirrorEvent[] = [];
  const mirror = new Mirror({ push: (e) => events.push(e) });
  const { pi, emit } = fakePi();
  serveMirror(pi, mirror);
  emit("input", { text: "look", source: "interactive" });
  expect(events).toEqual([]);
  const broken = new Mirror({
    push: () => {
      throw new Error("broken");
    },
  });
  broken.on = true;
  const second = fakePi();
  serveMirror(second.pi, broken);
  expect(second.emit("input", { text: "look", source: "interactive" })).toEqual([undefined]);
});

// carrier is a link the test answers by hand.
function carrier(ready = true) {
  const posted: MirrorEvent[][] = [];
  const answers: ((ok: boolean) => void)[] = [];
  const timers: (() => void)[] = [];
  const c: Carrier = {
    ready: () => ready,
    post: (events) => {
      posted.push(events);
      return new Promise((resolve) => answers.push(resolve));
    },
    after: (_ms, run) => timers.push(run),
  };
  const fire = () => {
    for (const run of timers.splice(0)) run();
  };
  return { c, posted, answers, fire };
}

test("the outbox sends one batch at a time and a failed batch stands as lost", async () => {
  const { c, posted, answers, fire } = carrier();
  const box = new Outbox(c);
  box.push({ kind: "delta", id: "b", text: "a" });
  box.push({ kind: "input", id: "c", text: "{}" });
  box.push({ kind: "input", id: "c", text: '{"a":1}' });
  fire();
  expect(posted.length).toBe(1);
  // A later input replaces one still queued.
  expect(posted[0].map((e) => e.text)).toEqual(["a", '{"a":1}']);
  box.push({ kind: "delta", id: "b", text: "b" });
  fire();
  answers[0](false);
  await Promise.resolve();
  fire();
  await Promise.resolve();
  expect(posted[1][0]).toEqual({ kind: "delta", id: "b", text: "b" });
  expect(posted[1][1]).toEqual({ kind: "resync", lost: 2 });
});

test("the outbox stays within its bound: deltas go first, then whole items behind a marker", () => {
  const { c } = carrier(false);
  const box = new Outbox(c);
  const text = "x".repeat(30000);
  box.push({ kind: "tool", id: "t", name: "Bash" });
  for (let i = 0; i < 20; i++) box.push({ kind: "delta", id: "b", text });
  expect(box.bytes).toBeLessThanOrEqual(bounds.outbox);
  for (let i = 0; i < 20; i++) box.push({ kind: "settle", id: `s${i}`, part: 0, of: 1, text });
  expect(box.bytes).toBeLessThanOrEqual(bounds.outbox);
});

test("each run pi starts within a turn has blocks of its own, a retry's never settling over an earlier step", () => {
  const { events, emit } = mirrored();
  emit("agent_start", {});
  emit("turn_start", { turnIndex: 0 });
  emit("message_update", update({ type: "text_end", contentIndex: 0, content: "first" }));
  // A retryable failure starts pi's loop again, its turns counted from 0.
  emit("agent_start", {});
  emit("turn_start", { turnIndex: 0 });
  emit("message_update", update({ type: "text_end", contentIndex: 0, content: "retried" }));
  emit("agent_settled", { aborted: false });
  const settles = events.filter((e) => e.kind === "settle");
  expect(settles.map((e) => e.text)).toEqual(["first", "retried"]);
  expect(settles[0].id).not.toBe(settles[1].id);
  expect(new Set(events.map((e) => e.turn)).size).toBe(1);
});

test("a long call's input and output stay within an event, and every batch within its cap", () => {
  const { events, emit } = mirrored();
  emit("agent_start", {});
  const content = "x".repeat(60000);
  emit("tool_execution_start", {
    toolCallId: "w",
    toolName: "write",
    args: { path: "a", content },
  });
  emit("tool_execution_end", {
    toolCallId: "w",
    result: { content: [{ type: "text", text: "é".repeat(60000) }] },
    isError: false,
    durationMs: 1,
  });
  for (const e of events)
    expect(Buffer.byteLength(JSON.stringify(e))).toBeLessThanOrEqual(bounds.event);
  const { c, posted, answers, fire } = carrier();
  const box = new Outbox(c);
  for (const e of events) box.push(e);
  // An event past the bound is never sent: its item is lost instead.
  box.push({ kind: "result", id: "big", status: "done", text: "y".repeat(bounds.batch) });
  for (let i = 0; i < 10; i++) {
    fire();
    answers.splice(0).map((answer) => answer(true));
  }
  expect(posted.length).toBeGreaterThan(1);
  for (const batch of posted)
    expect(Buffer.byteLength(JSON.stringify(batch))).toBeLessThanOrEqual(bounds.batch);
  const sent = posted.flat();
  expect(sent.some((e) => e.id === "big" && e.kind === "result")).toBe(false);
  expect(sent).toContainEqual({ kind: "lost", id: "big" });
});
