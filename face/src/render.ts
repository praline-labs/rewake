// The face's drawing as rows of styled spans (docs/face.md, "The face"): the
// worker renders, the shell's thread only paints the spans in the person's
// colours, so a face that changes or breaks never touches the shell's tree.

import { entriesOf, lostLine, runLines } from "./fold.ts";
import { hung, marked } from "./hang.ts";
import { markdownRows } from "./markdown.ts";
import type { FaceLine, Span, Tone } from "./messages.ts";
import type { Item } from "./mirror.gen.ts";
import type { FaceModel } from "./model.ts";
import { promptLines } from "./prompt.ts";
import { resultOf } from "./results.ts";
import { type PartKind, partKey } from "./select.ts";
import { headRowsOf } from "./tools.ts";
import { blinkOff, type Clock, turnLine } from "./turn.ts";
import { type Opened, shut } from "./unfold.ts";
import { clean, widthOf, wrapSpans } from "./wrap.ts";

// parted names the part a click on each row copies: kind gives it by the
// row, none for a row of no part.
const parted = (
  lines: FaceLine[],
  item: Item,
  kind: (line: FaceLine) => PartKind | undefined,
): FaceLine[] =>
  lines.map((line) => {
    const k = kind(line);
    return k ? { ...line, part: partKey(k, item) } : line;
  });

// resultPart is what a row under a call copies: an edit's old or new lines
// by its mark, a written file's lines, else the call's output.
function resultPart(item: Item, line: FaceLine): PartKind {
  if (item.status === "done" && item.name === "Write") return "content";
  const mark = line.spans.find((s) => s.mark)?.mark;
  return mark === "removed" ? "old" : mark === "added" ? "new" : "output";
}

const dotTone = (item: Item): Tone =>
  item.status === "running"
    ? "faint"
    : item.status === "error" || item.status === "refused"
      ? "error"
      : item.status === "gap"
        ? "gap"
        : "done";

// permitRows is what the terminal program says under a call of how it was
// let run or refused, where it says anything: only of auto mode's
// classifier. A refusal's line stands for the error it refused with, its
// text the session's own, as the adapter gave it.
function permitRows(item: Item): Span[][] | undefined {
  const p = item.permit;
  if (p?.by !== "classifier") return undefined;
  if (p.allowed)
    return item.status === "done" && item.name !== "Agent"
      ? [[{ text: "Allowed by auto mode classifier", tone: "faint" }]]
      : undefined;
  if (item.status !== "refused" && item.status !== "error") return undefined;
  const text = p.reason
    ? `Denied by auto mode classifier ∙ ${p.reason}`
    : "Denied by auto mode classifier";
  return [[{ text, tone: "faint" }]];
}

// cutRow is the faint row that counts the lines a folded part leaves out, or
// the lines an opened one shows past its fold; a click on it opens or closes
// the part. Where everything is open nothing is counted.
function cutRow(more: number, open: boolean, view: Opened): Span[] | undefined {
  if (more <= 0 || (open && view.all)) return undefined;
  return [{ text: `… ${open ? "−" : "+"}${more} lines`, tone: "faint" }];
}

function toolLines(item: Item, width: number, view: Opened): FaceLine[] {
  const open = view.open(partKey("more", item));
  const dot: Span = { text: "● ", tone: dotTone(item), blink: item.status === "running" };
  const out = parted(marked(headRowsOf(item, width - 2, open), dot), item, () => "input");
  const mark = { text: "  ⎿  " };
  if (item.output === undefined) {
    if (item.status === "running")
      out.push(...marked([[{ text: "Running…", tone: "faint" }]], mark));
    return out;
  }
  const permit = permitRows(item);
  const under = (rows: Span[][]) =>
    marked(
      rows.flatMap((row) => wrapSpans(row, width - 5)),
      mark,
    );
  if (permit && !item.permit?.allowed) return [...out, ...under(permit)];
  const { rows, keep, more } = resultOf(item, width - 5);
  const cut = cutRow(more, open, view);
  const shown = open ? rows : rows.slice(0, keep);
  const lines = under(cut ? [...shown, cut] : shown);
  const last = lines.at(-1);
  out.push(
    ...parted(lines, item, (line) => (cut && line === last ? "more" : resultPart(item, line))),
  );
  if (permit) out.push(...under(permit));
  return out;
}

