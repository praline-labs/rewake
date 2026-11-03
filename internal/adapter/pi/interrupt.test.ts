// The person's Esc and shift+tab in the face, run by go test
// (worktree_tool_test.go). What pi's screen shows of the run Esc stops is
// the wrapper's translation (translate_test.go).
import { expect, test } from "bun:test";
import { faceInterrupt, facePrompt, faceThinking } from "./faced.ts";

test("Esc stops a working pi through its own abort, and leaves an idle one alone", () => {
  let aborted = 0;
  const pi = (idle: boolean) => ({ isIdle: () => idle, abort: () => aborted++ });
  expect(faceInterrupt(true, pi(false), false)).toEqual({ ok: true });
  expect(aborted).toBe(1);
  expect(faceInterrupt(true, pi(true), false).error).toContain("not working");
  expect(faceInterrupt(true, pi(false), true).error).toContain("compacting");
  const throwing = {
    isIdle: () => false,
    abort: () => {
      throw new Error("x");
    },
  };
  expect(faceInterrupt(true, throwing, false).error).toBeDefined();
  expect(faceInterrupt(false, pi(false), false).error).toContain("does not draw");
  expect(aborted).toBe(1);
});

// pi's shift+tab moves the level to the next of those the wrapper read off
// its model and wraps round to off (core/agent-session.js
// cycleThinkingLevel, read on 1.4.0; native.go, levelsOf).
test("shift+tab moves pi's thinking level on as pi's own does", () => {
  const levels = ["off", "low", "medium", "high", "xhigh"];
  let level = "high";
  const pi = {
    getThinkingLevel: () => level as never,
    setThinkingLevel: (to: string) => {
      level = to;
    },
  };
  const cycled = (on = true) => faceThinking(pi, on, levels);
  expect(cycled()).toEqual({ ok: true });
  expect(level).toBe("xhigh");
  cycled();
  expect(level).toBe("off");
  cycled();
  expect(level).toBe("low");
  // A level the model does not take moves to its first, as in pi.
  level = "minimal";
  cycled();
  expect(level).toBe("off");
  expect(faceThinking(pi, true, undefined).error).toBeDefined();
  expect(cycled(false).error).toContain("does not draw");
  expect(level).toBe("off");
});

test("the face's prompt is the person's to pi, steered while pi works", () => {
  const sent: { text: string; options: unknown }[] = [];
  const pi = {
    sendUserMessage: (text: string, options: unknown) => sent.push({ text, options }),
  } as never;
  expect(facePrompt(pi, true, true, false, "go")).toEqual({ ok: true, steered: false });
  expect(facePrompt(pi, true, false, false, "more")).toEqual({ ok: true, steered: true });
  expect(sent).toEqual([
    { text: "go", options: undefined },
    { text: "more", options: { deliverAs: "steer" } },
  ]);
  expect(facePrompt(pi, true, true, true, "x").error).toContain("compacting");
  expect(facePrompt(pi, true, true, false, " ").error).toBeDefined();
  expect(facePrompt(pi, false, true, false, "x").error).toContain("does not draw");
});
