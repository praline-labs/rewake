import { expect, test } from "bun:test";
import { type ModePlan, Modes } from "./modes.ts";
import type { Frame } from "./protocol.ts";

type PromptEvent = { systemPromptOptions: { sections: Record<string, string> } };

function rig() {
  let idle = true;
  let model = { provider: "p", id: "first" };
  let effort = "low";
  let accepted = true;
  const status: string[] = [];
  const warnings: string[] = [];
  const handlers = new Map<string, (event: PromptEvent) => void>();
  const native = {
    on: (name: string, fn: (event: PromptEvent) => void) => handlers.set(name, fn),
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
      return { op: "answer", id: 1, ok: true, mode: destination };
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
  r.handlers.get("before_agent_start")?.(event);
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
