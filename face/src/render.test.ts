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
    "",
  ]);
  expect(text(render(m, 40, 2))).toEqual(["  [lost on the way — the native face has it]", ""]);
  expect(text(render(m, 40, 2, 3))).toEqual(["● 1", "  2"]);
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
    "● Rewake: main task, 1 new message",
    "  ↳ Do it (a preview only: run rewake inbox)",
  ]);
  expect(lines[0]?.band).toBeUndefined();
  // Narrow, every wrapped row hangs under its text, the preview's under its own.
  expect(text(linesOf(letter, 24))).toEqual([
    "● Rewake: main task, 1",
    "  new message",
    "  ↳ Do it (a preview",
    "    only: run rewake",
    "    inbox)",
  ]);
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
    "     … +2 lines (ctrl+o to expand)",
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
    { id: "th", kind: "thinking", status: "settled", text: "" },
    call("Grep", { pattern: "x" }, "No files found"),
    call("Bash", { command: "ls docs" }, "a"),
    call("Bash", { command: "cd /a && grep -rl x ." }, "", "error"),
    { id: "t", kind: "text", status: "settled", text: "done" },
    call("Bash", { command: "cat a | wc -l" }, "1"),
  );
  expect(text(render(m, 80, 100))).toEqual([
    "  Read 1 file (ctrl+o to expand)",
    "",
    "∴ Thinking",
    "",
    "  Searched for 1 pattern, listed 1 directory, ran 1 shell command, 1 failed",
    "  (ctrl+o to expand)",
    "",
    "● done",
    "",
    "  Read 1 file (ctrl+o to expand)",
    "",
  ]);
  const run = render(m, 80, 100)[0]?.spans.find((s) => s.text === "1");
  expect(run).toEqual({ text: "1", tone: "faint", bold: true });
  // Review of c617953: a call lost on the way is marked within its run.
  const lost = model(call("Read", { file_path: "/d1" }, "1\tx", "gap"));
  expect(text(render(lost, 80, 100))).toEqual([
    "  Read 1 file (ctrl+o to expand)",
    "  [lost on the way — the native face has it]",
    "",
  ]);
  // While the turn works, its last run says what it does and on what.
  m.turn = "working";
  m.items.push(call("Bash", { command: "sleep 5", description: "Wait" }, undefined, "running"));
  expect(text(render(m, 80, 4))).toEqual([
    "● Wait… (ctrl+o to expand)",
    "  ⎿  $ sleep 5",
    "",
    "⠏ Working…",
  ]);
});

