import { expect, test } from "bun:test";
import { type ModePlan, Modes } from "./modes.ts";
import type { Frame } from "./protocol.ts";

type PromptEvent = { systemPromptOptions: { sections: Record<string, string> } };

function rig() {
  let idle = true;
  let model = { provider: "p", id: "first" };
  let effort = "low";
  let accepted = true;
  let shutdowns = 0;
  let aborts = 0;
  const status: string[] = [];
  const warnings: string[] = [];
  const handlers = new Map<string, (event: PromptEvent) => unknown>();
  const native = {
    on: (name: string, fn: (event: PromptEvent, ctx: unknown) => unknown) =>
      handlers.set(name, (event) => fn(event, ctx)),
    getThinkingLevel: () => effort,
    setThinkingLevel: (level: string) => {
      effort = level;
    },
    setModel: async (next: typeof model) => {
      if (!accepted) return false;
      model = next;
      effort = "medium";
      return true;
    },
  };
  const ctx = {
    isIdle: () => idle,
    shutdown: () => shutdowns++,
    abort: () => aborts++,
    get model() {
      return model;
    },
    hasUI: true,
    ui: { setStatus: (_key: string, text: string) => status.push(text) },
    modelRegistry: {
      find: (provider: string, id: string) => (id === "missing" ? undefined : { provider, id }),
    },
  };
  const asks: Omit<Frame, "id">[] = [];
  let destination: ModePlan | undefined;
  let changed = 0;
  const modes = new Modes(
    native as never,
    () => ctx as never,
    () => false,
    async (frame) => {
      asks.push(frame);
      return { op: "answer", id: 1, ok: true, mode: destination, levels: ["off", "low", "high"] };
    },
    () => ({ sequence: 1, idle, compacting: false, native: {} }) as never,
    () => changed++,
    (text) => warnings.push(text),
    async () => ({ outcome: "done" }),
  );
  return {
    modes,
    status,
    warnings,
    handlers,
    asks,
    native,
    values: () => ({ model, effort, changed }),
    stopped: () => ({ shutdowns, aborts }),
    idle: (value: boolean) => {
      idle = value;
    },
    accepts: (value: boolean) => {
      accepted = value;
    },
    select: (plan: ModePlan) => {
      destination = plan;
      return modes.cycle();
    },
  };
}
const plan = (name: string, extra: Partial<ModePlan> = {}): ModePlan => ({
  name,
  instructions: `go-owned ${name}`,
  status: `mode: ${name}`,
  busy: "go busy",
  failed: "go failed",
  ...extra,
});

test("the bridge applies Go's mode and keeps unset selections, including after a model change", async () => {
  const r = rig();
  await r.modes.initialize(plan("review"));
  expect(r.values()).toEqual({ model: { provider: "p", id: "first" }, effort: "low", changed: 1 });
  expect(await r.select(plan("advisor", { provider: "p", model: "second" }))).toEqual({ ok: true });
  expect(r.asks[0].op).toBe("mode-cycle");
  expect(r.values()).toEqual({ model: { provider: "p", id: "second" }, effort: "low", changed: 2 });
  expect(r.modes.name).toBe("advisor");
  expect(r.status).toEqual(["mode: review", "mode: advisor"]);
  // A reconnect's hello does not replace an already observed stance.
  await r.modes.initialize(plan("review"));
  expect(r.modes.name).toBe("advisor");
  const event = { systemPromptOptions: { sections: { existing: "kept" } } };
  await r.handlers.get("before_agent_start")?.(event);
  expect(event.systemPromptOptions.sections).toEqual({
    existing: "kept",
    rewake_work_mode: "go-owned advisor",
  });
  await r.select(plan("build", { effort: "high" }));
  expect(r.values().effort).toBe("high");
});

test("busy and failed registry/model selection leave all three live values unchanged", async () => {
  const r = rig();
  await r.modes.initialize(plan("review"));
  const before = r.values();
  r.idle(false);
  expect(await r.select(plan("build", { effort: "high" }))).toEqual({ error: "go busy" });
  r.idle(true);
  expect(
    await r.select(plan("build", { provider: "p", model: "missing", effort: "high" })),
  ).toEqual({ error: "go failed" });
  r.accepts(false);
  expect(await r.select(plan("build", { provider: "p", model: "second", effort: "high" }))).toEqual(
    { error: "go failed" },
  );
  expect(r.warnings).toEqual(["go busy", "go failed", "go failed"]);
  expect(r.values()).toEqual(before);
  expect(r.modes.name).toBe("review");
  expect(r.status).toEqual(["mode: review"]);
});

const prompt = () => ({ systemPromptOptions: { sections: {} as Record<string, string> } });

test("a native start waits for the initial instructions instead of running with no mode", async () => {
  const r = rig();
  const event = prompt();
  let started = false;
  const starting = Promise.resolve(r.handlers.get("before_agent_start")?.(event)).then(() => {
    started = true;
  });
  await Promise.resolve();
  expect(started).toBe(false);
  await r.modes.initialize(plan("review"));
  await starting;
  expect(event.systemPromptOptions.sections.rewake_work_mode).toBe("go-owned review");
});

