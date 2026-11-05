// Reproduced in brief 63: hello+wake in one chunk and a wake during
// asynchronous model selection. Exercise the actual wire dispatcher, not
// just the mode bridge; pending wakes remain the host's to retry.
import { expect, mock, test } from "bun:test";
import { mkdtemp, rm } from "node:fs/promises";
import { createServer, type Socket } from "node:net";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { ModePlan } from "./modes.ts";
import type { Frame } from "./protocol.ts";

mock.module("@earendil-works/pi-coding-agent", () => ({ CustomEditor: class {} }));
mock.module("@earendil-works/pi-tui", () => ({ matchesKey: () => false }));

async function until(check: () => boolean) {
  const end = Date.now() + 2000;
  while (!check() && Date.now() < end) await Bun.sleep(2);
  expect(check()).toBe(true);
}

const plan = (name: string, model: string): ModePlan => ({
  name,
  model,
  provider: "p",
  effort: "high",
  instructions: `go-owned ${name}`,
  status: `mode: ${name}`,
  busy: "go busy",
  failed: "go failed",
});

test("wire admission returns pending wakes and retains face prompts until initialization or selection commits", async () => {
  const dir = await mkdtemp(join(tmpdir(), "rw-mode-"));
  const socket = join(dir, "p.sock");
  const oldSocket = process.env.REWAKE_PI_SOCKET;
  process.env.REWAKE_PI_SOCKET = socket;
  let peer: Socket | undefined;
  let buffer = "";
  const received: Frame[] = [];
  const send = (frame: Frame) => peer?.write(`${JSON.stringify(frame)}\n`);
  const server = createServer((connected) => {
    peer = connected;
    connected.on("data", (data) => {
      buffer += data.toString();
      for (;;) {
        const end = buffer.indexOf("\n");
        if (end < 0) return;
        const frame = JSON.parse(buffer.slice(0, end)) as Frame;
        buffer = buffer.slice(end + 1);
        received.push(frame);
        if (frame.op === "answer") continue;
        const answer: Frame = { op: "answer", id: frame.id, ok: true };
        if (frame.op === "hello") {
          answer.mode = plan("review", "first");
          answer.mirror = true;
          // One write preserves the reviewer's exact startup ordering.
          connected.write(
            `${JSON.stringify(answer)}\n${JSON.stringify({
              op: "wake",
              id: 100,
              thread: "thread",
              notice: "mail",
            })}\n`,
          );
        } else {
          if (frame.op === "mode-cycle") answer.mode = plan("build", "second");
          send(answer);
        }
      }
    });
  });
  await new Promise<void>((resolve) => server.listen(socket, resolve));
  const hooks = new Map<string, ((event: unknown, ctx: unknown) => unknown)[]>();
  const emit = async (name: string, event: unknown) => {
    for (const hook of hooks.get(name) ?? []) await hook(event, ctx);
  };
  let model = { provider: "p", id: "first" };
  let level = "low";
  let hold = Promise.withResolvers<void>();
  let selected = Promise.withResolvers<void>();
  const calls: { text: string; model: string; level: string }[] = [];
  const take = (text: string) => calls.push({ text, model: model.id, level });
  const native = {
    on: (name: string, hook: (event: unknown, ctx: unknown) => unknown) => {
      hooks.set(name, [...(hooks.get(name) ?? []), hook]);
    },
    getThinkingLevel: () => level,
    setThinkingLevel: (next: string) => {
      level = next;
    },
    setModel: async (next: typeof model) => {
      model = next;
      level = "medium";
      await emit("model_select", {});
      selected.resolve();
      await hold.promise;
      return true;
    },
    sendMessage: (message: { content: string }) => take(message.content),
    sendUserMessage: (text: string) => take(text),
  };
  const ctx = {
    mode: "rpc",
    hasUI: false,
    isIdle: () => true,
    get model() {
      return model;
    },
    get thinkingLevel() {
      return level;
    },
    getContextUsage: () => null,
    sessionManager: { getSessionId: () => "thread" },
    modelRegistry: { find: (provider: string, id: string) => ({ provider, id }) },
  };
  const answer = (id: number) => received.find((frame) => frame.op === "answer" && frame.id === id);
  try {
    const { default: extension } = await import("./extension.ts");
    extension(native as never);
    const starting = emit("session_start", {});
    await selected.promise;
    await until(() => !!answer(100));
    expect(answer(100)?.pending).toBe(true);
    expect(calls).toEqual([]);
    send({ op: "prompt", id: 101, thread: "thread", text: "initial prompt" });
    await Bun.sleep(5);
    expect(answer(101)).toBeUndefined();
    hold.resolve();
    await starting;
    await until(() => !!answer(101));
    expect(calls).toEqual([{ text: "initial prompt", model: "first", level: "high" }]);
    send({ op: "wake", id: 102, thread: "thread", notice: "mail" });
    await until(() => !!answer(102));
    expect(answer(102)?.ok).toBe(true);
    expect(calls.filter((call) => call.text === "mail")).toHaveLength(1);

    hold = Promise.withResolvers<void>();
    selected = Promise.withResolvers<void>();
    send({ op: "mode-key", id: 103, thread: "thread" });
    await selected.promise;
    send({ op: "wake", id: 104, thread: "thread", notice: "later mail" });
    send({ op: "prompt", id: 105, thread: "thread", text: "later prompt" });
    send({ op: "thinking", id: 106, thread: "thread", levels: ["off", "low", "high"] });
    await until(() => !!answer(104) && !!answer(106));
    expect(answer(104)?.pending).toBe(true);
    expect(answer(106)?.error).toBe("go busy");
    expect(answer(105)).toBeUndefined();
    expect(calls).toHaveLength(2);
    hold.resolve();
    await until(() => !!answer(103) && !!answer(105));
    expect(answer(103)?.ok).toBe(true);
    send({ op: "wake", id: 107, thread: "thread", notice: "later mail" });
    await until(() => !!answer(107));
    expect(answer(107)?.ok).toBe(true);
    expect(calls.slice(2)).toEqual([
      { text: "later prompt", model: "second", level: "high" },
      { text: "later mail", model: "second", level: "high" },
    ]);
  } finally {
    hold.resolve();
    await emit("session_shutdown", { reason: "quit" });
    peer?.destroy();
    await new Promise<void>((resolve) => server.close(() => resolve()));
    if (oldSocket === undefined) delete process.env.REWAKE_PI_SOCKET;
    else process.env.REWAKE_PI_SOCKET = oldSocket;
    await rm(dir, { recursive: true, force: true });
  }
});