test("a prompt and a letter wrap under their text, a letter's dot by its status", () => {
  const prompt: Item = { id: "p", kind: "prompt", status: "settled", text: "aaa bbb ccc" };
  expect(text(linesOf(prompt, 9))).toEqual(["❯ aaa bbb", "  ccc"]);
  const two: Item = { ...prompt, text: "aaa bbb ccc\nddd" };
  expect(text(linesOf(two, 9))).toEqual(["❯ aaa bbb", "  ccc", "  ddd"]);
  expect(linesOf(prompt, 9)[0]?.spans[0]).toEqual({ text: "❯ ", tone: "faint", gutter: true });
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
  expect(text(linesOf(long, 20))).toContain("  … +3500 lines …");
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
// Review of c617953: an edit of a wide character at width 6 threw, a quote at
// width 4 drew rows of 5; every kind of item is drawn at every width down to 1.
test("every row the face draws covers the pane's width, however narrow", () => {
  const m = model(
    { id: "p", kind: "prompt", status: "settled", text: "a prompt\nand 界界 more" },
    // Second review of 7ddebbb: a cell counted by code point, not by grapheme.
    { id: "e1", kind: "prompt", status: "settled", text: "❤️ ok" },
    { id: "e2", kind: "prompt", status: "settled", text: "👨‍👩‍👧‍👦 ok" },
    {
      id: "l",
      kind: "prompt",
      status: "settled",
      text: "<task-notification><status>failed</status><summary>Rewake: a letter</summary></task-notification>",
    },
    {
      id: "t",
      kind: "text",
      status: "settled",
      text: `# Head\n${"界".repeat(30)}\n- item **b** \`c\`\n  1. nested\n> a quote\n---\n\`\`\`\ncode 界\n\`\`\`\n| a | b |\n|---|---|\n| long cell | c |`,
    },
    call("Bash", "{}", "\x1b[31mred\x1b[0m\tx", "error"),
    { id: "th", kind: "thinking", status: "settled", text: "" },
    { id: "n", kind: "notice", status: "settled", text: "a notice 界" },
    { id: "x", kind: "notice", status: "error", text: "Operation aborted 界" },
    call("Edit", { file_path: "/a/b/c.ts", old_string: "界", new_string: "x" }, "ok"),
    call("Write", { file_path: "/a/w.ts", content: "one 界\ntwo" }, "ok"),
    call("Probe", { n: 1 }, Array.from({ length: 30 }, (_, i) => `line ${i}`).join("\n")),
    { ...call("Probe", { n: 2 }, "x"), id: "g", status: "gap" },
    { ...call("Probe", { n: 3 }, "x"), id: "c", cut: true },
    call("Read", { file_path: "/a/r.ts" }, "1\tx", "gap"),
    call("Bash", { command: "sleep 9", description: "Wait 界" }, undefined, "running"),
  );
  for (const turn of ["working", "idle"] as const) {
    m.turn = turn;
    for (const width of [...Array.from({ length: 30 }, (_, i) => i + 1), 80]) {
      const clock = { now: 9_000, since: 1_000, worked: { took: 8_000, at: 9_000 } };
      const rows = fill(render(m, width, 400, 0, clock), width, 400);
      expect(rows).toHaveLength(400);
      for (const row of rows)
        expect(Bun.stringWidth(row.spans.map((s) => s.text).join(""))).toBe(width);
    }
  }
});

test("the turn's line says it works, and how long the last one took", () => {
  const said = (line: FaceLine | undefined) => text(line ? [line] : []);
  const progress = { phase: "tool-use", tokens: 0, tool: "Bash" } as const;
  expect(said(turnLine("working", { now: 75_000, since: 3_000, progress }))[0]).toMatch(
    /^. Running Bash… \(1m 12s\)$/u,
  );
  // A face that came mid-turn does not know its start, nor how long it took.
  expect(said(turnLine("working", { now: 75_000 }))[0]).toMatch(/^. Working…$/);
  const at = new Date(2026, 9, 30, 19, 5).getTime();
  expect(said(turnLine("idle", { now: 0, worked: { took: 37_400, at } }))[0]).toBe(
    "  Done in 37s · 7:05 PM",
  );
  expect(said(turnLine("idle", { now: 0, worked: { took: 10_000, at, stopped: true } }))[0]).toBe(
    "  Stopped after 10s · 7:05 PM",
  );
  expect(turnLine("idle", { now: 0 })).toBeUndefined();
});

// Brief 53: a thinking block that came before later calls of a run sat after
// them; it ends the run and sits where it came.
test("thinking sits where it came, between two folded runs", () => {
  const m = model(
    call("Read", { file_path: "/a" }),
    { id: "th", kind: "thinking", status: "settled", text: "" },
    call("Bash", { command: "make" }, "", "error"),
    { id: "n", kind: "notice", status: "error", text: "Error: aborted" },
  );
  expect(text(render(m, 60, 100))).toEqual([
    "  Read 1 file (ctrl+o to expand)",
    "",
    "∴ Thinking",
    "",
    "  Ran 1 shell command, 1 failed (ctrl+o to expand)",
    "",
    "  ⎿  Error: aborted",
    "",
  ]);
});

// Under a call the classifier let run the terminal program says so; a call
// it refused shows its reason in place of the error; any other permit shows
// nothing of itself.
test("a call auto mode's classifier settled says so under it", () => {
  const call = (over: Partial<Item>): Item => ({
    id: "c",
    kind: "tool",
    name: "WebFetch",
    status: "done",
    text: '{"url":"https://example.com"}',
    output: "fetched",
    ...over,
  });
  const rows = (item: Item) => text(linesOf(item, 200));
  expect(rows(call({ permit: { by: "classifier", allowed: true } })).at(-1)).toBe(
    "  ⎿  Allowed by auto mode classifier",
  );
  expect(
    rows(call({ name: "Agent", permit: { by: "classifier", allowed: true } })).join("\n"),
  ).not.toContain("Allowed by");
  expect(rows(call({ permit: { by: "rule", allowed: true } })).join("\n")).not.toContain(
    "Allowed by",
  );
  const denied = rows(
    call({
      status: "refused",
      output: "Permission for this action was denied. Reason: [Scope].",
      permit: { by: "classifier", allowed: false, reason: "[Scope] ∙ see the mode's page" },
    }),
  );
  // The face draws the refusal's text as the adapter gave it, in place of the error.
  expect(denied.slice(1)).toEqual([
    "  ⎿  Denied by auto mode classifier ∙ [Scope] ∙ see the mode's page",
  ]);
});

// An interrupt or a failure the session told is a red line under what it
// stopped, as the terminal program draws one; another notice stays faint.
test("a notice of an interrupt is red under its mark, another faint", () => {
  const aborted = linesOf(
    { id: "x", kind: "notice", status: "error", text: "Operation aborted" },
    80,
  );
  expect(text(aborted)).toEqual(["  ⎿  Operation aborted"]);
  expect(aborted[0]?.spans.map((s) => [s.gutter ?? false, s.tone])).toEqual([
    [true, undefined],
    [false, "error"],
  ]);
  const said = linesOf({ id: "n", kind: "notice", status: "settled", text: "lost" }, 80);
  expect(text(said)).toEqual(["· lost"]);
  expect(said[0]?.spans[1]?.tone).toBe("faint");
});

// Review of b1f3f12: the end line says "Stopped" from the turn line's own fact.
test("the model keeps whether the last turn line ended the turn by an interrupt", () => {
  const m = model();
  m.apply({ op: "turn", turn: "working" });
  expect(m.stopped).toBe(false);
  m.apply({ op: "turn", turn: "idle", stopped: true });
  expect(m.stopped).toBe(true);
  m.apply({ op: "turn", turn: "working" });
  expect(m.stopped).toBe(false);
});

// Brief 55: the turn's times are the wrapper model's, so a face that
// connects after a turn, or during one, draws what one that watched did, and
// the conversation keeps a blank row above the input with no Done line.
test("a face that connects after a turn draws its Done, one mid-turn its start", () => {
  const snapshot = (begin: object) => {
    const m = new FaceModel();
    m.apply({ op: "begin", items: 0, ...begin } as never);
    m.apply({ op: "end" } as never);
    m.apply({ op: "add", item: { id: "t", kind: "text", status: "settled", text: "hi" } } as never);
    return m;
  };
  const at = new Date(2026, 9, 31, 20, 43).getTime();
  const done = snapshot({ turn: "idle", took: 69_000, at });
  expect(text(render(done, 80, 10, 0, { now: 0, worked: done.worked }))).toEqual([
    "● hi",
    "",
    "  Done in 1m 9s · 8:43 PM",
  ]);
  const stopped = snapshot({ turn: "idle", took: 10_000, at, stopped: true });
  expect(stopped.worked?.stopped).toBe(true);
  const working = snapshot({ turn: "working", since: 5_000 });
  expect(working.since).toBe(5_000);
  expect(working.worked).toBeUndefined();
  working.apply({ op: "turn", turn: "idle", took: 3_000, at: 8_000 } as never);
  expect(working.worked).toEqual({ took: 3_000, at: 8_000, stopped: false });
  expect(working.since).toBeUndefined();
  working.apply({ op: "turn", turn: "working", since: 9_000 } as never);
  expect(working.worked).toBeUndefined();
  expect(working.since).toBe(9_000);
  // Never ended with a Done: a blank row still stands above the input.
  expect(text(render(snapshot({ turn: "idle" }), 80, 10))).toEqual(["● hi", ""]);
});
