// The session tools' own contract, the same in every harness: each input's
// arguments or refusal, and the result texts. Run by go test
// (sessiontools_test.go); each adapter's tests hold its bridge.
import { expect, test } from "bun:test";
import {
  briefCommentTool,
  briefReadTool,
  briefTickTool,
  briefTimeoutMs,
  type CommandTool,
  didNotEnd,
  landTimeoutMs,
  outcome,
  statusTimeoutMs,
  worktreeLandTool,
  worktreeListTool,
  worktreeStatusTool,
} from "./sessiontools.ts";

const tools = [
  worktreeStatusTool,
  worktreeListTool,
  worktreeLandTool,
  briefReadTool,
  briefTickTool,
  briefCommentTool,
];

test("each tool's schema is closed and requires only what it describes", () => {
  expect(new Set(tools.map((tool) => tool.name)).size).toBe(tools.length);
  for (const tool of tools) {
    expect(tool.inputSchema.additionalProperties).toBe(false);
    for (const name of tool.inputSchema.required ?? []) {
      expect(Object.keys(tool.inputSchema.properties)).toContain(name);
    }
  }
});

test("the worktree tools ask for the session's status, this room's checkouts and a land", () => {
  expect(worktreeStatusTool.args({})).toEqual(["worktree", "status", "--json"]);
  expect(worktreeListTool.args({})).toEqual(["worktree", "ls", "--this-room", "--json"]);
  expect(worktreeLandTool.args({ name: "fix-login", into: "release" })).toEqual([
    "worktree",
    "land",
    "fix-login",
    "--into=release",
    "--json",
  ]);
  expect(worktreeLandTool.args({ name: "rewake/fix-login" })).toEqual([
    "worktree",
    "land",
    "rewake/fix-login",
    "--json",
  ]);
});

// An empty into once landed into the default branch (2afdce1).
test("land refuses a name or a branch that would be read as a flag or left out", () => {
  for (const input of [
    {},
    { name: "" },
    { name: "--force" },
    { name: 3 },
    { name: "x", into: "-f" },
    { name: "x", into: "" },
    { name: "x", into: 4 },
  ]) {
    expect(typeof worktreeLandTool.args(input)).toBe("string");
  }
  expect(worktreeLandTool.args({ name: "x", into: "" })).toBe(
    "into must be a branch name; leave it out for the default",
  );
});

test("the brief tools run their brief command, a comment after --", () => {
  expect(briefReadTool.args({ id: 3 })).toEqual(["brief", "show", "3"]);
  expect(briefTickTool.args({ id: 3, item: 1, edition: 2 })).toEqual([
    "brief",
    "tick",
    "3",
    "1",
    "--edition=2",
  ]);
  expect(briefTickTool.args({ id: 3, item: 1, edition: 2, undo: false })).toEqual([
    "brief",
    "tick",
    "3",
    "1",
    "--edition=2",
  ]);
  expect(briefTickTool.args({ id: 3, item: 1, edition: 2, undo: true })).toEqual([
    "brief",
    "tick",
    "3",
    "1",
    "--edition=2",
    "--undo",
  ]);
  expect(briefCommentTool.args({ id: 3, text: "--force was not needed" })).toEqual([
    "brief",
    "comment",
    "3",
    "--",
    "--force was not needed",
  ]);
});

test("a brief input that is no positive integer or an empty text is refused", () => {
  const refused: [CommandTool, object][] = [
    [briefReadTool, {}],
    [briefReadTool, { id: 0 }],
    [briefReadTool, { id: "3" }],
    [briefReadTool, { id: -1 }],
    [briefTickTool, { id: 3, item: 1.5, edition: 2 }],
    [briefTickTool, { id: 3, item: 0, edition: 2 }],
    [briefTickTool, { id: 3, item: 1 }],
    [briefTickTool, { id: 3, item: 1, edition: 2, undo: "yes" }],
    [briefCommentTool, { id: 3 }],
    [briefCommentTool, { id: 3, text: "  " }],
    [briefCommentTool, { id: 3, text: 5 }],
  ];
  for (const [tool, input] of refused) {
    expect(typeof tool.args(input as Record<string, unknown>)).toBe("string");
  }
});

test("each tool is bounded, land past its own minute's wait for a hook", () => {
  expect(worktreeStatusTool.timeoutMs).toBe(statusTimeoutMs);
  expect(worktreeListTool.timeoutMs).toBe(statusTimeoutMs);
  expect(worktreeLandTool.timeoutMs).toBe(landTimeoutMs);
  expect(landTimeoutMs).toBeGreaterThan(60_000);
  for (const tool of [briefReadTool, briefTickTool, briefCommentTool]) {
    expect(tool.timeoutMs).toBe(briefTimeoutMs);
  }
});

test("a command that ran answers its output, or its failure with what it said", () => {
  expect(outcome("rewake worktree status", 0, ' {"a":1}\n', "")).toEqual({
    isError: false,
    text: '{"a":1}',
  });
  expect(outcome("rewake brief tick", 1, "out", " edition 3, not 2 \n")).toEqual({
    isError: true,
    text: "rewake brief tick failed (exit 1): edition 3, not 2",
  });
  expect(outcome("rewake worktree land", 2, " only out ", "").text).toBe(
    "rewake worktree land failed (exit 2): only out",
  );
  expect(outcome("rewake worktree ls", null, "", "").text).toBe(
    "rewake worktree ls failed (exit null): no output",
  );
});

test("a command that did not run to its end is an error saying why", () => {
  expect(didNotEnd("rewake brief show", new Error("spawn ENOENT"))).toEqual({
    isError: true,
    text: "rewake brief show did not run to its end: spawn ENOENT",
  });
  expect(didNotEnd("rewake brief show", "stopped after 60000 ms").text).toBe(
    "rewake brief show did not run to its end: stopped after 60000 ms",
  );
});