test("native starts and compaction wait through asynchronous selection and see the committed stance", async () => {
  const r = rig();
  await r.modes.initialize(plan("review"));
  const selected = Promise.withResolvers<void>();
  const finish = Promise.withResolvers<void>();
  const setModel = r.native.setModel;
  r.native.setModel = async (next) => {
    await setModel(next);
    selected.resolve();
    await finish.promise;
    return true;
  };
  const switching = r.select(plan("build", { provider: "p", model: "second", effort: "high" }));
  await selected.promise;
  const event = prompt();
  let started = false;
  let compacted = false;
  const starting = Promise.resolve(r.handlers.get("before_agent_start")?.(event)).then(() => {
    started = true;
  });
  const compacting = Promise.resolve(r.handlers.get("session_before_compact")?.(prompt())).then(
    (result) => {
      compacted = true;
      return result;
    },
  );
  try {
    await Promise.resolve();
    expect(started).toBe(false);
    expect(compacted).toBe(false);
  } finally {
    finish.resolve();
    await switching;
    await starting;
  }
  expect(await compacting).toBeUndefined();
  expect(event.systemPromptOptions.sections.rewake_work_mode).toBe("go-owned build");
  expect(r.values().effort).toBe("high");
});

test("a refused selection restores native state even if the native model already changed", async () => {
  const r = rig();
  await r.modes.initialize(plan("review"));
  const before = r.values();
  const setModel = r.native.setModel;
  const restoring = Promise.withResolvers<void>();
  const finish = Promise.withResolvers<void>();
  r.native.setModel = async (next) => {
    await setModel(next);
    if (next.id === "second") return false;
    restoring.resolve();
    await finish.promise;
    return true;
  };
  const switching = r.select(plan("build", { provider: "p", model: "second", effort: "high" }));
  await restoring.promise;
  const event = prompt();
  const waiting = r.handlers.get("before_agent_start")?.(event);
  await Promise.resolve();
  expect(event.systemPromptOptions.sections).toEqual({});
  finish.resolve();
  expect(await switching).toEqual({ error: "go failed" });
  await waiting;
  expect(event.systemPromptOptions.sections.rewake_work_mode).toBe("go-owned review");
  expect(r.values().model).toEqual(before.model);
  expect(r.values().effort).toBe(before.effort);
  expect(r.modes.name).toBe("review");
});

test("busy initialization stops rather than dropping its plan and opening an uninitialized run", async () => {
  const r = rig();
  r.idle(false);
  const starting = r.handlers.get("before_agent_start")?.(prompt());
  const input = r.handlers.get("input")?.(prompt());
  const compacting = r.handlers.get("session_before_compact")?.(prompt());
  await r.modes.initialize(plan("review"));
  expect(r.warnings).toEqual(["go busy"]);
  await starting;
  expect(await input).toEqual({ action: "handled" });
  expect(await compacting).toEqual({ cancel: true });
  expect(r.stopped()).toEqual({ shutdowns: 1, aborts: 1 });
});

test("native and face thinking close admission before mutation and cannot overtake a mode selection", async () => {
  const r = rig();
  await r.modes.initialize(plan("review"));
  const setLevel = r.native.setThinkingLevel;
  const event = prompt();
  let preparing: unknown;
  r.native.setThinkingLevel = (level) => {
    preparing = r.handlers.get("before_agent_start")?.(event);
    setLevel(level);
    expect(event.systemPromptOptions.sections).toEqual({});
  };
  r.idle(false); // Thinking is not restricted to an idle turn.
  expect(await r.modes.thinking()).toEqual({ ok: true });
  await preparing;
  expect(r.values().effort).toBe("high");
  expect(event.systemPromptOptions.sections.rewake_work_mode).toBe("go-owned review");
  r.idle(true);
  r.native.setThinkingLevel = setLevel;
  const finish = Promise.withResolvers<void>();
  const selected = Promise.withResolvers<void>();
  const setModel = r.native.setModel;
  r.native.setModel = async (next) => {
    await setModel(next);
    selected.resolve();
    await finish.promise;
    return true;
  };
  const switching = r.select(plan("build", { provider: "p", model: "second", effort: "low" }));
  await selected.promise;
  expect(await r.modes.thinking(["off", "low", "high"])).toEqual({ error: "go busy" });
  finish.resolve();
  await switching;
  expect(r.values().effort).toBe("low");
});

test("process and a name of the person's own switch like any other mode", async () => {
  const r = rig();
  await r.modes.initialize(plan("process"));
  expect(r.modes.name).toBe("process");
  expect(r.status).toEqual(["mode: process"]);
  // The destination's selections apply; an unset one keeps the native value.
  expect(await r.select(plan("audit", { provider: "p", model: "second" }))).toEqual({ ok: true });
  expect(r.modes.name).toBe("audit");
  expect(r.values().effort).toBe("low");
  const event = prompt();
  await r.handlers.get("before_agent_start")?.(event);
  expect(event.systemPromptOptions.sections.rewake_work_mode).toBe("go-owned audit");
  // A built-in after a custom name: the cycle is Go's, the bridge only applies it.
  expect(await r.select(plan("process", { effort: "high" }))).toEqual({ ok: true });
  expect(r.modes.name).toBe("process");
  expect(r.values().effort).toBe("high");
});
