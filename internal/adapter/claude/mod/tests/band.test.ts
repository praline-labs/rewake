import { expect, test } from "claude-code/testing";
import { contextOf, kilo } from "../hooks/band.ts";
import { start, world } from "./world.ts";

test("the context is written as the face's rule writes it", () => {
  expect(kilo(950)).toBe("950");
  expect(kilo(46_300)).toBe("46k");
  expect(kilo(1_500_000)).toBe("1.5M");
  expect(contextOf(46_000, 272_000)).toBe("46k/272k");
  expect(contextOf(undefined, 272_000)).toBe("–/272k");
  expect(contextOf(5, undefined)).toBeUndefined();
});

const HINT = {
  plugin: "rewake",
  component: "PromptHint",
  surface: "terminal",
  viewport: { columns: 80, rows: 30 },
  props: { isDraft: false, isWorking: false, hint: "? for shortcuts" },
} as const;

const env = {
  REWAKE_MOD_SOCKET: "/state/sock/api.1.mod.sock",
  REWAKE_EPOCH: "7.100.boot",
  REWAKE_SESSION: "api-claude",
};

// What the engine's own site was handed last: the stand-in records the props
// the mod's hook passed down, as the engine would draw them.
let seen: Record<string, unknown> | undefined;

function engine(on: any) {
  on("ui.render", (_: unknown, e: any) => {
    seen = e.props;
    return { type: "Text", props: {}, children: ["native"] };
  });
}

// tailed is the tail the band adds to the hint line, undefined where it adds none.
async function tailed($: any, props: Record<string, unknown> = {}) {
  seen = undefined;
  const ui = await $.ui.mount({ ...HINT, props: { ...HINT.props, ...props } });
  await ui.unmount();
  return seen?.tail;
}

function standIn(on: any, run: Record<string, unknown> = {}) {
  engine(on);
  on("classic.SessionStart", () => ({}));
  return world(on, {
    env,
    usage: () => ({ window: 272000, tokens: 46000, percent: 17 }),
    ...run,
  });
}

test("outside a rewake run the engine draws its own hint line", async ($, on) => {
  engine(on);
  world(on, { env: {} });
  await start($);
  expect(await tailed($)).toBeUndefined();
});

test("the hint line gets the name and the context, native's own text kept", async ($, on) => {
  standIn(on);
  await start($);
  expect(await tailed($)).toBe("api-claude · 46k/272k");
  expect(seen?.hint).toBe("? for shortcuts");
  expect(await tailed($, { isWorking: true, hint: "esc to interrupt" })).toBe(
    "api-claude · 46k/272k",
  );
  expect(seen?.hint).toBe("esc to interrupt");
});

test("a measure moves the context", async ($, on) => {
  let tokens = 46000;
  standIn(on, { usage: () => ({ window: 272000, tokens, percent: 17 }) });
  on("session.measure", () => ({ changed: ["context"] }));
  await start($);
  tokens = 51000;
  await $.session.measure({
    context: { window: 272000, tokens },
    rateLimits: [],
    changed: ["context"],
  } as any);
  expect(await tailed($)).toBe("api-claude · 51k/272k");
});

test("a usage the harness refuses leaves the name alone, not broken", async ($, on) => {
  standIn(on, {
    usage: () => {
      throw new Error("no usage yet");
    },
  });
  await start($);
  expect(await tailed($)).toBe("api-claude");
});

test("the context is measured against the window the launch limited the session to, not the model's", async ($, on) => {
  standIn(on, {
    env: { ...env, REWAKE_CONTEXT_WINDOW: "272000" },
    usage: () => ({ window: 1_000_000, tokens: 46000, percent: 5 }),
  });
  await start($);
  expect(await tailed($)).toBe("api-claude · 46k/272k");
});

test("a limit above the model's window, or none, leaves the model's", async ($, on) => {
  standIn(on, {
    env: { ...env, REWAKE_CONTEXT_WINDOW: "0" },
    usage: () => ({ window: 1_000_000, tokens: 46000, percent: 5 }),
  });
  await start($);
  expect(await tailed($)).toBe("api-claude · 46k/1M");
});
