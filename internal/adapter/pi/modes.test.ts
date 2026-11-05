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
    destination: (plan: ModePlan) => {
      destination = plan;
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
  expect(await r.modes.apply(plan("review"))).toEqual({ ok: true });
  expect(r.values()).toEqual({ model: { provider: "p", id: "first" }, effort: "low", changed: 1 });
  r.destination(plan("advisor", { provider: "p", model: "second" }));
  expect(await r.modes.cycle()).toEqual({ ok: true });
  expect(r.asks[0].op).toBe("mode-cycle");
  expect(r.values()).toEqual({ model: { provider: "p", id: "second" }, effort: "low", changed: 2 });
  expect(r.modes.current?.name).toBe("advisor");
  expect(r.status).toEqual(["mode: review", "mode: advisor"]);
  const event = { systemPromptOptions: { sections: { existing: "kept" } } };
  await r.handlers.get("before_agent_start")?.(event);
  expect(event.systemPromptOptions.sections).toEqual({
    existing: "kept",
    rewake_work_mode: "go-owned advisor",
  });
  await r.modes.apply(plan("build", { effort: "high" }));
  expect(r.values().effort).toBe("high");
});

test("busy and failed registry/model selection leave all three live values unchanged", async () => {
  const r = rig();
  await r.modes.apply(plan("review"));
  const before = r.values();
  r.idle(false);
  expect(await r.modes.apply(plan("build", { effort: "high" }))).toEqual({ error: "go busy" });
  r.idle(true);
  expect(
    await r.modes.apply(plan("build", { provider: "p", model: "missing", effort: "high" })),
  ).toEqual({ error: "go failed" });
  r.accepts(false);
  r.destination(plan("build", { provider: "p", model: "second", effort: "high" }));
  expect(await r.modes.cycle()).toEqual({ error: "go failed" });
  expect(r.warnings).toEqual(["go failed"]);
  expect(r.values()).toEqual(before);
  expect(r.modes.current?.name).toBe("review");
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
  expect(await r.modes.apply(plan("review"), true)).toEqual({ ok: true });
  await starting;
  expect(event.systemPromptOptions.sections.rewake_work_mode).toBe("go-owned review");
});

test("native starts wait through asynchronous model selection and see the committed stance", async () => {
  const r = rig();
  await r.modes.apply(plan("review"));
  const selected = Promise.withResolvers<void>();
  const finish = Promise.withResolvers<void>();
  const setModel = r.native.setModel;
  r.native.setModel = async (next) => {
    await setModel(next);
    selected.resolve();
    await finish.promise;
    return true;
  };
  const switching = r.modes.apply(
    plan("build", { provider: "p", model: "second", effort: "high" }),
  );
  await selected.promise;
  const event = prompt();
  let started = false;
  const starting = Promise.resolve(r.handlers.get("before_agent_start")?.(event)).then(() => {
    started = true;
  });
  try {
    await Promise.resolve();
    expect(started).toBe(false);
    expect(r.modes.current?.name).toBe("review");
  } finally {
    finish.resolve();
    await switching;
    await starting;
  }
  expect(event.systemPromptOptions.sections.rewake_work_mode).toBe("go-owned build");
  expect(r.values().effort).toBe("high");
});

test("a refused selection restores native state even if the native model already changed", async () => {
  const r = rig();
  await r.modes.apply(plan("review"));
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
  const switching = r.modes.apply(
    plan("build", { provider: "p", model: "second", effort: "high" }),
  );
  await restoring.promise;
  let admitted = false;
  const waiting = r.modes.admit(() => {
    admitted = true;
  });
  await Promise.resolve();
  expect(admitted).toBe(false);
  finish.resolve();
  expect(await switching).toEqual({ error: "go failed" });
  expect(await waiting).toBe(true);
  expect(r.values().model).toEqual(before.model);
  expect(r.values().effort).toBe(before.effort);
  expect(r.modes.current?.name).toBe("review");
});

test("busy initialization stops rather than dropping its plan and opening an uninitialized run", async () => {
  const r = rig();
  r.idle(false);
  const starting = r.handlers.get("before_agent_start")?.(prompt());
  const input = r.handlers.get("input")?.(prompt());
  expect(await r.modes.apply(plan("review"), true)).toEqual({ error: "go busy" });
  await starting;
  expect(await input).toEqual({ action: "handled" });
  expect(r.stopped()).toEqual({ shutdowns: 1, aborts: 1 });
});

test("native and face thinking close admission before mutation and cannot overtake a mode selection", async () => {
  const r = rig();
  await r.modes.apply(plan("review"));
  const setLevel = r.native.setThinkingLevel;
  const admitted: string[] = [];
  r.native.setThinkingLevel = (level) => {
    expect(r.modes.blocked).toBe(true);
    void r.modes.admit(() => {
      admitted.push(r.values().effort);
    });
    setLevel(level);
    expect(admitted).toEqual([]);
  };
  expect(await r.modes.thinking()).toEqual({ ok: true });
  expect(admitted).toEqual(["high"]);
  r.native.setThinkingLevel = setLevel;
  const finish = Promise.withResolvers<void>();
  const setModel = r.native.setModel;
  r.native.setModel = async (next) => {
    await setModel(next);
    await finish.promise;
    return true;
  };
  const switching = r.modes.apply(plan("build", { provider: "p", model: "second", effort: "low" }));
  expect(await r.modes.thinking(["off", "low", "high"])).toEqual({ error: "go busy" });
  finish.resolve();
  await switching;
  expect(r.values().effort).toBe("low");
});
