// A tool call's head as the face draws it (docs/face.md, "The look"): the
// name Claude Code's terminal gives the tool and its main argument,
// Name(argument), with the name bold.

import type { Span } from "./messages.ts";
import type { Item } from "./mirror.gen.ts";
import { clean, wrapSpans } from "./wrap.ts";

type Input = Record<string, unknown>;

// An item's input is parsed once for each text it has: a redraw comes thirty
// times a second, and a tool's input may be 64 KiB.
const parsed = new WeakMap<Item, { text: string; input: Input | undefined }>();

// inputOf is a call's input as an object, or none when it is not one (cut to
// its bound, still streaming, or empty).
export function inputOf(item: Item): Input | undefined {
  const hit = parsed.get(item);
  if (hit && hit.text === item.text) return hit.input;
  let input: Input | undefined;
  try {
    const value: unknown = JSON.parse(item.text);
    if (value && typeof value === "object" && !Array.isArray(value)) input = value as Input;
  } catch {}
  parsed.set(item, { text: item.text, input });
  return input;
}

export const field = (input: Input | undefined, key: string): string | undefined => {
  const v = input?.[key];
  return typeof v === "string" ? v : undefined;
};

const oneLine = (text: string) => clean(text).replace(/\s+/g, " ").trim();

const home = process.env.HOME ?? "";

// shortPath is a path as the terminal shows it: under the home directory as
// ~/…; the session's own directory, which it shows paths relative to, is not
// in the mirror.
export function shortPath(path: string): string {
  return home && path.startsWith(`${home}/`) ? `~${path.slice(home.length)}` : path;
}

// Calls the terminal does not draw: a tool search loads tools, a to-do list
// draws as its own panel.
const unseen = new Set(["ToolSearch", "TodoWrite"]);

// A prompt the session holds is drawn under the conversation until it takes
// it (render.ts, statusOf), not where it was sent.
export const hidden = (item: Item) =>
  (item.kind === "tool" && unseen.has(item.name ?? "")) || item.status === "queued";

// nameOf is the name the terminal heads a call with.
export function nameOf(item: Item): string {
  const name = item.name ?? "tool";
  const mcp = /^mcp__(.+?)__(.+)$/.exec(name);
  if (mcp) return `${mcp[1]} - ${mcp[2]} (MCP)`;
  switch (name) {
    case "Edit":
    case "MultiEdit":
      return field(inputOf(item), "old_string") === "" ? "Create" : "Update";
    case "Grep":
    case "Glob":
      return "Search";
    case "WebFetch":
      return "Fetch";
    case "WebSearch":
      return "Web Search";
    default:
      return name;
  }
}

// A command is shown to two lines and 160 characters.
const commandLines = 2;
const commandChars = 160;

function commandOf(command: string): string {
  const lines = command.split("\n");
  let shown = lines.length > commandLines ? lines.slice(0, commandLines).join("\n") : command;
  if (shown.length > commandChars) shown = shown.slice(0, commandChars);
  return shown === command ? command : `${shown.trim()}…`;
}

const quoted = (v: unknown) => (typeof v === "string" ? JSON.stringify(v) : JSON.stringify(v));

// keyed is an input as key: value pairs, as the terminal heads a tool it has
// no words of its own for.
const keyed = (input: Input) =>
  Object.entries(input)
    .map(([k, v]) => `${k}: ${quoted(v)}`)
    .join(", ");

// argumentOf is what the head shows in parentheses: the tool's main field, or
// its input as typed when it does not read as an object.
export function argumentOf(item: Item): string {
  if (!item.text.trim()) return "";
  const input = inputOf(item);
  if (!input) return oneLine(item.text);
  const path = (key: string) => shortPath(field(input, key) ?? "");
  switch (item.name) {
    case "Bash":
      return oneLine(commandOf(field(input, "command") ?? ""));
    case "Read": {
      const pages = field(input, "pages");
      return pages ? `${path("file_path")} · pages ${pages}` : path("file_path");
    }
    case "Edit":
    case "MultiEdit":
    case "Write":
      return path("file_path");
    case "NotebookEdit":
      return path("notebook_path");
    case "Grep":
    case "Glob": {
      const where = field(input, "path");
      const pattern = `pattern: ${quoted(field(input, "pattern") ?? "")}`;
      return where ? `${pattern}, path: ${quoted(shortPath(where))}` : pattern;
    }
    case "WebFetch":
      return field(input, "url") ?? "";
    case "WebSearch":
      return quoted(field(input, "query") ?? "");
    case "Task":
    case "Agent":
      return oneLine(field(input, "description") ?? "");
    case "Skill":
      return field(input, "skill") ?? "";
    default:
      return oneLine(keyed(input));
  }
}

// A head longer than this many rows ends in an ellipsis.
const headRows = 3;

// headRowsOf is the call's head after its dot, at most a few rows unless
// the call is shown whole.
export function headRowsOf(item: Item, width: number, whole = false): Span[][] {
  const arg = argumentOf(item);
  const spans: Span[] = [{ text: nameOf(item), bold: true }];
  if (arg) spans.push({ text: `(${arg})` });
  const rows = wrapSpans(spans, width);
  if (whole || rows.length <= headRows) return rows;
  const kept = rows.slice(0, headRows);
  // The last row kept gives a cell to the ellipsis.
  const last = wrapSpans(kept[headRows - 1] ?? [], Math.max(width - 1, 1))[0] ?? [];
  kept[headRows - 1] = [...last, { text: "…" }];
  return kept;
}
