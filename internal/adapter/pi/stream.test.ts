// The stream of pi's own events to the wrapper, through its interface: pi's
// events in, batches with their parts and lost markers out. Run by go test
// (worktree_tool_test.go); what the wrapper makes of them is translate_test.go's.
import { expect, test } from "bun:test";
import { forward } from "./forward.ts";
import { bounds, type Forwarded, Stream } from "./stream.ts";

// streaming is a stream whose link posts at will: timers run when told, and
// each post waits for its answer.
function streaming(ready = true) {
  const timers: { ms: number; run: () => void }[] = [];
  const posted: Forwarded[][] = [];
  const answers: ((ok: boolean) => void)[] = [];
  const link = { ready };
  const stream = new Stream({
    ready: () => link.ready,
    post: (lines) => {
      posted.push(lines);
      return new Promise((resolve) => answers.push(resolve));
    },
    after: (ms, run) => timers.push({ ms, run }),
  });
  stream.on = true;
  // tick runs the send timers; a batch's 5 s bound runs only when its
  // answer is told.
  const tick = async () => {
    const due = timers.filter((t) => t.ms < 5000);
    timers.splice(0, timers.length, ...timers.filter((t) => t.ms >= 5000));
    for (const t of due) t.run();
    await Promise.resolve();
  };
  const answer = async (ok = true) => {
    answers.shift()?.(ok);
    await new Promise((resolve) => setTimeout(resolve, 0));
  };
  return { stream, posted, tick, answer, link };
}

// fakePi keeps the handlers an extension registers and runs them as pi does.
function fakePi() {
  const handlers = new Map<string, ((e: unknown, ctx: unknown) => unknown)[]>();
  const pi = {
    on: (name: string, handle: (e: unknown, ctx: unknown) => unknown) =>
      handlers.set(name, [...(handlers.get(name) ?? []), handle]),
  };
  const emit = (name: string, e: unknown, ctx: unknown = {}) =>
    (handlers.get(name) ?? []).map((handle) => handle(e, ctx));
  return { pi: pi as never, emit };
}

const wire = (value: unknown) => Buffer.byteLength(JSON.stringify(value));

test("pi's events go as pi gave them, less the partial message a piece grows", async () => {
  const { stream, posted, tick } = streaming();
  const { pi, emit } = fakePi();
  forward(pi, stream);
  const call = { type: "toolCall", id: "c1", name: "write", arguments: {} };
  const partial = { content: [{ type: "text", text: "so far" }, call] };
  emit("input", { text: "go", source: "interactive" });
  emit("turn_start", { turnIndex: 2 }, { hasPendingMessages: () => false });
  emit("message_update", {
    assistantMessageEvent: { type: "text_delta", contentIndex: 0, delta: "so", partial },
  });
  emit("message_update", {
    assistantMessageEvent: { type: "toolcall_start", contentIndex: 1, partial },
  });
  emit("message_update", {
    assistantMessageEvent: { type: "toolcall_delta", contentIndex: 1, delta: '{"pa', partial },
  });
  emit("message_end", {
    message: {
      role: "assistant",
      stopReason: "aborted",
      content: [{ type: "thinking", thinking: "hm", signature: "s" }, call],
      usage: { input: 1 },
    },
  });
  emit("message_end", { message: { role: "assistant", stopReason: "stop", content: [call] } });
  emit(
    "tool_execution_end",
    {
      toolCallId: "c1",
      result: {
        content: [{ type: "image", data: "x".repeat(1000), mimeType: "image/png" }],
        details: { diff: "d" },
      },
      isError: false,
      durationMs: 2,
    },
    { signal: { aborted: true } },
  );
  await tick();
  expect(posted).toEqual([
    [
      { event: "input", text: "go", source: "interactive" },
      { event: "turn_start", turnIndex: 2, pending: false },
      { event: "message_update", type: "text_delta", contentIndex: 0, delta: "so" },
      { event: "message_update", type: "toolcall_start", contentIndex: 1, block: call },
      { event: "message_update", type: "toolcall_delta", contentIndex: 1, delta: '{"pa' },
      {
        event: "message_end",
        role: "assistant",
        stopReason: "aborted",
        content: [{ type: "thinking", thinking: "hm" }, { type: "toolCall" }],
      },
      { event: "message_end", role: "assistant", stopReason: "stop" },
      {
        event: "tool_execution_end",
        toolCallId: "c1",
        result: { content: [{ type: "image" }] },
        isError: false,
        durationMs: 2,
        aborted: true,
      },
    ],
  ]);
});

test("nothing goes before the hello's answer says mirror, and a failure of its own is dropped", async () => {
  const { stream, posted, tick } = streaming();
  stream.on = false;
  const { pi, emit } = fakePi();
  forward(pi, stream);
  emit("agent_start", {});
  await tick();
  expect(posted).toEqual([]);
  stream.on = true;
  // A message_start without its message throws in the handler: pi's event
  // goes on as it came.
  expect(emit("message_start", {})).toEqual([undefined]);
  expect(emit("session_before_compact", { reason: "manual" })).toEqual([undefined]);
});

