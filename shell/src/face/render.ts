// The face's drawing as rows of styled spans (docs/face.md, "The face"): the
// worker renders, the shell's thread only paints the spans in the person's
// colours, so a face that changes or breaks never touches the shell's tree.

import { markdownRows } from "./markdown.ts";
import type { FaceLine, Span, Tone } from "./messages.ts";
import type { Item } from "./mirror.gen.ts";
import type { FaceModel } from "./model.ts";
import { headRowsOf, outputRowsOf } from "./tools.ts";
import { clean, widthOf, wrapSpans } from "./wrap.ts";

// marked puts a mark before the first row and an indent of its width before
// the others.
function marked(rows: Span[][], mark: Span, band?: boolean): FaceLine[] {
  const indent = { text: " ".repeat(widthOf(mark.text)) };
  return rows.map((row, i) => ({ spans: [i === 0 ? mark : indent, ...row], band }));
}

const plainRows = (text: string, width: number, base: Span = { text: "" }) =>
  text.split("\n").flatMap((line) => wrapSpans([{ ...base, text: line }], width));

// A letter rewake delivered reaches the harness as its notification: the face
// shows its summary, as a terminal program shows the notice it was given.
const notification = /<task-notification>[\s\S]*?<\/task-notification>/g;
const summary = /<summary>([\s\S]*?)<\/summary>/;

function promptLines(item: Item, width: number): FaceLine[] {
  const out: FaceLine[] = [];
  const text = clean(item.text);
  for (const m of text.matchAll(notification)) {
    const said = (summary.exec(m[0])?.[1] ?? m[0]).replace(/\s+/g, " ").trim();
    out.push(...marked(plainRows(said, width - 2), { text: "● ", tone: "done" }));
  }
  const rest = text.replace(notification, "").trim();
  if (rest || out.length === 0) {
    const tone: Tone | undefined = item.status === "refused" ? "error" : undefined;
    const rows = markdownRows(rest, width - 2, { text: "", tone });
    out.push(...marked(rows, { text: "❯ ", tone }, true));
  }
  return out;
}

const dotTone = (item: Item): Tone =>
  item.status === "running"
    ? "faint"
    : item.status === "error" || item.status === "refused"
      ? "error"
      : item.status === "gap"
        ? "gap"
        : "done";

function toolLines(item: Item, width: number): FaceLine[] {
  const out = marked(headRowsOf(item, width - 2), { text: "● ", tone: dotTone(item) });
  const mark = { text: "  ⎿  " };
  if (item.output === undefined) {
    if (item.status === "running")
      out.push(...marked([[{ text: "Running…", tone: "faint" }]], mark));
    return out;
  }
  const { rows, more } = outputRowsOf(item, width - 5);
  const tone: Tone | undefined =
    item.status === "error" || item.status === "refused" ? "error" : undefined;
  const shown = rows.map((row) => row.map((s) => (tone ? { ...s, tone } : s)));
  if (shown.length === 0 || (shown.length === 1 && shown[0]?.length === 0))
    shown.splice(0, shown.length, [{ text: "(No content)", tone: "faint" }]);
  if (more > 0) shown.push([{ text: `… +${more} lines`, tone: "faint" }]);
  out.push(...marked(shown, mark));
  return out;
}

// linesOf is one item's rows.
export function linesOf(item: Item, width: number): FaceLine[] {
  const out: FaceLine[] = [];
  switch (item.kind) {
    case "prompt":
      out.push(...promptLines(item, width));
      break;
    case "text":
      out.push(...marked(markdownRows(clean(item.text) || "…", width - 2), { text: "● " }));
      break;
    case "thinking": {
      const base: Span = { text: "", tone: "faint", italic: true };
      const rows = item.text.trim()
        ? markdownRows(clean(item.text), width - 2, base)
        : [[{ ...base, text: "Thinking…" }]];
      out.push(...marked(rows, { text: "✻ ", tone: "faint" }));
      break;
    }
    case "tool":
      out.push(...toolLines(item, width));
      break;
    case "notice":
      out.push(
        ...marked(plainRows(clean(item.text), width - 2, { text: "", tone: "faint" }), {
          text: "· ",
          tone: "faint",
        }),
      );
      break;
  }
  if (item.status === "gap")
    out.push({ spans: [{ text: "  [lost on the way — the native face has it]", tone: "gap" }] });
  if (item.cut)
    out.push({ spans: [{ text: "  [cut — the native face has it whole]", tone: "faint" }] });
  return out;
}

