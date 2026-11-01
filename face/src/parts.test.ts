// What a click on a row of the face does (docs/face.md, "Copying" and
// "Unfolding"), through the part module's one interface: each row of the
// table a conversation drawn, the row clicked, and what the click gives.
import { expect, test } from "bun:test";
import type { FaceLine } from "./messages.ts";
import type { Item } from "./mirror.gen.ts";
import { FaceModel } from "./model.ts";
import { type Click, clickOn, type Fold, lit, topFor } from "./parts.ts";
import { bodyOf } from "./render.ts";
import { Unfold } from "./unfold.ts";

const drawn = (line: FaceLine) => line.spans.map((s) => s.text).join("");

const model = (items: Item[], turn: "idle" | "working" = "idle") => {
  const m = new FaceModel();
  m.apply({ op: "begin", items: items.length, turn });
  for (const item of items) m.apply({ op: "item", item });
  m.apply({ op: "end" });
  return m;
};

const reply = (text: string, id = "r"): Item => ({ id, kind: "text", status: "settled", text });
const call = (name: string, input: object, output?: string, id = name[0] ?? "c"): Item => ({
  id,
  kind: "tool",
  status: output === undefined ? "running" : "done",
  name,
  text: JSON.stringify(input),
  output,
});

const source = "Run it:\n\n```sh\nmake all\n```\n\n**done**";
const bash = call("Bash", { command: "make all" }, "1\n2\n3\n4\n5\n6", "b");
const mcp = call("mcp__s__t", { q: 1 }, "a\nb\nc\nd\ne\nf", "m");
const edit = call("Edit", { file_path: "/x", old_string: "old", new_string: "new" }, "updated");
const write = call("Write", { file_path: "/a.txt", content: "one\ntwo" }, "ok");
const steps = Array.from({ length: 14 }, (_, i) => `step ${i}`).join("\n\n");

type Gives = { copy: { text: string; what: string } } | { toggle: Fold } | undefined;

type Case = {
  what: string;
  items: Item[];
  // row is the clicked row as drawn, its trailing blanks left out.
  row: string;
  turn?: "working";
  // opened are the folds a click opened before.
  opened?: Fold[];
  gives: Gives;
};

const cases: Case[] = [
  {
    what: "a reply's row copies its markdown source",
    items: [reply(source)],
    row: "  done",
    gives: { copy: { text: source, what: "the reply" } },
  },
  {
    what: "a code block's row copies its code alone",
    items: [reply(source)],
    row: "  make all",
    gives: { copy: { text: "make all", what: "the code block" } },
  },
  {
    what: "a second code block's row copies its own code",
    items: [reply("```\none\n```\n\n```\ntwo\n```")],
    row: "  two",
    gives: { copy: { text: "two", what: "the code block" } },
  },
  {
    what: "a thinking block's row copies its text as written",
    items: [{ id: "th", kind: "thinking", status: "settled", text: "**Plan** it" }],
    row: "∴ Plan it",
    gives: { copy: { text: "**Plan** it", what: "the thinking" } },
  },
  {
    what: "a call's head copies its input as given when no field is shown",
    items: [mcp],
    row: "● s - t (MCP)(q: 1)",
    gives: { copy: { text: '{\n  "q": 1\n}', what: "the call's input" } },
  },
  {
    what: "an output's row copies the call's output whole",
    items: [mcp],
    row: "  ⎿  a",
    gives: { copy: { text: "a\nb\nc\nd\ne\nf", what: "the call's output" } },
  },
  {
    what: "an edit's red row copies its old lines",
    items: [edit],
    row: "     -old",
    gives: { copy: { text: "old", what: "the old lines" } },
  },
  {
    what: "an edit's green row copies its new lines",
    items: [edit],
    row: "     +new",
    gives: { copy: { text: "new", what: "the new lines" } },
  },
  {
    what: "a write's preview copies the file's lines",
    items: [write],
    row: "     two",
    gives: { copy: { text: "one\ntwo", what: "the file's lines" } },
  },
  {
    what: "a write whose input is not JSON copies the output drawn under it",
    items: [{ ...write, id: "w", text: '{"file_path":"/a' }],
    row: "  ⎿  ok",
    gives: { copy: { text: "ok", what: "the call's output" } },
  },
  {
    what: "an open run's ⎿ row copies its latest call's command",
    items: [{ ...bash, status: "running", output: undefined }],
    turn: "working",
    row: "  ⎿  $ make all",
    gives: { copy: { text: "make all", what: "the command" } },
  },
  {
    what: "a run's line opens the run, by its first call",
    items: [bash, reply("ok")],
    row: "  Ran 1 shell command (ctrl+o to expand)",
    gives: { toggle: { kind: "fold", id: "b" } },
  },
  {
    what: "an opened run's line folds it",
    items: [bash, reply("ok")],
    opened: [{ kind: "fold", id: "b" }],
    row: "  Ran 1 shell command (click to fold)",
    gives: { toggle: { kind: "fold", id: "b" } },
  },
  {
    what: "a cut output's row opens it",
    items: [mcp],
    row: "     … +3 lines (ctrl+o to expand)",
    gives: { toggle: { kind: "more", id: "m" } },
  },
  {
    what: "an opened output's row folds it",
    items: [mcp],
    opened: [{ kind: "more", id: "m" }],
    row: "     … −3 lines (click to fold)",
    gives: { toggle: { kind: "more", id: "m" } },
  },
  {
    what: "a cut thinking block's row opens it",
    items: [{ id: "th", kind: "thinking", status: "settled", text: steps }],
    row: "  … +17 lines (ctrl+o to expand)",
    gives: { toggle: { kind: "more", id: "th" } },
  },
  {
    what: "a prompt's row does nothing",
    items: [{ id: "p", kind: "prompt", status: "settled", text: "hi" }, reply("ok")],
    row: "❯ hi",
    gives: undefined,
  },
  {
    what: "the blank row between items does nothing",
    items: [reply("one", "a"), reply("two", "b")],
    row: "",
    gives: undefined,
  },
  {
    what: "an empty output copies nothing",
    items: [call("Probe", {}, "", "e")],
    row: "  ⎿  (No content)",
    gives: undefined,
  },
];

