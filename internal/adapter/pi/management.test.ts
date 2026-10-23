// The cache figure's source in a pi message, run by go test (worktree_tool_test.go).
import { expect, test } from "bun:test";
import { cacheOf, lifeOf } from "./management.ts";

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

test("the cache lives the tier's lifetime from the request, the long tier by PI_CACHE_RETENTION", () => {
  const model = { provider: "p", id: "m", promptCache: { short: 300, long: 3600 } };
  const message = { provider: "p", model: "m", timestamp: 1_000_000 };
  expect(lifeOf(message, model, undefined)).toEqual({ ttl: 300, expiresAt: 1_300_000 });
  expect(lifeOf(message, model, "long")).toEqual({ ttl: 3600, expiresAt: 4_600_000 });
  expect(lifeOf(message, { ...model, promptCache: { short: 300 } }, "long")).toEqual({});
  expect(lifeOf(message, { provider: "p", id: "m" }, undefined)).toEqual({});
  expect(lifeOf(message, { ...model, id: "other" }, undefined)).toEqual({});
});
