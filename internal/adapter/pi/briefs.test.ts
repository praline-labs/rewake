// The brief tools' contract with pi, run by go test (worktree_tool_test.go).
import { expect, test } from "bun:test";
import { briefTimeoutMs, briefTools } from "./briefs.ts";

const shown =
  "brief 3 · Login · edition 2 · revision 5 · 1 of 2 done · 0 comments\n\n- [x] #1 move it";

// stage registers the tools on a pi that records each exec and answers it
// with ran, and returns the tools and the calls.
function stage(ran: () => Promise<any>, executable = "/opt/rewake/bin/rewake", role = "write") {
  const tools: any[] = [];
  const execs: any[] = [];
  const pi: any = {
    registerTool: (tool: any) => tools.push(tool),
    exec: (command: string, args: string[], options: any) => {
      execs.push({ command, args, options });
      return ran();
    },
  };
  briefTools(pi, executable, role);
  return { tools, execs };
}

const named = (tools: any[], name: string) => tools.find((tool) => tool.name === name);
const call = (tool: any, params: object) =>
  tool.execute("call-1", params, undefined, undefined, { cwd: "/work/tree" } as any);
const ok = async () => ({ stdout: shown + "\n", stderr: "", code: 0, killed: false });

test("a worker is offered the three tools, main and a run without rewake none", () => {
  expect(stage(ok).tools.map((tool) => tool.name)).toEqual([
    "brief_read",
    "brief_tick",
    "brief_comment",
  ]);
  expect(stage(ok, "/opt/rewake/bin/rewake", "main").tools).toEqual([]);
  expect(stage(ok, "").tools).toEqual([]);
});

test("each tool runs its rewake brief command by argv and answers its lines", async () => {
  const seen = stage(ok);
  const read = await call(named(seen.tools, "brief_read"), { id: 3 });
  expect(read).toEqual({
    content: [{ type: "text", text: shown }],
    details: undefined,
    isError: false,
  });
  await call(named(seen.tools, "brief_tick"), { id: 3, item: 1, edition: 2 });
  await call(named(seen.tools, "brief_tick"), { id: 3, item: 1, edition: 2, undo: true });
  await call(named(seen.tools, "brief_comment"), { id: 3, text: "--force was not needed" });
  expect(seen.execs.map((exec) => exec.args)).toEqual([
    ["brief", "show", "3"],
    ["brief", "tick", "3", "1", "--edition=2"],
    ["brief", "tick", "3", "1", "--edition=2", "--undo"],
    ["brief", "comment", "3", "--", "--force was not needed"],
  ]);
  for (const exec of seen.execs) {
    expect(exec.command).toBe("/opt/rewake/bin/rewake");
    expect(exec.options).toEqual({ cwd: "/work/tree", signal: undefined, timeout: briefTimeoutMs });
  }
});

test("an input that is no positive integer or an empty text runs nothing", async () => {
  const seen = stage(ok);
  const results = [
    await call(named(seen.tools, "brief_read"), { id: 0 }),
    await call(named(seen.tools, "brief_read"), { id: "3" }),
    await call(named(seen.tools, "brief_tick"), { id: 3, item: 1.5, edition: 2 }),
    await call(named(seen.tools, "brief_tick"), { id: 3, item: 1 }),
    await call(named(seen.tools, "brief_tick"), { id: 3, item: 1, edition: 2, undo: "yes" }),
    await call(named(seen.tools, "brief_comment"), { id: 3, text: "  " }),
  ];
  for (const result of results) expect(result.isError).toBe(true);
  expect(seen.execs).toEqual([]);
});

test("a refusal and a child killed at the bound are errors", async () => {
  const results = [
    async () => ({ stdout: "", stderr: "brief 3 is at edition 3, not 2", code: 1, killed: false }),
    async () => ({ stdout: "", stderr: "", code: 0, killed: true }),
  ];
  const seen = stage(() => results.shift()!());
  const refused = await call(named(seen.tools, "brief_tick"), { id: 3, item: 1, edition: 2 });
  expect(refused.isError).toBe(true);
  expect(refused.content[0].text).toContain(
    "rewake brief tick failed (exit 1): brief 3 is at edition 3, not 2",
  );
  const killed = await call(named(seen.tools, "brief_read"), { id: 3 });
  expect(killed.isError).toBe(true);
  expect(killed.content[0].text).toContain("did not run to its end");
});
