// The face's mirror of a pi session, run by go test (worktree_tool_test.go).
import { expect, test } from "bun:test";
import { cases } from "../../../face/src/call.cases.ts";
import { facePrompt } from "./faced.ts";
import { bounds, type MirrorEvent } from "./feed.ts";
import { Mirror } from "./mirror.ts";
import { serveMirror } from "./mirrorhooks.ts";
import { facedTool, resultText } from "./toolmap.ts";

// Each of pi's tools as the face reads it: every form toolmap.ts makes is a
// row of the face's table of its vocabulary (face/src/call.cases.ts), where
// what the face draws of it is said; a listing is the shell's ls, as Claude
// Code lists a directory, so it folds as a listed directory.
const forms: [string, unknown][] = [
  ["bash", { command: "make", timeout: 5 }],
  ["read", { path: "a.go", offset: 2 }],
  ["write", { path: "a", content: "x" }],
  ["edit", { path: "a", edits: [{ oldText: "x", newText: "y" }] }],
  [
    "edit",
    {
      path: "a",
      edits: [
        { oldText: "x", newText: "y" },
        { oldText: "p", newText: "q" },
      ],
    },
  ],
  // A call's input while it streams is whatever parsed so far.
  ["edit", undefined],
  ["grep", { pattern: "x", path: "src", ignoreCase: true }],
  ["find", { pattern: "*.ts", path: "src", limit: 3 }],
  ["ls", { path: "src" }],
  ["ls", { path: "my dir" }],
  ["ls", {}],
  ["brief_tick", { n: 1 }],
];

test("pi's tools are named and given as rows of the face's vocabulary, another as it is", () => {
  for (const [tool, args] of forms) {
    const faced = facedTool(tool, args);
    const row = cases.find((c) => c.tool === faced.name && Bun.deepEquals(c.input, faced.input));
    expect([tool, args, row ? "a row" : faced]).toEqual([tool, args, "a row"]);
  }
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
  expect(events[3]).toMatchObject({ block: "text", part: 0, of: 1, turn: run });
  expect(events[9].status).toBe("done");
  expect(events.at(-1)?.turn).toBe(run);
});

