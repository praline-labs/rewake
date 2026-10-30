import { expect, test } from "bun:test";
import type { FaceLine } from "./messages.ts";
import type { Item } from "./mirror.gen.ts";
import { FaceModel } from "./model.ts";
import { fill, linesOf, render } from "./render.ts";
import { turnLine } from "./turn.ts";
import { wrap } from "./wrap.ts";

const text = (lines: FaceLine[]) => lines.map((l) => l.spans.map((s) => s.text).join(""));

const model = (...items: Item[]) => {
  const m = new FaceModel();
  m.apply({ op: "begin", items: items.length, turn: "idle" });
  for (const item of items) m.apply({ op: "item", item });
  m.apply({ op: "end" });
  return m;
};

// The face draws the model's tail: a streamed block as it grows, a lost one
// marked, and scrolling back shows what is above.
test("the face draws the tail of the model, marks a gap, and scrolls back", () => {
  const m = new FaceModel();
  for (const line of [
    { op: "hello", v: 1 },
    { op: "begin", items: 1, turn: "idle" },
    { op: "item", item: { id: "p", kind: "prompt", status: "settled", text: "count" } },
    { op: "end" },
    { op: "add", item: { id: "b", kind: "text", status: "streaming", text: "1\n2" } },
    { op: "append", id: "b", text: "\n3" },
    { op: "set", item: { id: "b", kind: "text", status: "gap", text: "1\n2\n3" } },
  ] as const)
    m.apply(line as never);
  expect(text(render(m, 40, 100))).toEqual([
    "❯ count",
    "",
    "● 1",
    "  2",
    "  3",
    "  [lost on the way — the native face has it]",
  ]);
  expect(text(render(m, 40, 2))).toEqual(["  3", "  [lost on the way — the native face has it]"]);
  expect(text(render(m, 40, 2, 3))).toEqual(["", "● 1"]);
});

// Seen live on 2.11.0: a reply broke inside a word ("fil" / "es").
test("a text breaks at spaces and cuts only a word wider than the row", () => {
  expect(wrap("how many files are there", 12)).toEqual(["how many", "files are", "there"]);
  expect(wrap("abcdefghij kl", 4)).toEqual(["abcd", "efgh", "ij", "kl"]);
  expect(wrap("ab界cd", 3)).toEqual(["ab", "界c", "d"]);
  const reply: Item = { id: "t", kind: "text", status: "settled", text: "a **bold** word `code`" };
  const [row] = linesOf(reply, 12);
  expect(row?.spans.find((s) => s.text === "bold")?.bold).toBe(true);
  expect(text(linesOf(reply, 12))).toEqual(["● a bold", "  word code"]);
});

// Seen live on 2.11.0: a letter showed as the notification's XML.
test("a letter rewake delivered shows its summary, not the notification", () => {
  const letter: Item = {
    id: "l",
    kind: "prompt",
    status: "settled",
    origin: "peer",
    text: "<task-notification>\n<task-id>rewake-1</task-id>\n<status>completed</status>\n<summary>Rewake: main task, 1 new message\n  ↳ Do it (a preview only: run rewake inbox)</summary>\n</task-notification>",
  };
  const lines = linesOf(letter, 80);
  expect(text(lines)).toEqual([
    "● Rewake: main task, 1 new message ↳ Do it (a preview only: run rewake inbox)",
  ]);
  expect(lines[0]?.band).toBeUndefined();
});

const call = (name: string, input: object | string, output?: string, status = "done"): Item => ({
  id: name,
  kind: "tool",
  status: status as Item["status"],
  name,
  text: typeof input === "string" ? input : JSON.stringify(input),
  output,
});

// Seen live on 2.11.0: a call showed its input as JSON; brief 40 took the
// terminal's names and arguments.
test("a tool's head is the terminal's name and argument, its output folded", () => {
  const mcp = call("mcp__rewake__probe", { n: 1 }, "a\nb\nc\nd\ne");
  expect(text(linesOf(mcp, 40))).toEqual([
    "● rewake - probe (MCP)(n: 1)",
    "  ⎿  a",
    "     b",
    "     c",
    "     … +2 lines",
  ]);
  expect(linesOf(mcp, 40)[0]?.spans[1]).toEqual({ text: "rewake - probe (MCP)", bold: true });
  // Only one more line shows rather than a fold of one.
  expect(text(linesOf(call("Probe", {}, "a\nb\nc\nd"), 40))).toHaveLength(5);
  expect(text(linesOf(call("Grep", { pattern: "x", path: "/a" }, "", "running"), 40))[0]).toBe(
    '● Search(pattern: "x", path: "/a")',
  );
  // An input cut to its bound no longer reads as JSON and shows as it is.
  expect(text(linesOf(call("Bash", '{"command":"l'), 40))[0]).toBe('● Bash({"command":"l)');
  const failed = call("Probe", {}, "<tool_use_error>no such thing</tool_use_error>", "error");
  expect(text(linesOf(failed, 40))[1]).toBe("  ⎿  Error: no such thing");
});

test("an edit shows its counts and its old and new lines marked", () => {
  const edit = call("Edit", { file_path: "/a.ts", old_string: "a\nb", new_string: "c" }, "ok");
  const lines = linesOf(edit, 40);
  expect(text(lines).map((row) => row.trimEnd())).toEqual([
    "● Update(/a.ts)",
    "  ⎿  Added 1 line, removed 2 lines",
    "     -a",
    "     -b",
    "     +c",
  ]);
  // A marked row runs to the pane's edge, as the terminal's does.
  expect(Bun.stringWidth(text(lines)[2] ?? "")).toBe(40);
  expect(lines.slice(2).map((l) => l.spans[1]?.mark)).toEqual(["removed", "removed", "added"]);
});

