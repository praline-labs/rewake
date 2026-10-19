import { expect, test } from "claude-code/testing";
import { closingWait } from "../hooks/closing.ts";
import { start, world } from "./world.ts";

const closings = (run: ReturnType<typeof world>) =>
  run.sent.filter((one) => one.path === "/v1/session/closing").map((one) => one.body.reason);

test("/exit tells the wrapper before the command runs, and an exit's session.end tells it again", async ($, on) => {
  const run = world(on);
  const order: string[] = [];
  on("command.run", () => {
    order.push("exit ran after " + closings(run).length);
    return { text: "" };
  });
  on("session.end", (_: any, e: any) => ({ sessionId: e.sessionId }));
  await start($);
  await run.clock.settle();
  await $.command.run({ command: "exit", args: "" } as any);
  expect(order).toEqual(["exit ran after 1"]);
  await $.session.end({ reason: "prompt_input_exit", sessionId: "conversation-1" } as any);
  expect(closings(run)).toEqual(["prompt_input_exit", "prompt_input_exit"]);
});

test("a conversation's end that keeps the process tells nothing", async ($, on) => {
  const run = world(on);
  on("session.end", (_: any, e: any) => ({ sessionId: e.sessionId }));
  await start($);
  await run.clock.settle();
  await $.session.end({ reason: "clear", sessionId: "conversation-1" } as any);
  await $.session.end({ reason: "resume", sessionId: "conversation-1" } as any);
  expect(closings(run)).toEqual([]);
});

test("a wrapper that does not answer holds the exit no longer than the bound", async ($, on) => {
  const run = world(on, { replies: { "/v1/session/closing": [() => new Promise(() => {})] } });
  let ran = false;
  on("command.run", () => {
    ran = true;
    return { text: "" };
  });
  await start($);
  await run.clock.settle();
  const exit = $.command.run({ command: "exit", args: "" } as any);
  await run.clock.advance(closingWait - 1);
  expect(ran).toBe(false);
  await run.clock.advance(1);
  await exit;
  expect(ran).toBe(true);
});