// clicked is what a click on the case's row gives, and the rows it lights.
function clicked(c: Case): {
  gives: Gives;
  click: Click | undefined;
  rows: FaceLine[];
  row: number;
} {
  const view = new Unfold();
  for (const fold of c.opened ?? []) view.toggle(fold);
  const m = model(c.items, c.turn === "working" ? "working" : "idle");
  const rows = bodyOf(m, 40, Infinity, view).rows;
  const row = rows.findIndex((l) => drawn(l).trimEnd() === c.row);
  if (row < 0) throw new Error(`no row ${c.row} in ${JSON.stringify(rows.map(drawn))}`);
  const click = clickOn(m.items, rows, row, 0);
  const gives: Gives =
    click === undefined
      ? undefined
      : "toggle" in click
        ? { toggle: click.toggle }
        : { copy: click.copy };
  return { gives, click, rows, row };
}

for (const c of cases)
  test(`a click: ${c.what}`, () => {
    const { gives, click, rows, row } = clicked(c);
    expect(gives).toEqual(c.gives);
    // A copy lights the rows of the clicked row's part, and only them.
    if (click && "copy" in click)
      expect(lit(rows, click.light).map((l) => l.lit === true)).toEqual(
        rows.map((l) => l.part === rows[row]?.part),
      );
  });

test("a part whose item has gone copies nothing", () => {
  const m = model([reply("ok")]);
  const rows = bodyOf(m, 40).rows;
  expect(clickOn(m.items, rows, 0, 0)).toBeDefined();
  expect(clickOn([], rows, 0, 0)).toBeUndefined();
  expect(clickOn(m.items, rows, rows.length, 0)).toBeUndefined();
});

test("a copy lights the rows of the part clicked, no other", () => {
  const m = model([reply(source)]);
  const rows = bodyOf(m, 40).rows;
  const row = rows.findIndex((l) => drawn(l).trimEnd() === "  make all");
  const click = clickOn(m.items, rows, row, 0);
  const shown = click && "copy" in click ? lit(rows, click.light) : rows;
  expect(shown.filter((l) => l.lit).map(drawn)).toEqual(["  make all"]);
  expect(lit(rows, undefined)).toBe(rows);
});

// The row a click opens or closes keeps its row in the view, whatever opened
// above or under it.
test("the row a click opens stays where it stood in the view", () => {
  const items = [
    call("Read", { file_path: "/a" }, "1\tx", "r"),
    reply("done", "t"),
    call("Probe", {}, Array.from({ length: 8 }, (_, i) => `line ${i + 1}`).join("\n"), "o"),
  ];
  const view = new Unfold();
  const m = model(items);
  const before = bodyOf(m, 40, Infinity, view).rows;
  const row = before.findIndex((l) => drawn(l).startsWith("     … +5 lines"));
  const click = clickOn(m.items, before, row, row - 2);
  expect(click && "toggle" in click ? click.toggle : undefined).toEqual({ kind: "more", id: "o" });
  if (!click || !("toggle" in click)) return;
  // The run above opens too, and the output grows under the row's place.
  view.toggle({ kind: "fold", id: "r" });
  view.toggle(click.toggle);
  const after = bodyOf(m, 40, Infinity, view).rows;
  const top = topFor(after, click.anchor);
  expect(after.slice(top, (top ?? 0) + 3).map(drawn)).toEqual([
    "     line 7",
    "     line 8",
    "     … −5 lines (click to fold)",
  ]);
  // A part gone leaves the view where it was.
  expect(topFor(bodyOf(model(items.slice(0, 2)), 40).rows, click.anchor)).toBeUndefined();
});
