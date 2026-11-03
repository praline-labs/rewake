// The tools' bridge to pi, run by go test (worktree_tool_test.go): the
// table and the wording are the wrapper's (internal/core/sessiontools), so
// what is held here is that the extension registers what the table lists,
// runs what the plan names and sends the raw end.
import { expect, test } from "bun:test";
import { fit, maxOutput, type ToolEntry, Tools, unanswered } from "./tools.ts";

const schema = { type: "object", properties: {}, additionalProperties: false };
const status: ToolEntry = {
  name: "worktree_status",
  label: "Worktree status",
  description: "Say which worktree.",
  inputSchema: schema,
};
const tick: ToolEntry = {
  name: "brief_tick",
  label: "Tick",
  description: "Tick an item.",
  inputSchema: schema,
};
const plan = { argv: ["/r/rewake", "brief", "tick", "3", "1", "--edition=2"], timeoutMs: 30_000 };

// stage is a pi that records what is registered and run, and a wrapper
// that answers each request with the next of its replies.
function stage(
  replies: Record<string, any[]>,
  ran: () => Promise<any> = async () => ({ stdout: "done\n", stderr: "", code: 0, killed: false }),
) {
  const registered: any[] = [];
  const execs: any[] = [];
  const asked: any[] = [];
  const pi: any = {
    registerTool: (tool: any) => registered.push(tool),
    exec: (command: string, args: string[], options: any) => {
      execs.push({ command, args, options });
      return ran();
    },
  };
  const tools = new Tools(pi, async (fields) => {
    asked.push(fields);
    const next = replies[fields.op]?.shift();
    if (next instanceof Error) throw next;
    return typeof next === "function" ? next() : next;
  });
  const call = (name: string, params: object, signal?: AbortSignal) =>
    registered
      .findLast((tool) => tool.name === name)
      .execute("c", params, signal, undefined, { cwd: "/work/tree" });
  return { tools, registered, execs, asked, call };
}

test("a table is registered as it lists, a changed tool registered again, a dropped one hidden and a returning one shown", () => {
  const { tools, registered } = stage({});
  tools.take([status, tick]);
  tools.take([status, tick]);
  expect(registered.map((tool) => [tool.name, tool.description, tool.exposure])).toEqual([
    ["worktree_status", "Say which worktree.", undefined],
    ["brief_tick", "Tick an item.", undefined],
  ]);
  tools.take([{ ...status, description: "Say which worktree, now." }]);
  tools.take([{ ...status, description: "Say which worktree, now." }]);
  tools.take([{ ...status, description: "Say which worktree, now." }, tick]);
  expect(registered.slice(2).map((tool) => [tool.name, tool.description, tool.exposure])).toEqual([
    ["worktree_status", "Say which worktree, now.", undefined],
    ["brief_tick", "Tick an item.", "hidden"],
    ["brief_tick", "Tick an item.", undefined],
  ]);
  expect(registered[0].parameters).toEqual(schema);
  expect(registered[0].label).toBe("Worktree status");
});

test("a call is planned by the wrapper, run by argv in the session's directory with its abort and bound, and its raw end worded by the wrapper", async () => {
  const seen = stage({ "tool-plan": [plan], "tool-result": [{ text: "worded", isError: false }] });
  seen.tools.take([tick]);
  const signal = new AbortController().signal;
  const result = await seen.call("brief_tick", { id: 3, item: 1, edition: 2 }, signal);
  expect(seen.asked[0]).toEqual({
    op: "tool-plan",
    tool: "brief_tick",
    arguments: { id: 3, item: 1, edition: 2 },
  });
  expect(seen.execs).toEqual([
    {
      command: "/r/rewake",
      args: ["brief", "tick", "3", "1", "--edition=2"],
      options: { cwd: "/work/tree", signal, timeout: 30_000 },
    },
  ]);
  expect(seen.asked[1]).toEqual({
    op: "tool-result",
    tool: "brief_tick",
    ran: { exitCode: 0, stdout: "done\n", stderr: "" },
  });
  expect(result).toEqual({
    content: [{ type: "text", text: "worded" }],
    details: undefined,
    isError: false,
  });
});

test("a refused plan runs nothing and answers the wrapper's refusal as an error", async () => {
  const seen = stage({ "tool-plan": [{ refused: "brief_tick needs id" }] });
  seen.tools.take([tick]);
  const result = await seen.call("brief_tick", {});
  expect(result.isError).toBe(true);
  expect(result.content[0].text).toBe("brief_tick needs id");
  expect(seen.execs).toEqual([]);
});

test("a call aborted while it was planned runs nothing, so a cancelled tick writes nothing, and is sent as killed", async () => {
  const abort = new AbortController();
  const seen = stage({
    "tool-plan": [
      () => {
        abort.abort();
        return plan;
      },
    ],
    "tool-result": [{ text: "stopped", isError: true }],
  });
  seen.tools.take([tick]);
  const result = await seen.call("brief_tick", { id: 3, item: 1, edition: 2 }, abort.signal);
  expect(seen.execs).toEqual([]);
  expect(seen.asked[1].ran).toEqual({ stdout: "", stderr: "", killed: true });
  expect(result.isError).toBe(true);
});

test("a child pi killed and an exec that threw are sent raw for the wrapper to word", async () => {
  const ends = [
    async () => ({ stdout: "", stderr: "", code: 0, killed: true }),
    async () => {
      throw new Error("spawn ENOENT");
    },
  ];
  const seen = stage(
    {
      "tool-plan": [plan, plan],
      "tool-result": [
        { text: "a", isError: true },
        { text: "b", isError: true },
      ],
    },
    () => ends.shift()!(),
  );
  seen.tools.take([tick]);
  await seen.call("brief_tick", {});
  await seen.call("brief_tick", {});
  expect(seen.asked[1].ran).toEqual({ exitCode: 0, stdout: "", stderr: "", killed: true });
  expect(seen.asked[3].ran).toEqual({ stdout: "", stderr: "", error: "spawn ENOENT" });
});

test("a wrapper that does not answer, or refuses the request, gives the one fixed text", async () => {
  const seen = stage({
    "tool-plan": [new Error("no answer to tool-plan"), plan],
    "tool-result": [{ error: "unknown extension operation" }],
  });
  seen.tools.take([tick]);
  const lost = await seen.call("brief_tick", {});
  expect(lost.isError).toBe(true);
  expect(lost.content[0].text).toBe(unanswered("no answer to tool-plan"));
  const refused = await seen.call("brief_tick", {});
  expect(refused.content[0].text).toBe(unanswered("unknown extension operation"));
});

test("an output past the bound is cut at a whole character and marked cut", () => {
  const long = "é".repeat(maxOutput);
  const kept = fit({ exitCode: 0, stdout: long, stderr: "x".repeat(10_000) });
  expect(Buffer.byteLength(kept.stdout)).toBeLessThanOrEqual(maxOutput);
  expect(kept.stdout.endsWith("é")).toBe(true);
  expect(kept.stdoutCut).toBe(true);
  expect(kept.stderr.length).toBe(4096);
  const small = { exitCode: 0, stdout: "ok", stderr: "" };
  expect(fit(small)).toBe(small);
});
