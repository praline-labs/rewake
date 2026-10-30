// A reply's markdown as the face draws it (docs/face.md, "The face"): bold,
// italic and inline code within a line, headings bold, list items hanging
// under their marker, a fenced block as it is written. Only what a terminal
// shows; anything else stays as typed.
import type { Span } from "./messages.ts";
import { wrapSpans } from "./wrap.ts";

const inline =
  /(`+)(.+?)\1|\*\*(.+?)\*\*|__(.+?)__|(?<![\w*])\*(?![\s*])(.+?)(?<![\s*])\*(?![\w*])|(?<![\w_])_(?![\s_])(.+?)(?<![\s_])_(?![\w_])/g;

// spansOfInline is one line's inline markdown as spans over a base style.
export function spansOfInline(text: string, base: Span = { text: "" }): Span[] {
  const out: Span[] = [];
  const plain = (s: string) => s && out.push({ ...base, text: s });
  let at = 0;
  for (const m of text.matchAll(inline)) {
    plain(text.slice(at, m.index));
    at = m.index + m[0].length;
    if (m[2] !== undefined) out.push({ ...base, text: m[2], tone: "code" });
    else if (m[3] !== undefined || m[4] !== undefined)
      out.push(...spansOfInline(m[3] ?? m[4] ?? "", { ...base, bold: true }));
    else out.push(...spansOfInline(m[5] ?? m[6] ?? "", { ...base, italic: true }));
  }
  plain(text.slice(at));
  return out;
}

const fence = /^\s*(```|~~~)/;
const heading = /^#{1,6}\s+(.*)$/;
const item = /^(\s*)([-*+]|\d+[.)])\s+(.*)$/;

const indent = (n: number): Span => ({ text: " ".repeat(n) });

// markdownRows lays a text out as rows of at most width columns.
export function markdownRows(text: string, width: number, base: Span = { text: "" }): Span[][] {
  const rows: Span[][] = [];
  let inCode = false;
  for (const line of text.split("\n")) {
    if (fence.test(line)) {
      inCode = !inCode;
      continue;
    }
    if (inCode) {
      rows.push(...wrapSpans([{ ...base, text: line }], width));
      continue;
    }
    const h = heading.exec(line);
    if (h) {
      rows.push(...wrapSpans(spansOfInline(h[1] ?? "", { ...base, bold: true }), width));
      continue;
    }
    const li = item.exec(line);
    if (li) {
      const marker = `${li[1] ?? ""}${li[2] === "*" || li[2] === "+" ? "-" : li[2]} `;
      const hang = Math.min(Bun.stringWidth(marker), Math.max(width - 1, 0));
      const body = wrapSpans(spansOfInline(li[3] ?? "", base), Math.max(width - hang, 1));
      body.forEach((row, i) => {
        rows.push([i === 0 ? { ...base, text: marker } : indent(hang), ...row]);
      });
      continue;
    }
    rows.push(...wrapSpans(spansOfInline(line, base), width));
  }
  return rows;
}
