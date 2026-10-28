// The face's drawing as plain lines (docs/face.md, "The face"): the worker
// renders, the shell's thread only paints lines with their tones, so a face
// that changes or breaks never touches the shell's tree.

import type { Item } from "./mirror.gen.ts";
import type { FaceModel } from "./model.ts";

export type Tone = "prompt" | "text" | "thinking" | "tool" | "output" | "notice" | "gap";
export type FaceLine = { text: string; tone: Tone };

// A tool's output shows its first lines; the native face has the rest.
const outputLines = 8;

// wrap cuts a text into rows of at most width columns, by code point and
// its width in the terminal.
export function wrap(text: string, width: number): string[] {
  const rows: string[] = [];
  for (const line of text.split("\n")) {
    let row = "";
    let used = 0;
    for (const ch of line.replace(/\t/g, "  ")) {
      const w = Bun.stringWidth(ch);
      if (used + w > width && row !== "") {
        rows.push(row);
        row = "";
        used = 0;
      }
      row += ch;
      used += w;
    }
    rows.push(row);
  }
  return rows;
}

const marked = (rows: string[], first: string, rest: string) =>
  rows.map((row, i) => (i === 0 ? first : rest) + row);

// linesOf is one item's rows.
export function linesOf(item: Item, width: number): FaceLine[] {
  const inner = Math.max(width - 2, 1);
  const out: FaceLine[] = [];
  const push = (tone: Tone, rows: string[]) => {
    for (const text of rows) out.push({ text, tone });
  };
  switch (item.kind) {
    case "prompt": {
      const from = item.origin && item.origin !== "person" ? ` (${item.origin})` : "";
      push(
        item.status === "refused" ? "gap" : "prompt",
        marked(wrap(item.text + from, inner), "› ", "  "),
      );
      break;
    }
    case "text":
      push("text", marked(wrap(item.text || "…", inner), "  ", "  "));
      break;
    case "thinking":
      push("thinking", marked(wrap(item.text || "thinking…", inner), "∴ ", "  "));
      break;
    case "tool": {
      const input = item.text.replace(/\s+/g, " ");
      const head = `${item.name ?? "tool"} ${input}`;
      push("tool", [`● ${wrap(head, inner)[0] ?? ""}`]);
      if (item.output !== undefined) {
        const rows = wrap(item.output.trimEnd(), Math.max(inner - 2, 1));
        const shown = rows.slice(0, outputLines);
        if (rows.length > outputLines) shown.push(`… ${rows.length - outputLines} more lines`);
        push(
          item.status === "error" || item.status === "refused" ? "gap" : "output",
          marked(shown, "  ⎿ ", "    "),
        );
      } else if (item.status === "running") push("output", ["  ⎿ running…"]);
      break;
    }
    case "notice":
      push("notice", marked(wrap(item.text, inner), "· ", "  "));
      break;
  }
  if (item.status === "gap")
    out.push({ text: "  [lost on the way — the native face has it]", tone: "gap" });
  if (item.cut) out.push({ text: "  [cut — the native face has it whole]", tone: "notice" });
  return out;
}

// render is the last height rows of the model, scrolled up by back rows:
// only the tail is laid out, so a full model costs what the screen shows.
export function render(model: FaceModel, width: number, height: number, back = 0): FaceLine[] {
  const want = height + back;
  const rows: FaceLine[] = [];
  for (let i = model.items.length - 1; i >= 0 && rows.length < want; i--) {
    const item = model.items[i];
    if (!item) continue;
    const lines = linesOf(item, width);
    if (i > 0) lines.unshift({ text: "", tone: "text" });
    rows.unshift(...lines);
  }
  if (rows.length < want && model.dropped > 0)
    rows.unshift({
      text: `· ${model.dropped} earlier items left the face's memory`,
      tone: "notice",
    });
  const end = Math.max(rows.length - back, Math.min(rows.length, height));
  return rows.slice(Math.max(end - height, 0), end);
}
