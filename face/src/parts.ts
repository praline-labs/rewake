// What a click on a row of the face acts on (docs/face.md, "Copying" and
// "Unfolding"), in one place: the part each row is of, and what a click on it
// does. Most parts copy whole — a reply as its markdown source, a code
// block's code, a thinking block's text, a call's input (a command, a path, a
// pattern, else the input as given), a call's output, an edit's old or new
// lines, a written file's lines — their text taken from the model when the
// click comes, so the rows carry no copy of it. Two open or close instead: a
// run's line, and the row that counts the lines a part leaves out, the row
// clicked kept where it stood in the view.
import { callOf } from "./call.ts";
import { codeBlocks } from "./markdown.ts";
import type { FaceLine } from "./messages.ts";
import type { Item } from "./mirror.gen.ts";

// Fold is what a click opens or closes: a run, by its first call ("fold"),
// or an output or thinking block cut short ("more").
export type Fold = { kind: "fold" | "more"; id: string };

// Part is what a row is of: an item's part a click copies, its n-th code
// block among them, or a fold.
export type Part =
  | { kind: "reply" | "thinking" | "input" | "output" | "old" | "new" | "content"; id: string }
  | { kind: "code"; id: string; n: number }
  | Fold;

// keyOf is a part as a row carries it, the string that crosses to the shell
// with the row (a FaceLine's part); readKey reads it back. The grammar is
// this module's alone.
export const keyOf = (part: Part): string =>
  part.kind === "code" ? `code${part.n} ${part.id}` : `${part.kind} ${part.id}`;

const isFold = (part: Part): part is Fold => part.kind === "fold" || part.kind === "more";

function readKey(key: string): Part | undefined {
  const at = key.indexOf(" ");
  if (at < 0) return undefined;
  const kind = key.slice(0, at);
  const id = key.slice(at + 1);
  const code = /^code(\d+)$/.exec(kind);
  if (code) return { kind: "code", id, n: Number(code[1]) };
  switch (kind) {
    case "reply":
    case "thinking":
    case "input":
    case "output":
    case "old":
    case "new":
    case "content":
    case "fold":
    case "more":
      return { kind, id };
  }
  return undefined;
}

// resultOf is what a row under a call copies: a written file's lines where
// they are drawn, as results.ts draws them from an input that reads, else an
// edit's old or new lines by the row's mark, else the call's output.
function resultOf(item: Item, line: FaceLine): Part {
  const { id } = item;
  if (item.status === "done" && callOf(item).written) return { kind: "content", id };
  const mark = line.spans.find((s) => s.mark)?.mark;
  return { kind: mark === "removed" ? "old" : mark === "added" ? "new" : "output", id };
}

// Rows is what rows are, which names the part of each: a reply's (its code
// blocks' their own), a thinking block's text, a call's head or what an open
// run says its latest call works on ("input"), the rows under a call
// ("result"), a cut row ("more"), a run's line ("fold", by its first call).
export type Rows = "reply" | "thinking" | "input" | "result" | "more" | "fold";

// tagged is the rows with the part a click on each acts on.
export function tagged(lines: FaceLine[], item: Item, rows: Rows): FaceLine[] {
  const { id } = item;
  const partOf = (line: FaceLine): Part => {
    switch (rows) {
      case "reply": {
        const n = line.spans.find((s) => s.code !== undefined)?.code;
        return n === undefined ? { kind: "reply", id } : { kind: "code", id, n };
      }
      case "result":
        return resultOf(item, line);
      default:
        return { kind: rows, id };
    }
  };
  return lines.map((line) => ({ ...line, part: keyOf(partOf(line)) }));
}

// edits is an edit's old or new strings, one after another.
const edits = (item: Item, key: "old" | "new") =>
  (callOf(item).edits ?? [])
    .map((e) => e[key])
    .filter((s): s is string => s !== undefined)
    .join("\n");

type Copy = { text: string; what: string };

function copyOf(item: Item, part: Exclude<Part, Fold>): Copy {
  switch (part.kind) {
    case "code":
      return { text: codeBlocks(item.text)[part.n] ?? "", what: "the code block" };
    case "reply":
      return { text: item.text, what: "the reply" };
    case "thinking":
      return { text: item.text, what: "the thinking" };
    case "input":
      return callOf(item).copy;
    case "output":
      return { text: item.output ?? "", what: "the call's output" };
    case "old":
      return { text: edits(item, "old"), what: "the old lines" };
    case "new":
      return { text: edits(item, "new"), what: "the new lines" };
    case "content":
      return { text: callOf(item).written?.content ?? "", what: "the file's lines" };
  }
}

// Anchor is a row to keep where it stood in the view once the conversation is
// laid out again: its part, its place among that part's rows and its row in
// the view.
export type Anchor = { key: string; k: number; y: number };

// anchorAt is the anchor of a row of the view whose first row is top, none
// for a row of no part.
export function anchorAt(rows: FaceLine[], row: number, top: number): Anchor | undefined {
  const key = rows[row]?.part;
  if (key === undefined) return undefined;
  return { key, k: row - rows.findIndex((r) => r.part === key), y: row - top };
}

// topFor is the first row of a view that shows the anchor's row where it
// stood, none when its part is gone; a part grown shorter keeps its last row.
export function topFor(rows: FaceLine[], { key, k, y }: Anchor): number | undefined {
  const first = rows.findIndex((r) => r.part === key);
  if (first < 0) return undefined;
  const last = rows.findLastIndex((r) => r.part === key);
  return Math.max(first + Math.min(k, last - first) - y, 0);
}

// Click is what a click on a row does: copy a part's text, lighting its
// rows, or open or close a fold, keeping the row by its anchor.
export type Click = { copy: Copy; light: Part } | { toggle: Fold; anchor: Anchor };

// clickOn is what a click on a row of the view whose first row is top does,
// none for a row of no part, a part whose item is gone, or an empty one.
export function clickOn(
  items: Item[],
  rows: FaceLine[],
  row: number,
  top: number,
): Click | undefined {
  const key = rows[row]?.part;
  const part = key === undefined ? undefined : readKey(key);
  if (!part) return undefined;
  if (isFold(part)) {
    const anchor = anchorAt(rows, row, top);
    return anchor && { toggle: part, anchor };
  }
  const item = items.find((i) => i.id === part.id);
  const copy = item && copyOf(item, part);
  return copy?.text ? { copy, light: part } : undefined;
}

// lit is the rows with those of one part marked, the part a click just
// copied.
export function lit(rows: FaceLine[], part: Part | undefined): FaceLine[] {
  if (!part) return rows;
  const key = keyOf(part);
  return rows.map((line) => (line.part === key ? { ...line, lit: true } : line));
}
