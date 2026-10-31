import { expect, test } from "claude-code/testing";
import { holdMs, start, world } from "./world.ts";

const HINT = {
  plugin: "rewake",
  component: "PromptHint",
  surface: "terminal",
  viewport: { columns: 80, rows: 30 },
  props: { isDraft: false, isWorking: false, hint: "? for shortcuts" },
} as const;

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

// standIn is a run whose heartbeat answers each poll at once with each
// answer in turn, and then holds its polls as the endpoint holds them.
function standIn(on: any, ...answers: Record<string, unknown>[]) {
  engine(on);
  on("classic.SessionStart", () => ({}));
  let run: ReturnType<typeof world> | undefined;
  const held = async () => {
    await run?.clock.sleep(holdMs);
    return { status: 200, text: "{}" };
  };
  const replies = answers.map((answer) => ({ status: 200, text: JSON.stringify(answer) }));
  run = world(on, { replies: { "/v1/mod/alive": [...replies, held] } });
  return run;
}

test("outside a rewake run the engine draws its own hint line", async ($, on) => {
  engine(on);
  world(on, { env: {} });
  await start($);
  expect(await tailed($)).toBeUndefined();
});

test("the hint line gets the run's work as the heartbeat tells it, native's own text kept", async ($, on) => {
  const run = standIn(on, { work: "brief 57 3/6" });
  await start($);
  await run.clock.settle();
  expect(await tailed($)).toBe("brief 57 3/6");
  expect(seen?.hint).toBe("? for shortcuts");
  expect(await tailed($, { isWorking: true, hint: "esc to interrupt" })).toBe("brief 57 3/6");
  expect(seen?.hint).toBe("esc to interrupt");
  // Each poll names what the band shows, so the host answers a change at once.
  expect(run.polls().map((poll) => poll.body.work)).toEqual(["", "brief 57 3/6"]);
});

test("an answer without work leaves the last, and work that is over leaves no tail", async ($, on) => {
  const run = standIn(on, { work: "brief 57 3/6" }, {}, { work: "" });
  await start($);
  await run.clock.settle();
  expect(run.polls().map((poll) => poll.body.work)).toEqual([
    "",
    "brief 57 3/6",
    "brief 57 3/6",
    "",
  ]);
  expect(await tailed($)).toBeUndefined();
});
