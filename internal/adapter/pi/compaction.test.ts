// The turn the face's mirror says while pi compacts, run by go test
// (worktree_tool_test.go). pi says every compaction alike, its own at the
// threshold, the person's /compact and the one main asks for: one
// session_before_compact, then session_compact or session_compact_failed.
import { expect, test } from "bun:test";
import type { MirrorEvent } from "./feed.ts";
import { Mirror } from "./mirror.ts";
import { serveMirror } from "./mirrorhooks.ts";

function mirrored() {
  const events: MirrorEvent[] = [];
  const mirror = new Mirror({ push: (e) => events.push(e) });
  mirror.on = true;
  const handlers = new Map<string, ((e: unknown, ctx: unknown) => unknown)[]>();
  const pi = {
    on: (name: string, handle: (e: unknown, ctx: unknown) => unknown) =>
      handlers.set(name, [...(handlers.get(name) ?? []), handle]),
  };
  serveMirror(pi as never, mirror);
  const emit = (name: string, e: unknown = {}) =>
    (handlers.get(name) ?? []).map((handle) => handle(e, { isIdle: () => true }));
  const turns = () => events.filter((e) => e.kind === "turn").map((e) => [e.turn, e.state]);
  return { emit, turns };
}

test("a compaction inside a run is said as the run's, which goes on working after it", () => {
  const { emit, turns } = mirrored();
  emit("agent_start");
  // Neither cancels pi's compaction nor gives it a summary of its own.
  expect(emit("session_before_compact", { reason: "threshold" })).toEqual([undefined]);
  emit("session_compact", { reason: "threshold" });
  emit("agent_settled", { aborted: false });
  const run = turns()[0][0];
  expect(turns()).toEqual([
    [run, "working"],
    [run, "compacting"],
    [run, "working"],
    [run, "idle"],
  ]);
});

test("a compaction outside a run, done, failed or aborted, leaves pi idle", () => {
  for (const end of ["session_compact", "session_compact_failed"]) {
    const { emit, turns } = mirrored();
    // pi's /compact while it works stops the run first.
    emit("agent_start");
    emit("agent_settled", { aborted: true });
    emit("session_before_compact", { reason: "manual" });
    emit(end, { reason: "manual", aborted: true });
    const [, , [own, state], ...rest] = turns();
    expect([state, rest]).toEqual(["compacting", [[own, "idle"]]]);
    expect(own).not.toBe(turns()[0][0]);
  }
});

test("a compaction pi refused before beginning, with nothing to compact, says nothing", () => {
  const { emit, turns } = mirrored();
  emit("session_compact_failed", { reason: "manual", errorMessage: "Nothing to compact" });
  expect(turns()).toEqual([]);
});