test("past its bound the oldest delta goes first, then the oldest event, each leaving a lost marker", async () => {
  const { stream, posted, answer, link } = streaming(false);
  const text = "x".repeat(30000);
  stream.push({ event: "agent_start" });
  for (let i = 0; i < 4; i++)
    stream.push({ event: "message_update", type: "text_delta", delta: text }, true);
  for (let i = 0; i < 10; i++)
    stream.push({ event: "message_update", type: "text_end", content: `${i}${text}` });
  expect(stream.bytes).toBeLessThanOrEqual(bounds.held);
  link.ready = true;
  for (let i = 0; i < 20 && stream.bytes > 0; i++) {
    void stream.send();
    await answer();
  }
  const lines = posted.flat();
  // Every delta went first, then agent_start and the two oldest ends, the
  // marker standing where they stood, in front of what was kept.
  expect(lines[0]).toEqual({ event: "lost", deltas: 4, events: 3 });
  expect(lines.slice(1).map((line) => String(line.content)[0])).toEqual([
    "2",
    "3",
    "4",
    "5",
    "6",
    "7",
    "8",
    "9",
  ]);
});

test("a long event goes as parts of its text, and one past its bound is cut", async () => {
  const { stream, posted, tick, answer } = streaming();
  const drain = async () => {
    for (let i = 0; i < 20 && stream.bytes > 0; i++) {
      await tick();
      await answer();
    }
  };
  const long = 'é"'.repeat(30000);
  stream.push({ event: "message_update", type: "text_end", content: long });
  await drain();
  const huge = "y".repeat(400000);
  stream.push({ event: "message_update", type: "text_end", content: huge });
  await drain();
  const lines = posted.flat();
  for (const batch of posted) expect(wire(batch)).toBeLessThanOrEqual(bounds.batch + 64);
  for (const line of lines) expect(wire(line)).toBeLessThanOrEqual(bounds.part);
  const events: Forwarded[] = [];
  let text = "";
  for (const line of lines) {
    expect(line.event).toBe("part");
    text += line.text;
    if (line.part === (line.of as number) - 1) {
      events.push(JSON.parse(text));
      text = "";
    }
  }
  expect(events[0].content).toBe(long);
  const cut = String(events[1].content);
  expect(cut).toContain("… cut …");
  expect(wire(events[1])).toBeLessThanOrEqual(bounds.event);
});

// Review of b832ce0: a call of many short hunks, which no string cut
// shortens, is held within its bound with its hunks' head; one nothing can
// cut goes as a lost marker, never past the bound.
test("an event of many short strings keeps its arrays' head within its bound, and one that cannot fit is lost", async () => {
  const { stream, posted, tick, answer } = streaming();
  const edits = Array.from({ length: 1500 }, (_, i) => ({
    oldText: "a".repeat(256) + i,
    newText: "b".repeat(256) + i,
  }));
  const args = { path: "big.ts", edits };
  stream.push(
    { event: "tool_execution_start", toolCallId: "e", toolName: "edit", args },
    false,
    bounds.part,
  );
  expect(stream.bytes).toBeLessThanOrEqual(bounds.part);
  const keys = Object.fromEntries(Array.from({ length: 20000 }, (_, i) => [`k${i}`, i]));
  stream.push({ event: "tool_execution_end", toolCallId: "e", result: keys }, false, bounds.part);
  expect(stream.bytes).toBeLessThanOrEqual(bounds.part + 64);
  for (let i = 0; i < 5 && stream.bytes > 0; i++) {
    await tick();
    await answer();
  }
  const lines = posted.flat();
  expect(lines.map((line) => line.event)).toEqual(["tool_execution_start", "lost"]);
  const sent = lines[0].args as typeof args;
  expect(sent.path).toBe("big.ts");
  expect(sent.edits.length).toBeGreaterThan(0);
  expect(sent.edits).toEqual(edits.slice(0, sent.edits.length));
  expect(lines[1]).toEqual({ event: "lost", deltas: 0, events: 1 });
});

test("one batch is in flight at a time, and one that fails stands as lost before what waits", async () => {
  const { stream, posted, tick, answer } = streaming();
  stream.push({ event: "agent_start" });
  await tick();
  stream.push({ event: "message_update", type: "text_delta", delta: "a" }, true);
  stream.push({ event: "turn_start", turnIndex: 1 });
  await tick();
  expect(posted.length).toBe(1);
  await answer(false);
  await tick();
  expect(posted.length).toBe(2);
  expect(posted[1]).toEqual([
    { event: "lost", deltas: 0, events: 1 },
    { event: "message_update", type: "text_delta", delta: "a" },
    { event: "turn_start", turnIndex: 1 },
  ]);
});