// A summary of the model's thinking shows as pi's screen draws it: markdown,
// faint and italic, whole; a long one folds after this many rows.
const thinkingRows = 10;

// thinkingLines is a thinking block: its summary after a faint `∴`, or only
// `∴ Thinking` where the session sent no text. A click on its text copies it.
function thinkingLines(item: Item, width: number, view: Opened): FaceLine[] {
  const text = clean(item.text).trim();
  if (text === "")
    return wrapSpans([{ text: "∴ Thinking", tone: "faint", italic: true }], width).map((spans) => ({
      spans,
    }));
  const rows = markdownRows(text, width - 2, { text: "", tone: "faint", italic: true }).map((row) =>
    row.map(({ code: _, ...span }) => span),
  );
  const open = view.open(partKey("more", item));
  const more = rows.length - thinkingRows;
  const cut = more > 1 ? cutRow(more, open, view) : undefined;
  if (more > 1 && !open) rows.splice(thinkingRows, more);
  const said = parted(marked(rows, { text: "∴ ", tone: "faint" }), item, () => "thinking");
  return cut ? [...said, ...parted(marked([cut], { text: "  " }), item, () => "more")] : said;
}

// linesOf is one item's rows, shown whole where the view opened it.
export function linesOf(item: Item, width: number, view: Opened = shut): FaceLine[] {
  const out: FaceLine[] = [];
  switch (item.kind) {
    case "prompt":
      out.push(...promptLines(item, width));
      break;
    case "text":
      out.push(
        ...parted(
          marked(markdownRows(clean(item.text) || "…", width - 2), { text: "● " }),
          item,
          (line) => {
            const code = line.spans.find((s) => s.code !== undefined)?.code;
            return code === undefined ? "reply" : `code${code}`;
          },
        ),
      );
      break;
    case "thinking":
      out.push(...thinkingLines(item, width, view));
      break;
    case "tool":
      out.push(...toolLines(item, width, view));
      break;
    case "notice":
      // A failure or an interrupt the session told, in red under what it
      // stopped, as the terminal program draws its own.
      if (item.status === "error") {
        out.push(...hung(clean(item.text), width, { text: "  ⎿  " }, { text: "", tone: "error" }));
        break;
      }
      out.push(
        ...marked(
          clean(item.text)
            .split("\n")
            .flatMap((line) => wrapSpans([{ text: line, tone: "faint" }], width - 2)),
          { text: "· ", tone: "faint" },
        ),
      );
      break;
  }
  if (item.status === "gap") out.push(lostLine);
  if (item.cut)
    out.push({ spans: [{ text: "  [cut — the native face has it whole]", tone: "faint" }] });
  return out;
}

// An item is laid out again only when what is drawn of it changed: a stream
// redraws on every tick of the face's clock, and only its last block grows.
type Drawn = Pick<Item, "kind" | "status" | "text" | "output" | "name" | "cut" | "permit"> & {
  width: number;
  open: boolean;
  all: boolean;
  lines: FaceLine[];
};
const laidOut = new WeakMap<Item, Drawn>();

function cachedLinesOf(item: Item, width: number, view: Opened): FaceLine[] {
  const hit = laidOut.get(item);
  const open = view.open(partKey("more", item));
  if (
    hit &&
    hit.width === width &&
    hit.open === open &&
    hit.all === view.all &&
    hit.kind === item.kind &&
    hit.status === item.status &&
    hit.text === item.text &&
    hit.output === item.output &&
    hit.name === item.name &&
    hit.cut === item.cut &&
    hit.permit === item.permit
  )
    return hit.lines;
  const lines = linesOf(item, width, view);
  const { kind, status, text, output, name, cut, permit } = item;
  laidOut.set(item, {
    kind,
    status,
    text,
    output,
    name,
    cut,
    permit,
    width,
    lines,
    open,
    all: view.all,
  });
  return lines;
}

const blank = (): FaceLine => ({ spans: [] });

