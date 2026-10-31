// What shows under a call's head, after the ⎿ mark (docs/face.md, "The
// look"): a counted line where Claude Code's terminal gives one, the old and
// new lines of an edit, or the first rows of the output and how many more.

import type { Span } from "./messages.ts";
import type { Item } from "./mirror.gen.ts";
import { field, inputOf, shortPath } from "./tools.ts";
import { clean, widthOf, wrapSpans } from "./wrap.ts";

export type Result = { rows: Span[][]; more: number };

// An output shows three rows, or four when only one more would be left.
const outputRows = 3;
// An error shows ten.
const errorRows = 10;
// A written file shows its first ten lines.
const writtenRows = 10;
// An edit shows at most this many rows of its old and new lines.
const diffRows = 40;

const lines = (text: string) => (text === "" ? [] : text.split("\n"));
const plural = (n: number, one: string) => `${one}${n === 1 ? "" : "s"}`;

// counted is "Verb N things" with the number bold, as the terminal says it.
const counted = (before: string, n: number, after: string): Span[] => [
  { text: before },
  { text: String(n), bold: true },
  { text: after },
];

// fold keeps the first rows, one more when it would be the only one left.
function fold(rows: Span[][], keep: number): Result {
  const shown = rows.length <= keep + 1 ? rows : rows.slice(0, keep);
  return { rows: shown, more: rows.length - shown.length };
}

// A line that is JSON as a whole is shown pretty-printed.
function pretty(text: string): string {
  return text
    .split("\n")
    .map((line) => {
      const t = line.trim();
      if (!/^[[{]/.test(t)) return line;
      try {
        return JSON.stringify(JSON.parse(t), null, 2);
      } catch {
        return line;
      }
    })
    .join("\n");
}

const rowsOf = (text: string, width: number, base: Span = { text: "" }) =>
  text.split("\n").flatMap((line) => wrapSpans([{ ...base, text: line }], width));

// errorText is a failed call's words: the tool's tags dropped, said as an
// error.
function errorText(output: string): string {
  const said = output.replace(/<\/?tool_use_error>|<\/?error>/g, "").trim();
  return /^(Error|Cancelled): /.test(said) ? said : `Error: ${said}`;
}

// diff is an edit's old lines on red and new lines on green, each row the
// width of the area, as the terminal marks a patch.
function diff(item: Item, width: number): Span[][] {
  const input = inputOf(item);
  const edits: unknown[] = Array.isArray(input?.edits) ? input.edits : [input];
  const rows: Span[][] = [];
  for (const edit of edits) {
    const e = edit as Record<string, unknown> | undefined;
    const old = typeof e?.old_string === "string" ? e.old_string : "";
    const now = typeof e?.new_string === "string" ? e.new_string : "";
    // The sign and the padding are drawing: a selection copies the line.
    for (const [sign, mark, text] of [
      ["-", "removed", old],
      ["+", "added", now],
    ] as const)
      for (const line of lines(clean(text)))
        for (const row of wrapSpans([{ text: sign, gutter: true }, { text: line }], width)) {
          const used = row.reduce((n, s) => n + widthOf(s.text), 0);
          const pad = " ".repeat(Math.max(width - used, 0));
          rows.push([
            ...row.map((s) => ({ ...s, mark })),
            ...(pad ? [{ text: pad, mark, gutter: true }] : []),
          ]);
        }
  }
  return rows;
}

function editResult(item: Item, width: number): Result {
  const input = inputOf(item);
  const edits: unknown[] = Array.isArray(input?.edits) ? input.edits : [input];
  let added = 0;
  let removed = 0;
  for (const edit of edits) {
    const e = edit as Record<string, unknown> | undefined;
    removed += lines(typeof e?.old_string === "string" ? e.old_string : "").length;
    added += lines(typeof e?.new_string === "string" ? e.new_string : "").length;
  }
  const said: Span[] = [
    ...counted("Added ", added, ` ${plural(added, "line")}, `),
    ...counted("removed ", removed, ` ${plural(removed, "line")}`),
  ];
  const patch = diff(item, width);
  const rows = [said, ...patch.slice(0, diffRows)];
  return { rows, more: Math.max(patch.length - diffRows, 0) };
}

function writeResult(item: Item, width: number): Result {
  const input = inputOf(item);
  const content = lines(clean(field(input, "content") ?? "").replace(/\n$/, ""));
  const path = shortPath(field(input, "file_path") ?? "");
  const said: Span[] = [
    ...counted("Wrote ", content.length, ` ${plural(content.length, "line")} to `),
    { text: path, bold: true },
  ];
  const body = content.slice(0, writtenRows).flatMap((line) => rowsOf(line, width));
  return { rows: [said, ...body], more: Math.max(content.length - writtenRows, 0) };
}

// resultOf is the rows under a finished call's head; reads, searches and
// commands fold into their run's line instead (fold.ts).
export function resultOf(item: Item, width: number): Result {
  const output = clean(item.output ?? "").trimEnd();
  if (item.status === "error" || item.status === "refused") {
    const rows = rowsOf(errorText(output), width, { text: "", tone: "error" });
    return fold(rows, errorRows);
  }
  if (item.status === "done")
    switch (item.name) {
      case "Edit":
      case "MultiEdit":
        if (inputOf(item)) return editResult(item, width);
        break;
      case "Write":
        if (inputOf(item)) return writeResult(item, width);
        break;
    }
  if (output === "" || output === "(Bash completed with no output)")
    return {
      rows: [[{ text: item.name === "Bash" ? "(No output)" : "(No content)", tone: "faint" }]],
      more: 0,
    };
  return fold(rowsOf(pretty(output), width), outputRows);
}
