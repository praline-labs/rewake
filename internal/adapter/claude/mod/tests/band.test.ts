import { expect, test } from "claude-code/testing";
import { contextOf, kilo, rowOf } from "../hooks/band.ts";
import { start, world } from "./world.ts";

const auto = { label: "⏵⏵ auto mode", color: "warning" };
const plan = { label: "⏸ plan mode", color: "planMode" };
const text = (row: ReturnType<typeof rowOf>) =>
  row ? (row.mode ? row.mode.label : "") + " ".repeat(row.gap) + row.right : undefined;

test("the context is written as the face's rule writes it", () => {
  expect(kilo(950)).toBe("950");
  expect(kilo(46_300)).toBe("46k");
  expect(kilo(1_500_000)).toBe("1.5M");
  expect(contextOf(46_000, 272_000)).toBe("46k/272k");
  expect(contextOf(undefined, 272_000)).toBe("–/272k");
  expect(contextOf(5, undefined)).toBeUndefined();
});

test("the row has the mode at its left and the name with the context at its right", () => {
  const row = rowOf(50, "w-claude", "46k/272k", auto);
  expect(text(row)).toBe("⏵⏵ auto mode" + " ".repeat(50 - 12 - 19) + "w-claude · 46k/272k");
  expect(row?.mode).toBe(auto);
  expect(text(rowOf(50, "w-claude", "46k/272k", undefined))).toBe(
    " ".repeat(31) + "w-claude · 46k/272k",
  );
});

test("a narrow row drops the mode first, then the context, then cuts the name", () => {
  const full = "⏵⏵ auto mode w-claude · 46k/272k".length;
  expect(text(rowOf(full, "w-claude", "46k/272k", auto))).toBe("⏵⏵ auto mode w-claude · 46k/272k");
  expect(text(rowOf(full - 1, "w-claude", "46k/272k", auto))).toBe(
    " ".repeat(12) + "w-claude · 46k/272k",
  );
  expect(text(rowOf(18, "w-claude", "46k/272k", auto))).toBe(" ".repeat(10) + "w-claude");
  expect(text(rowOf(6, "w-claude", "46k/272k", plan))).toBe("w-cla…");
  expect(rowOf(1, "w-claude", "46k/272k", plan)).toBeUndefined();
});

test("a mode alone, with no name, has the row to itself", () => {
  expect(text(rowOf(30, undefined, undefined, plan))).toBe("⏸ plan mode" + " ".repeat(19));
  expect(rowOf(30, undefined, undefined, undefined)).toBeUndefined();
});

const PROMPT = {
  plugin: "rewake",
  component: "AbovePrompt",
  surface: "terminal",
  viewport: { columns: 80, rows: 30 },
  props: { bodyColumns: 60, hasSurvey: false, isWorking: false, maxRows: 5, view: {} },
} as const;

const env = {
  REWAKE_MOD_SOCKET: "/state/sock/api.1.mod.sock",
  REWAKE_EPOCH: "7.100.boot",
  REWAKE_SESSION: "api-claude",
};

// drawn is the band's text as the person would see it, undefined where the
// engine's own site was drawn.
async function drawn($: any, props: Record<string, unknown> = {}) {
  const ui = await $.ui.mount({ ...PROMPT, props: { ...PROMPT.props, ...props } });
  const box = (await ui.find({ type: "Box" })) as any;
  await ui.unmount();
  return box ? box.children.map((c: any) => c.children.join("")).join("") : undefined;
}

function standIn(on: any, run: Record<string, unknown> = {}) {
  on("ui.render", () => ({ type: "Text", props: {}, children: ["native"] }));
  on("classic.SessionStart", () => ({}));
  on("classic.UserPromptSubmit", () => ({}));
  return world(on, {
    env,
    usage: () => ({ window: 272000, tokens: 46000, percent: 17 }),
    ...run,
  });
}

test("outside a rewake run the engine draws its own band", async ($, on) => {
  on("ui.render", () => ({ type: "Text", props: {}, children: ["native"] }));
  world(on, { env: {} });
  await start($);
  expect(await drawn($)).toBeUndefined();
});

test("the band says the name and the context, and the mode once an event reports it", async ($, on) => {
  standIn(on);
  await start($);
  expect(await drawn($)).toBe(" ".repeat(39) + "api-claude · 46k/272k");
  await $.classic.UserPromptSubmit({ prompt: "go", permission_mode: "auto" } as any);
  expect(await drawn($)).toBe("⏵⏵ auto mode" + " ".repeat(27) + "api-claude · 46k/272k");
  await $.classic.UserPromptSubmit({ prompt: "go", permission_mode: "plan" } as any);
  expect(await drawn($)).toBe("⏸ plan mode" + " ".repeat(28) + "api-claude · 46k/272k");
  await $.classic.UserPromptSubmit({ prompt: "go", permission_mode: "default" } as any);
  expect(await drawn($)).toBe(" ".repeat(39) + "api-claude · 46k/272k");
});

test("a narrow band cuts as the rule does", async ($, on) => {
  standIn(on);
  await start($);
  await $.classic.UserPromptSubmit({ prompt: "go", permission_mode: "acceptEdits" } as any);
  expect(await drawn($, { bodyColumns: 60 })).toBe(
    "⏵⏵ accept edits" + " ".repeat(24) + "api-claude · 46k/272k",
  );
  expect(await drawn($, { bodyColumns: 30 })).toBe(" ".repeat(9) + "api-claude · 46k/272k");
  expect(await drawn($, { bodyColumns: 12 })).toBe("  api-claude");
});

test("a survey keeps the band, and a measure moves the context", async ($, on) => {
  let tokens = 46000;
  standIn(on, { usage: () => ({ window: 272000, tokens, percent: 17 }) });
  on("session.measure", () => ({ changed: ["context"] }));
  await start($);
  expect(await drawn($, { hasSurvey: true })).toBeUndefined();
  tokens = 51000;
  await $.session.measure({
    context: { window: 272000, tokens },
    rateLimits: [],
    changed: ["context"],
  } as any);
  expect(await drawn($)).toBe(" ".repeat(39) + "api-claude · 51k/272k");
});

test("a usage the harness refuses leaves the band without a context, not broken", async ($, on) => {
  standIn(on, {
    usage: () => {
      throw new Error("no usage yet");
    },
  });
  await start($);
  expect(await drawn($)).toBe(" ".repeat(50) + "api-claude");
});