// An item is laid out again only when what is drawn of it changed: a stream
// redraws thirty times a second, and only its last block grows.
type Drawn = Pick<Item, "kind" | "status" | "text" | "output" | "name" | "cut"> & {
  width: number;
  lines: FaceLine[];
};
const laidOut = new WeakMap<Item, Drawn>();

function cachedLinesOf(item: Item, width: number): FaceLine[] {
  const hit = laidOut.get(item);
  if (
    hit &&
    hit.width === width &&
    hit.kind === item.kind &&
    hit.status === item.status &&
    hit.text === item.text &&
    hit.output === item.output &&
    hit.name === item.name &&
    hit.cut === item.cut
  )
    return hit.lines;
  const lines = linesOf(item, width);
  const { kind, status, text, output, name, cut } = item;
  laidOut.set(item, { kind, status, text, output, name, cut, width, lines });
  return lines;
}

// Clock is what the face knows of the turn's time: when the working turn
// began, or how long the last one took, each only when the face saw it.
export type Clock = { now: number; since?: number; worked?: { took: number; at: number } };

export function duration(ms: number): string {
  const s = Math.max(Math.floor(ms / 1000), 0);
  return s < 60 ? `${s}s` : `${Math.floor(s / 60)}m ${s % 60}s`;
}

// timeOf is a moment's local time as a terminal program says it, 7:01 AM.
export function timeOf(ms: number): string {
  const d = new Date(ms);
  const h = d.getHours();
  return `${h % 12 || 12}:${String(d.getMinutes()).padStart(2, "0")} ${h < 12 ? "AM" : "PM"}`;
}

// turnLine is the row under the conversation that says the turn goes on, or
// how long the last one took.
export function turnLine(turn: string, clock: Clock): FaceLine | undefined {
  const star = { text: "✻ ", tone: "accent" as const };
  if (turn === "working") {
    const spans: Span[] = [star, { text: "Working…", tone: "accent" }];
    if (clock.since !== undefined)
      spans.push({ text: ` (${duration(clock.now - clock.since)})`, tone: "faint" });
    return { spans };
  }
  if (turn === "compacting") return { spans: [star, { text: "Compacting…", tone: "accent" }] };
  if (turn === "idle" && clock.worked !== undefined) {
    const { took, at } = clock.worked;
    const said = `✻ Worked for ${duration(took)} · done ${timeOf(at)}`;
    return { spans: [{ text: said, tone: "faint" }] };
  }
  return undefined;
}

const blank = (): FaceLine => ({ spans: [] });

// render is the last height rows of the model, scrolled up by back rows:
// only the tail is laid out, so a full model costs what the screen shows.
export function render(
  model: FaceModel,
  width: number,
  height: number,
  back = 0,
  clock: Clock = { now: 0 },
): FaceLine[] {
  const want = height + back;
  const rows: FaceLine[] = [];
  const status = turnLine(model.turn, clock);
  if (status) rows.push(...(model.items.length > 0 ? [blank()] : []), status);
  for (let i = model.items.length - 1; i >= 0 && rows.length < want; i--) {
    const item = model.items[i];
    if (!item) continue;
    const lines = cachedLinesOf(item, width);
    rows.unshift(...(i > 0 ? [blank()] : []), ...lines);
  }
  if (rows.length < want && model.dropped > 0)
    rows.unshift({
      spans: [{ text: `· ${model.dropped} earlier items left the face's memory`, tone: "faint" }],
    });
  const end = Math.max(rows.length - back, Math.min(rows.length, height));
  return rows.slice(Math.max(end - height, 0), end);
}

// fill makes the drawing opaque: height rows, each as wide as the pane, the
// tail at the bottom. A terminal whose background the shell does not know
// gives the layer no colour to paint, and the native face under it would show
// through every cell the face leaves empty.
export function fill(lines: FaceLine[], width: number, height: number): FaceLine[] {
  const pad = (line: FaceLine): FaceLine => {
    const used = line.spans.reduce((n, s) => n + widthOf(s.text), 0);
    return used < width
      ? { ...line, spans: [...line.spans, { text: " ".repeat(width - used) }] }
      : line;
  };
  const top = Array.from({ length: Math.max(height - lines.length, 0) }, blank);
  return [...top, ...lines].map(pad);
}
