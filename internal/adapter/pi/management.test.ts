// What a state carries of pi, run by go test (worktree_tool_test.go): what the
// session read, unconverted, which the wrapper projects (native.go).
import { expect, test } from "bun:test";
import { management } from "./management.ts";

function staged(ctx: object) {
  const handlers = new Map<string, (e: unknown) => unknown>();
  const pi = { on: (name: string, handle: (e: unknown) => unknown) => handlers.set(name, handle) };
  const sent: any[] = [];
  const link = { ready: true, thread: "conversation" };
  const managed = management(
    pi as never,
    () => ctx as never,
    () => link as never,
    () => undefined,
    (_link, fields) => sent.push(fields),
  );
  return { managed, sent, emit: (name: string, e: unknown) => handlers.get(name)?.(e) };
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
