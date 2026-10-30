// A tool call as the face draws it (docs/face.md, "The face"): its name and
// main argument for a head, Name(argument) as a terminal program shows it,
// and its output folded to its first lines.

import type { Span } from "./messages.ts";
import type { Item } from "./mirror.gen.ts";
import { clean, wrapSpans } from "./wrap.ts";

// The input field a tool is known by; any other tool shows its input whole.
const mainField: Record<string, string> = {
  Bash: "command",
  Read: "file_path",
  Edit: "file_path",
  MultiEdit: "file_path",
  Write: "file_path",
  NotebookEdit: "notebook_path",
  Grep: "pattern",
  Glob: "pattern",
  WebFetch: "url",
  WebSearch: "query",
  Task: "description",
  Agent: "description",
  Skill: "skill",
};

// A head longer than this many rows ends in an ellipsis.
const headRows = 3;
// An output shows its first lines; the native face has the rest.
export const outputLines = 3;

const oneLine = (text: string) => clean(text).replace(/\s+/g, " ").trim();

// argumentOf is what the head shows in parentheses: the known field, or the
// input as typed when there is none or it does not read.
export function argumentOf(name: string | undefined, input: string): string {
  if (!input.trim()) return "";
  let value: unknown;
  try {
    value = JSON.parse(input);
  } catch {
    return oneLine(input);
  }
  const field = name === undefined ? undefined : mainField[name];
  if (field && value && typeof value === "object") {
    const main = (value as Record<string, unknown>)[field];
    if (typeof main === "string") return oneLine(main);
  }
  return oneLine(JSON.stringify(value));
}

// headRowsOf is the call's head after its dot, at most a few rows.
export function headRowsOf(item: Item, width: number): Span[][] {
  const arg = argumentOf(item.name, item.text);
  const spans: Span[] = [{ text: item.name ?? "tool", bold: true }];
  if (arg) spans.push({ text: `(${arg})` });
  const rows = wrapSpans(spans, width);
  if (rows.length <= headRows) return rows;
  const kept = rows.slice(0, headRows);
  const last = wrapSpans([...(kept[headRows - 1] ?? []), { text: "…" }], width);
  kept[headRows - 1] = last.length > 1 ? [...(last[0] ?? []), { text: "…" }] : (last[0] ?? []);
  return kept;
}

// outputRowsOf is what shows under the head, before the ⎿ mark: the first
// rows of the output and how many more there are, or a summary where the
// native face gives one.
export function outputRowsOf(item: Item, width: number): { rows: Span[][]; more: number } {
  const output = clean(item.output ?? "").trimEnd();
  if (item.name === "Read" && item.status === "done") {
    const n = output === "" ? 0 : output.split("\n").length;
    return {
      rows: [
        [
          { text: "Read " },
          { text: String(n), bold: true },
          { text: n === 1 ? " line" : " lines" },
        ],
      ],
      more: 0,
    };
  }
  const rows = output.split("\n").flatMap((line) => wrapSpans([{ text: line }], width));
  return { rows: rows.slice(0, outputLines), more: Math.max(rows.length - outputLines, 0) };
}