// runRows is a run's rows: its line, and once opened each call under it as
// a call alone is drawn.
function runRows(run: { calls: Item[]; open: boolean }, width: number, view: Opened): FaceLine[] {
  const first = run.calls[0];
  const open = first !== undefined && view.open(partKey("fold", first));
  const lines = runLines(run, width, open);
  if (!open) return lines;
  return [...lines, ...run.calls.flatMap((call) => [blank(), ...cachedLinesOf(call, width, view)])];
}

// statusOf is the rows under the conversation that move with the clock: the
// turn's line after a blank row; with no line, the blank row alone, so the
// conversation never sits on the input.
export function statusOf(model: FaceModel, clock: Clock, width = 80): FaceLine[] {
  const status = turnLine(model.turn, clock, width);
  if (model.items.length === 0) return status ? [status] : [];
  return status ? [blank(), status] : [blank()];
}

// bodyOf is the conversation's rows from the top, at least its last want of
// them, and whether they are all of them: only the tail is laid out, so a
// full model costs what the screen shows. Nothing in them moves with the
// clock, so a scroll can cut them again without laying them out.
export function bodyOf(
  model: FaceModel,
  width: number,
  want = Infinity,
  view: Opened = shut,
): { rows: FaceLine[]; whole: boolean } {
  const rows: FaceLine[] = [];
  const entries = entriesOf(model.items, model.turn === "working");
  let i = entries.length - 1;
  for (; i >= 0 && rows.length < want; i--) {
    const entry = entries[i];
    if (!entry) continue;
    const lines =
      "item" in entry ? cachedLinesOf(entry.item, width, view) : runRows(entry, width, view);
    rows.unshift(...(i > 0 ? [blank()] : []), ...lines);
  }
  const whole = i < 0;
  if (whole && model.dropped > 0)
    rows.unshift({
      spans: [{ text: `· ${model.dropped} earlier items left the face's memory`, tone: "faint" }],
    });
  return { rows, whole };
}

// rowsOf is the model's rows from the top, at least its last want of them.
export function rowsOf(model: FaceModel, width: number, clock: Clock, want = Infinity): FaceLine[] {
  const status = statusOf(model, clock, width);
  return [...bodyOf(model, width, want - status.length).rows, ...status];
}

// render is the last height rows of the model, scrolled up by back rows.
export function render(
  model: FaceModel,
  width: number,
  height: number,
  back = 0,
  clock: Clock = { now: 0 },
): FaceLine[] {
  const rows = rowsOf(model, width, clock, height + back);
  const end = Math.max(rows.length - back, Math.min(rows.length, height));
  return rows.slice(Math.max(end - height, 0), end);
}

// blinked is the rows at a moment: a running call's dot hidden in the half of
// its blink that is off, the cell kept.
export function blinked(lines: FaceLine[], now: number): FaceLine[] {
  if (!blinkOff(now)) return lines;
  return lines.map((line) =>
    line.spans.some((s) => s.blink)
      ? {
          ...line,
          spans: line.spans.map((s) => (s.blink ? { ...s, text: " ".repeat(widthOf(s.text)) } : s)),
        }
      : line,
  );
}

// fill makes the drawing opaque: height rows, each as wide as the pane, the
// tail at the bottom. A terminal whose background the shell does not know
// gives the layer no colour to paint, and the native face under it would show
// through every cell the face leaves empty.
// A row wider than the pane, a mark or a gutter on a pane narrower than it,
// is cut at the pane's edge: a wide character the edge splits becomes a space.
// A cell is a grapheme, not a code point: an emoji with its selector or its
// joined parts is one, as the terminal draws it.
const graphemes = new Intl.Segmenter();
export function fill(lines: FaceLine[], width: number, height: number): FaceLine[] {
  const pad = (line: FaceLine): FaceLine => {
    const spans: Span[] = [];
    let used = 0;
    let cut = false;
    for (const span of line.spans) {
      let text = "";
      for (const { segment } of graphemes.segment(span.text)) {
        cut = used + widthOf(segment) > width;
        if (cut) break;
        text += segment;
        used += widthOf(segment);
      }
      if (text) spans.push({ ...span, text });
      if (cut) break;
    }
    return {
      ...line,
      spans: used < width ? [...spans, { text: " ".repeat(width - used), gutter: true }] : spans,
    };
  };
  const top = Array.from({ length: Math.max(height - lines.length, 0) }, blank);
  return [...top, ...lines].map(pad);
}