// Seen live (October 31, 2026, brief 49): at minimal thinking every step of
// pi's opened an empty thinking block, a signature only, and the face drew a
// `∴ Thinking` row for each where the Claude Code worker's drew none.
test("a thinking block shows only when it has something in it", () => {
  const { events, emit } = mirrored();
  emit("agent_start", {});
  emit("turn_start", { turnIndex: 0 });
  const thinking = (type: string, contentIndex: number, more = {}) =>
    emit("message_update", update({ type, contentIndex, ...more }));
  // Empty: nothing at all, nor a whitespace-only end.
  thinking("thinking_start", 0);
  thinking("thinking_end", 0, { content: "" });
  thinking("thinking_start", 1);
  thinking("thinking_end", 1, { content: " \n" });
  // Streamed: its deltas and its settle; summed up at its end only: the settle.
  thinking("thinking_start", 2);
  thinking("thinking_delta", 2, { delta: "hmm" });
  thinking("thinking_end", 2, { content: "hmm" });
  thinking("thinking_start", 3);
  thinking("thinking_end", 3, { content: "a summary" });
  emit("agent_settled", { aborted: false });
  const run = events[0].turn;
  expect(events.map((e) => [e.kind, e.id, e.text ?? e.state])).toEqual([
    ["turn", undefined, "working"],
    ["delta", `${run}.0.0.2`, "hmm"],
    ["settle", `${run}.0.0.2`, "hmm"],
    ["settle", `${run}.0.0.3`, "a summary"],
    ["turn", undefined, "idle"],
  ]);
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

// Brief 56: pi tells no extension of its retries (auto_retry_* go to its own
// screen only); a run started again after a failed message is one. Its
// screen says each failure in red, then `Retry failed after N attempts` or
// `Aborted after N retry attempts`.
test("a failed message pi tries again is counted, and the retries' end said as pi says it", () => {
  const notices = (events: MirrorEvent[]) =>
    events.filter((e) => e.kind === "notice").map((e) => e.text);
  const failed = { message: { role: "assistant", stopReason: "error", errorMessage: "503" } };
  const exhausted = mirrored();
  exhausted.emit("agent_start", {});
  for (let i = 0; i < 3; i++) {
    exhausted.emit("message_end", failed);
    exhausted.emit("agent_start", {});
  }
  exhausted.emit("message_end", failed);
  exhausted.emit("agent_settled", { aborted: false });
  expect(notices(exhausted.events)).toEqual([
    "Error: 503",
    "Error: 503",
    "Error: 503",
    "Error: 503",
    "Error: Retry failed after 3 attempts: 503",
  ]);
  const stopped = mirrored();
  stopped.emit("agent_start", {});
  stopped.emit("message_end", failed);
  stopped.emit("agent_start", {});
  stopped.emit("message_end", { message: { role: "assistant", stopReason: "aborted" } });
  stopped.emit("agent_settled", { aborted: true });
  expect(notices(stopped.events)).toEqual(["Error: 503", "Aborted after 1 retry attempt"]);
  // A retry that came through, or a failure pi does not retry, says no more.
  const through = mirrored();
  through.emit("agent_start", {});
  through.emit("message_end", failed);
  through.emit("agent_start", {});
  through.emit("message_end", { message: { role: "assistant", stopReason: "stop" } });
  through.emit("agent_settled", { aborted: false });
  through.emit("agent_start", {});
  through.emit("message_end", failed);
  through.emit("agent_settled", { aborted: false });
  expect(notices(through.events)).toEqual(["Error: 503", "Error: 503"]);
});

// The count pi's screen shows as `Retrying (2/3)` is the turn's fact: each
// failed message says the attempt on a working turn event, a call it asked
// for or not, and the first item come back after it says none.
test("each failed message is said as the turn's retry attempt until something comes back", () => {
  const failed = { message: { role: "assistant", stopReason: "error", errorMessage: "503" } };
  const said = (events: MirrorEvent[]) =>
    events.filter((e) => e.kind === "turn").map((e) => `${e.state} ${e.retry ?? 0}`);
  const { events, emit } = mirrored();
  emit("agent_start", {});
  emit("message_end", failed);
  emit("agent_start", {});
  emit("turn_start", { turnIndex: 0 });
  const call = { type: "toolCall", id: "c", name: "bash", arguments: {} };
  emit(
    "message_update",
    update({ type: "toolcall_start", contentIndex: 0, partial: { content: [call] } }),
  );
  emit("message_end", { message: { ...failed.message, content: [{ type: "toolCall" }] } });
  emit("agent_start", {});
  emit("turn_start", { turnIndex: 0 });
  emit("message_update", update({ type: "text_delta", contentIndex: 0, delta: "ok" }));
  emit("message_update", update({ type: "text_delta", contentIndex: 0, delta: "!" }));
  emit("message_end", { message: { role: "assistant", stopReason: "stop" } });
  emit("agent_settled", { aborted: false });
  expect(said(events)).toEqual([
    "working 0",
    "working 1",
    "working 0",
    "working 2",
    "working 0",
    "idle 0",
  ]);
  // The attempt comes after the failure's notice, the reset before the item.
  const kinds = events.map((e) => e.kind);
  expect(kinds.slice(0, 4)).toEqual(["turn", "notice", "turn", "turn"]);
  expect(kinds.at(-4)).toBe("turn");
  expect(kinds.at(-3)).toBe("delta");
});

test("a long call's input and output stay within an event", () => {
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
  expect(events.filter((e) => e.kind === "input" || e.kind === "result").length).toBe(2);
  for (const e of events)
    expect(Buffer.byteLength(JSON.stringify(e))).toBeLessThanOrEqual(bounds.event);
});