// What a run of reads, searches and commands folds into is counted by rules
// cheaper to test than to try live.
test("a run of reads, searches and commands folds into one counted line", () => {
  const m = model(
    call("Read", { file_path: "/d1" }),
    call("Read", { file_path: "/d1" }),
    call("ToolSearch", { query: "q" }, ""),
    { id: "th", kind: "thinking", status: "settled", text: "hm" },
    call("Grep", { pattern: "x" }, "No files found"),
    call("Bash", { command: "ls docs" }, "a"),
    call("Bash", { command: "cd /a && grep -rl x ." }, "", "error"),
    { id: "t", kind: "text", status: "settled", text: "done" },
    call("Bash", { command: "cat a | wc -l" }, "1"),
  );
  expect(text(render(m, 80, 100))).toEqual([
    "  Searched for 1 pattern, read 1 file, listed 1 directory, ran 1 shell command",
    "",
    "∴ Thinking",
    "",
    "● done",
    "",
    "  Read 1 file",
  ]);
  const run = render(m, 80, 100)[0]?.spans.find((s) => s.text === "1");
  expect(run).toEqual({ text: "1", tone: "faint", bold: true });
  // While the turn works, its last run says what it does and on what.
  m.turn = "working";
  m.items.push(call("Bash", { command: "sleep 5", description: "Wait" }, undefined, "running"));
  expect(text(render(m, 80, 4))).toEqual(["● Wait…", "  ⎿  $ sleep 5", "", "· Working…"]);
});

test("a prompt and a letter wrap back to the edge, a letter's dot by its status", () => {
  const prompt: Item = { id: "p", kind: "prompt", status: "settled", text: "aaa bbb ccc" };
  expect(text(linesOf(prompt, 9))).toEqual(["❯ aaa bbb", "ccc"]);
  expect(linesOf(prompt, 9)[0]?.spans[0]).toEqual({ text: "❯ ", tone: "faint" });
  const letter = (status: string): Item => ({
    id: "l",
    kind: "prompt",
    status: "settled",
    text: `<task-notification><status>${status}</status><summary>s</summary></task-notification>`,
  });
  expect(linesOf(letter("failed"), 9)[0]?.spans[0]?.tone).toBe("error");
  expect(linesOf(letter("killed"), 9)[0]?.spans[0]?.tone).toBe("gap");
  expect(linesOf(letter("running"), 9)[0]?.spans[0]?.tone).toBeUndefined();
  const long: Item = { ...prompt, text: `${"a\n".repeat(6000)}` };
  expect(text(linesOf(long, 20))).toContain("… +3500 lines …");
});

test("a reply's heading, table and quote are drawn as the terminal draws them", () => {
  const reply: Item = {
    id: "t",
    kind: "text",
    status: "settled",
    text: "# Top\n\n\n> said\n\n| a | b |\n|---|---|\n| long cell | c |",
  };
  // The columns shrink to their longest word, the room left shared out.
  expect(text(linesOf(reply, 18))).toEqual([
    "● Top",
    "  ",
    "  ▎ said",
    "  ",
    "  ┌──────┬─────┐",
    "  │ a    │ b   │",
    "  ├──────┼─────┤",
    "  │ long │ c   │",
    "  │ cell │     │",
    "  └──────┴─────┘",
  ]);
  const top = linesOf(reply, 18)[0]?.spans[1];
  expect([top?.bold, top?.italic, top?.underline]).toEqual([true, true, true]);
});

// Seen live on 2.11.0: the native face showed through the pane's last column.
test("every row the face draws covers the pane's width", () => {
  const m = model(
    { id: "p", kind: "prompt", status: "settled", text: "a prompt" },
    { id: "t", kind: "text", status: "settled", text: "界".repeat(30) },
    {
      id: "o",
      kind: "tool",
      status: "error",
      name: "Bash",
      text: "{}",
      output: "\x1b[31mred\x1b[0m\tx",
    },
    { id: "th", kind: "thinking", status: "settled", text: "hm" },
    call("Edit", { file_path: "/a/b/c.ts", old_string: "x", new_string: "y" }, "ok"),
    call("Probe", { n: 1 }, Array.from({ length: 30 }, (_, i) => `line ${i}`).join("\n")),
    { id: "x", kind: "text", status: "settled", text: "| a | b |\n|---|---|\n| long cell | c |" },
  );
  for (const width of [9, 10, 16, 23]) {
    const rows = fill(render(m, width, 30), width, 30);
    expect(rows).toHaveLength(30);
    for (const row of rows)
      expect(Bun.stringWidth(row.spans.map((s) => s.text).join(""))).toBe(width);
  }
});

test("the turn's line says it works, and how long the last one took", () => {
  const said = (line: FaceLine | undefined) => text(line ? [line] : []);
  expect(said(turnLine("working", { now: 75_000, since: 3_000 }))[0]).toMatch(
    /^. \p{L}+… \(1m 12s\)$/u,
  );
  // A face that came mid-turn does not know its start, nor how long it took.
  expect(said(turnLine("working", { now: 75_000 }))[0]).toMatch(/^. Working…$/);
  const at = new Date(2026, 9, 30, 19, 5).getTime();
  expect(said(turnLine("idle", { now: 0, worked: { took: 37_400, at } }))[0]).toMatch(
    /^ {2}\p{L}+ for 37s · done 7:05 PM$/u,
  );
  expect(turnLine("idle", { now: 0 })).toBeUndefined();
});
