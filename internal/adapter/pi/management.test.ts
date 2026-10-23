// The cache figure's source in a pi message, run by go test (worktree_tool_test.go).
import { expect, test } from "bun:test";
import { cacheOf } from "./management.ts";

const usage = { input: 50, cacheRead: 900, cacheWrite: 50 };

test("a finished response's usage is the cache", () => {
  expect(cacheOf({ role: "assistant", usage })).toEqual({ read: 900, write: 50, input: 50 });
});

test("an interrupted or failed response keeps the counts it reported", () => {
  const cut = { input: 50, cacheRead: 0, cacheWrite: 950 };
  expect(cacheOf({ role: "assistant", usage: cut, stopReason: "aborted" } as any)).toEqual({
    read: 0,
    write: 950,
    input: 50,
  });
  expect(cacheOf({ role: "assistant", usage: cut, stopReason: "error" } as any)).toEqual({
    read: 0,
    write: 950,
    input: 50,
  });
});

test("a placeholder with no input counts, a missing usage and another role leave the last", () => {
  const empty = { input: 0, cacheRead: 0, cacheWrite: 0 };
  expect(cacheOf({ role: "assistant", usage: empty, stopReason: "error" } as any)).toBeNull();
  expect(cacheOf({ role: "assistant" })).toBeNull();
  expect(cacheOf({ role: "user", usage })).toBeNull();
});
