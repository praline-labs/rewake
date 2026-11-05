// What a state carries of pi, run by go test (worktree_tool_test.go): what the
// session read, unconverted, which the wrapper projects (native.go).
import { expect, test } from "bun:test";
import { management } from "./management.ts";

function staged(ctx: object, running?: () => unknown) {
  const handlers = new Map<string, (e: unknown) => unknown>();
  const traced: unknown[] = [];
  const pi = {
    on: (name: string, handle: (e: unknown) => unknown) => handlers.set(name, handle),
    sendMessage: (message: unknown, options: unknown) => traced.push([message, options]),
  };
  const sent: any[] = [];
  const link = { ready: true, thread: "conversation" };
  const managed = management(
    pi as never,
    () => ctx as never,
    () => link as never,
    () => running?.() as never,
    () => undefined,
    (_link, fields) => sent.push(fields),
  );
  return {
    managed,
    sent,
    link,
    traced,
    emit: (name: string, e: unknown) => handlers.get(name)?.(e),
  };
}

const model = {
  id: "m",
  name: "Model",
  provider: "p",
  baseUrl: "https://host.example/v1?key=secret",
  headers: { authorization: "Bearer secret" },
  reasoning: true,
  contextWindow: 200000,
  promptCache: { short: 300, long: 3600 },
};

test("a sample is what pi answered, its model less the headers and address that may carry credentials", () => {
  const usage = { tokens: 42000, contextWindow: 200000, percent: 21 };
  const { managed } = staged({
    isIdle: () => true,
    getContextUsage: () => usage,
    model,
    thinkingLevel: "high",
  });
  const sample = managed.sample();
  expect(sample).toEqual({
    sequence: 1,
    idle: true,
    compacting: false,
    native: {
      contextUsage: usage,
      model: {
        id: "m",
        name: "Model",
        provider: "p",
        reasoning: true,
        contextWindow: 200000,
        promptCache: { short: 300, long: 3600 },
      },
      thinkingLevel: "high",
      lastAssistant: null,
    },
  });
  expect(JSON.stringify(sample)).not.toContain("secret");
});

test("each assistant message's end is sampled with its usage as reported, a placeholder's too; another role's is not", () => {
  const { managed, sent, emit } = staged({
    isIdle: () => false,
    getContextUsage: () => undefined,
    model: undefined,
  });
  const usage = { input: 50, output: 9, cacheRead: 900, cacheWrite: 50, totalTokens: 1009 };
  emit("message_end", { message: { role: "user", content: "hi" } });
  expect(sent).toEqual([]);
  const end = {
    role: "assistant",
    usage,
    provider: "p",
    model: "m",
    timestamp: 1_000,
    content: [],
  };
  emit("message_end", { message: end });
  const empty = { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, totalTokens: 0 };
  emit("message_end", { message: { ...end, usage: empty, stopReason: "error" } });
  expect(sent.map((one) => one.state.native.lastAssistant)).toEqual([
    { usage, provider: "p", model: "m", timestamp: 1_000 },
    { usage: empty, provider: "p", model: "m", timestamp: 1_000 },
  ]);
  expect(sent[0].state).toMatchObject({ idle: false, native: { contextUsage: null, model: null } });
  managed.reset();
  expect(managed.sample().native.lastAssistant).toBeNull();
});

// The extension carries a command out after its check and answers as pi
// resolved; the wrapper words it (control.go).
test("a command is refused by the code of its check, and carried out as pi resolves it", async () => {
  let idle = false;
  let aborted = 0;
  let compacting: { onComplete: (r: unknown) => void; onError: (e: Error) => void } | undefined;
  const ctx = {
    isIdle: () => idle,
    getContextUsage: () => undefined,
    abort: () => aborted++,
    compact: (options: never) => {
      compacting = options;
    },
  };
  let running: unknown;
  const { managed, link, sent, traced } = staged(ctx, () => running);
  const control = (frame: object) =>
    managed.control(
      link as never,
      { op: "control", id: 1, request: "r", from: "main", ...frame } as never,
    );
  expect(await control({ action: "compact" })).toEqual({ outcome: "refused", code: "turn" });
  expect(await control({ action: "interrupt" })).toEqual({ outcome: "refused", code: "idle" });
  running = { id: "t1", link };
  expect(await control({ action: "interrupt", turn: "t0" })).toEqual({
    outcome: "refused",
    code: "turn",
  });
  expect(await control({ action: "interrupt", turn: "t1" })).toEqual({ outcome: "requested" });
  expect(aborted).toBe(1);
  expect(await control({ action: "compact" })).toEqual({ outcome: "refused", code: "busy" });
  expect(managed.trace("main interrupted this turn with rewake interrupt")).toEqual({ ok: true });
  expect(traced).toEqual([
    [
      {
        customType: "rewake_interrupt",
        content: "main interrupted this turn with rewake interrupt",
        display: true,
      },
      { triggerTurn: false, deliverAs: "steer" },
    ],
  ]);
  managed.settled(running as never);
  running = undefined;
  idle = true;
  const compacted = control({ action: "compact" });
  compacting!.onComplete({
    summary: "the conversation",
    tokensBefore: 9000,
    estimatedTokensAfter: 900,
  });
  expect(await compacted).toEqual({
    outcome: "done",
    native: { tokensBefore: 9000, estimatedTokensAfter: 900 },
  });
  expect(JSON.stringify(sent)).not.toContain("the conversation");
  const failed = control({ action: "compact" });
  compacting!.onError(new Error("Nothing to compact"));
  expect(await failed).toEqual({ outcome: "failed", native: { error: "Nothing to compact" } });
  const closed = control({ action: "compact" });
  managed.reset();
  expect(await closed).toEqual({ outcome: "failed", code: "closed" });
});
