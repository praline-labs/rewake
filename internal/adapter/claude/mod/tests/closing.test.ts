import { expect, test } from "claude-code/testing";
import { closingWait } from "../hooks/closing.ts";
import { start, world } from "./world.ts";

const closings = (run: ReturnType<typeof world>) =>
  run.sent.filter((one) => one.path === "/v1/session/closing").map((one) => one.body.reason);

test("/exit tells nothing before its command runs, and the exit's session.end tells the wrapper", async ($, on) => {
  const run = world(on);
  on("command.run", () => ({ text: "" }));
  on("session.end", (_: any, e: any) => ({ sessionId: e.sessionId }));
  await start($);
  await run.clock.settle();
  await $.command.run({ command: "exit", args: "" } as any);
  expect(closings(run)).toEqual([]);
  await $.session.end({ reason: "prompt_input_exit", sessionId: "conversation-1" } as any);
  expect(closings(run)).toEqual(["prompt_input_exit"]);
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

test("/logout's end tells the wrapper like an exit", async ($, on) => {
  const run = world(on);
  on("session.end", (_: any, e: any) => ({ sessionId: e.sessionId }));
  await start($);
  await run.clock.settle();
  await $.session.end({ reason: "logout", sessionId: "conversation-1" } as any);
  expect(closings(run)).toEqual(["logout"]);
});

test("a wrapper that does not answer holds the end no longer than the bound", async ($, on) => {
  const run = world(on, { replies: { "/v1/session/closing": [() => new Promise(() => {})] } });
  let ended = false;
  on("session.end", (_: any, e: any) => {
    ended = true;
    return { sessionId: e.sessionId };
  });
  await start($);
  await run.clock.settle();
  const end = $.session.end({ reason: "other", sessionId: "conversation-1" } as any);
  await run.clock.advance(closingWait - 1);
  expect(ended).toBe(false);
  await run.clock.advance(1);
  await end;
  expect(ended).toBe(true);
});
