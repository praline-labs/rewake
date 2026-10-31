// The person's Esc in the face and the run it stops, run by go test
// (worktree_tool_test.go): pi's own screen ends an aborted or failed message
// so (dist/modes/interactive/interactive-mode.js, message_end, and
// components/assistant-message.js, read on 1.4.0), and the face draws it the same.
import { expect, test } from "bun:test";
import { faceInterrupt } from "./faced.ts";
import { Mirror, serveMirror } from "./mirror.ts";
import type { MirrorEvent } from "./outbox.ts";

function mirrored() {
  const events: MirrorEvent[] = [];
  const handlers = new Map<string, ((e: unknown, ctx: unknown) => unknown)[]>();
  const pi = {
    on: (name: string, handle: (e: unknown, ctx: unknown) => unknown) =>
      handlers.set(name, [...(handlers.get(name) ?? []), handle]),
  };
  const mirror = new Mirror({ push: (e) => events.push(e) });
  mirror.on = true;
  serveMirror(pi as never, mirror);
  const emit = (name: string, e: unknown) =>
    (handlers.get(name) ?? []).map((handle) => handle(e, { isIdle: () => false }));
  const update = (assistantMessageEvent: unknown) =>
    emit("message_update", { assistantMessageEvent });
  return { mirror, events, emit, update };
}

const seen = (events: MirrorEvent[]) =>
  events.map((e) => [e.kind, e.id ?? "", e.status ?? e.state ?? "", e.text ?? e.name ?? ""]);

test("Esc stops a working pi through its own abort, and leaves an idle one alone", () => {
  const { mirror } = mirrored();
  let aborted = 0;
  const pi = (idle: boolean) => ({ isIdle: () => idle, abort: () => aborted++ });
  expect(faceInterrupt(mirror, pi(false), false)).toEqual({ ok: true });
  expect(aborted).toBe(1);
  expect(faceInterrupt(mirror, pi(true), false).error).toContain("not working");
  expect(faceInterrupt(mirror, pi(false), true).error).toContain("compacting");
  const throwing = {
    isIdle: () => false,
    abort: () => {
      throw new Error("x");
    },
  };
  expect(faceInterrupt(mirror, throwing, false).error).toBeDefined();
  mirror.on = false;
  expect(faceInterrupt(mirror, pi(false), false).error).toContain("does not draw");
  expect(aborted).toBe(1);
});

test("a reply stopped mid-stream keeps what streamed and says Operation aborted in red", () => {
  const { events, emit, update } = mirrored();
  emit("agent_start", {});
  emit("turn_start", { turnIndex: 0 });
  update({ type: "thinking_delta", contentIndex: 0, delta: "hm" });
  update({ type: "text_delta", contentIndex: 1, delta: "Half a" });
  emit("message_end", {
    message: {
      role: "assistant",
      stopReason: "aborted",
      content: [
        { type: "thinking", thinking: "hmm" },
        { type: "text", text: "Half a sentence" },
      ],
    },
  });
  emit("agent_settled", { aborted: true });
  const run = events[0].turn;
  expect(seen(events)).toEqual([
    ["turn", "", "working", ""],
    ["delta", `${run}.0.0.0`, "", "hm"],
    ["delta", `${run}.0.0.1`, "", "Half a"],
    ["settle", `${run}.0.0.0`, "", "hmm"],
    ["settle", `${run}.0.0.1`, "", "Half a sentence"],
    ["notice", events[5].id ?? "", "error", "Operation aborted"],
    ["turn", "", "error", ""],
  ]);
});

test("calls an aborted or failed message asked for end with its reason, a running one with its own", () => {
  const { events, emit, update } = mirrored();
  emit("agent_start", {});
  emit("turn_start", { turnIndex: 0 });
  const call = (id: string) => ({ type: "toolCall", id, name: "bash", arguments: {} });
  update({ type: "toolcall_start", contentIndex: 0, partial: { content: [call("ran")] } });
  emit("message_end", { message: { role: "assistant", stopReason: "toolUse", content: [] } });
  emit("tool_execution_start", {
    toolCallId: "ran",
    toolName: "bash",
    args: { command: "sleep 30" },
  });
  emit("tool_execution_end", {
    toolCallId: "ran",
    result: { content: [{ type: "text", text: "Command aborted" }] },
    isError: true,
    durationMs: 5000,
  });
  emit("turn_start", { turnIndex: 1 });
  update({ type: "toolcall_start", contentIndex: 0, partial: { content: [call("asked")] } });
  emit("message_end", {
    message: { role: "assistant", stopReason: "aborted", content: [call("asked")] },
  });
  // A failed message with no call says pi's error, as its screen does.
  emit("turn_start", { turnIndex: 2 });
  emit("message_end", {
    message: {
      role: "assistant",
      stopReason: "error",
      errorMessage: "This operation was aborted",
      content: [],
    },
  });
  const ended = events.filter((e) => e.kind === "result" || e.kind === "notice");
  expect(ended.map((e) => [e.kind, e.id, e.status, e.text])).toEqual([
    ["result", "ran", "error", "Command aborted"],
    ["result", "asked", "error", "Operation aborted"],
    ["notice", ended[2].id, "error", "Error: This operation was aborted"],
  ]);
});

// Review of b1f3f12: an Esc during a running call ends with the call's error
// and no notice; the stop is the turn's end saying so, not a guess from what
// was drawn. An ordinary tool error does not stop the turn.
test("a turn's end says it was stopped only when pi settled aborted", () => {
  const ended = (aborted: boolean) => {
    const { events, emit } = mirrored();
    emit("agent_start", {});
    emit("turn_start", { turnIndex: 0 });
    emit("tool_execution_start", { toolCallId: "c1", toolName: "bash", args: { command: "x" } });
    emit("tool_execution_end", {
      toolCallId: "c1",
      result: { content: [{ type: "text", text: "boom" }] },
      isError: true,
      durationMs: 1,
    });
    emit("agent_settled", { aborted });
    return events.at(-1);
  };
  expect(ended(true)).toMatchObject({ kind: "turn", state: "idle", status: "error" });
  expect(ended(false)).toEqual({ kind: "turn", turn: expect.any(String), state: "idle" });
});
