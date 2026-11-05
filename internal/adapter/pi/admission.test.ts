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
  let nextPlan = plan("build", "second");
  let stoppedStartup = false;
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
          const opening: Frame[] = [
            answer,
            { op: "wake", id: 100, thread: "thread", notice: "mail" },
          ];
          if (stoppedStartup)
            opening.push({ op: "prompt", id: 202, thread: "thread", text: "stopped prompt" });
          // One write preserves the reviewer's exact startup ordering.
          connected.write(`${opening.map((frame) => JSON.stringify(frame)).join("\n")}\n`);
        } else {
          if (frame.op === "mode-cycle") answer.mode = nextPlan;
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
  let refuseSelection = false;
  let restoring = false;
  const restored = Promise.withResolvers<void>();
  const restoreHold = Promise.withResolvers<void>();
  let thread = "thread";
  let idle = true;
  let shutdowns = 0;
  let aborts = 0;
  let compactions = 0;
  let completeCompact: (() => void) | undefined;
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
      if (restoring) {
        restored.resolve();
        await restoreHold.promise;
        restoring = false;
        return true;
      }
      selected.resolve();
      await hold.promise;
      if (refuseSelection) {
        refuseSelection = false;
        restoring = true;
        return false;
      }
      return true;
    },
    sendMessage: (message: { content: string }) => take(message.content),
    sendUserMessage: (text: string) => take(text),
  };
  const ctx = {
    mode: "rpc",
    hasUI: false,
    isIdle: () => idle,
    shutdown: () => shutdowns++,
    abort: () => aborts++,
    compact: (options: { onComplete: (result: unknown) => void }) => {
      compactions++;
      completeCompact = () => options.onComplete({ tokensBefore: 12, estimatedTokensAfter: 4 });
    },
    get model() {
      return model;
    },
    get thinkingLevel() {
      return level;
    },
    getContextUsage: () => null,
    sessionManager: { getSessionId: () => thread },
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
    send({ op: "control", id: 108, thread, action: "compact" });
    send({ op: "control", id: 109, thread, action: "interrupt" });
    await until(() => !!answer(104) && !!answer(106) && !!answer(108) && !!answer(109));
    for (const id of [108, 109])
      expect(answer(id)).toMatchObject({ outcome: "refused", code: "busy" });
    expect(compactions).toBe(0);
    expect(aborts).toBe(0);
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

    // A native refusal may happen after mutation. The old target must be
    // revalidated only after restoration, not when its prompt first arrived.
    hold = Promise.withResolvers<void>();
    selected = Promise.withResolvers<void>();
    nextPlan = plan("review", "first");
    refuseSelection = true;
    send({ op: "mode-key", id: 110, thread });
    await selected.promise;
    hold.resolve();
    await restored.promise;
    send({ op: "prompt", id: 111, thread, text: "stale prompt" });
    send({ op: "control", id: 112, thread, action: "compact" });
    send({ op: "control", id: 113, thread, action: "interrupt" });
    send({ op: "wake", id: 114, thread, notice: "restoring mail" });
    await until(() => !!answer(112) && !!answer(113) && !!answer(114));
    for (const id of [112, 113])
      expect(answer(id)).toMatchObject({ outcome: "refused", code: "busy" });
    expect(answer(114)?.pending).toBe(true);
    expect(answer(111)).toBeUndefined();
    expect(compactions).toBe(0);
    expect(aborts).toBe(0);
    thread = "replacement";
    restoreHold.resolve();
    await until(() => !!answer(110) && !!answer(111));
    expect(answer(110)?.error).toBe("go failed");
    expect(answer(111)).toMatchObject({ outcome: "refused", code: "conversation" });
    expect(calls).toHaveLength(4);
    thread = "thread";

    // Management still owns its pending compact and its result callbacks.
    send({ op: "control", id: 120, thread, action: "compact" });
    await until(() => compactions === 1);
    send({ op: "wake", id: 121, thread, notice: "compacting mail" });
    send({ op: "prompt", id: 122, thread, text: "compacting prompt" });
    await until(() => !!answer(121) && !!answer(122));
    expect(answer(121)).toMatchObject({ pending: true, error: "compaction in progress" });
    expect(answer(122)?.error).toBe(
      "pi is compacting its context; send it again once that is done",
    );
    expect(calls).toHaveLength(4);
    completeCompact?.();
    await until(() => !!answer(120));
    expect(answer(120)?.outcome).toBe("done");
    send({ op: "wake", id: 123, thread, notice: "compacting mail" });
    await until(() => !!answer(123));
    expect(answer(123)?.ok).toBe(true);
    expect(calls).toHaveLength(5);

    await emit("session_shutdown", { reason: "quit" });
    hooks.clear();
    stoppedStartup = true;
    idle = false;
    extension(native as never);
    await emit("session_start", {});
    await until(() => peer?.destroyed === true);
    expect(shutdowns).toBe(1);
    expect(answer(202)).toBeUndefined();
    expect(calls).toHaveLength(5);
  } finally {
    hold.resolve();
    restoreHold.resolve();
    await emit("session_shutdown", { reason: "quit" });
    peer?.destroy();
    await new Promise<void>((resolve) => server.close(() => resolve()));
    if (oldSocket === undefined) delete process.env.REWAKE_PI_SOCKET;
    else process.env.REWAKE_PI_SOCKET = oldSocket;
    await rm(dir, { recursive: true, force: true });
  }
});
