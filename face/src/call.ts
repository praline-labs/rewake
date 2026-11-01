// A tool call as the face knows it (docs/face.md, "Calls"): every harness's
// adapter names its calls by Claude Code's tools and gives their input in
// those tools' fields, and this is the one place the face reads that
// vocabulary. An item goes in; out comes all the face draws of the call —
// its head, what it does in a run, what a click copies, what is under it —
// read from its input, parsed once. The callers only draw, fold and copy.

import type { Item } from "./mirror.gen.ts";
import { clean } from "./wrap.ts";

type Input = Record<string, unknown>;

// Op is what a call does in a run of calls folded into one line (fold.ts).
export type Op = "search" | "read" | "list" | "command";

// An edit's old and new strings, each none when the input does not give one.
export type Edit = { old?: string; new?: string };

export type Call = {
  // hidden says the terminal draws the call nowhere.
  hidden: boolean;
  // name and argument are its head, Name(argument); an empty argument shows
  // no parentheses.
  name: string;
  argument: string;
  // op is what it does in a run; none for a call that does not fold.
  op?: Op;
  // file is the file a read reads, which a run counts once.
  file?: string;
  // hint is what an open run says its latest call works on.
  hint: string;
  // shell says it runs a shell command, whose own description an open run
  // shows while it runs.
  shell: boolean;
  description?: string;
  // copy is what a click on its head or its hint copies, and what to call it.
  copy: { text: string; what: string };
  // result is what is under it once done: an edit's lines, a written file's,
  // else its output, which empty says empty is.
  result: "edit" | "write" | "output";
  empty: string;
  // edits are an edit's strings and written a write's file, when its input
  // reads as an object.
  edits?: Edit[];
  written?: { content: string; path: string };
  // permitRow says an allowed call says so under it: a subagent's does not.
  permitRow: boolean;
};

const field = (input: Input | undefined, key: string): string | undefined => {
  const v = input?.[key];
  return typeof v === "string" ? v : undefined;
};

// inputOf is a call's input as an object, or none when it is not one (cut to
// its bound, still streaming, or empty).
function inputOf(text: string): Input | undefined {
  try {
    const value: unknown = JSON.parse(text);
    if (value && typeof value === "object" && !Array.isArray(value)) return value as Input;
  } catch {}
  return undefined;
}

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

function nameOf(name: string, input: Input | undefined): string {
  const mcp = /^mcp__(.+?)__(.+)$/.exec(name);
  if (mcp) return `${mcp[1]} - ${mcp[2]} (MCP)`;
  switch (name) {
    // Claude Code heads a call with a list of edits Update, whatever they
    // do, and Create only an empty old string of its own (2.1.310's edit
    // tool, read from its executable).
    case "Edit":
    case "MultiEdit":
      return input?.edits == null && field(input, "old_string") === "" ? "Create" : "Update";
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

const quoted = (v: unknown) => JSON.stringify(v);

// keyed is an input as key: value pairs, as the terminal heads a tool it has
// no words of its own for.
const keyed = (input: Input) =>
  Object.entries(input)
    .map(([k, v]) => `${k}: ${quoted(v)}`)
    .join(", ");

// argumentOf is what the head shows in parentheses: the tool's main field, or
// its input as typed when it does not read as an object.
function argumentOf(name: string, text: string, input: Input | undefined): string {
  if (!text.trim()) return "";
  if (!input) return oneLine(text);
  const path = (key: string) => shortPath(field(input, key) ?? "");
  switch (name) {
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

// The words a command is read by: its first word in each part of a pipeline
// or a chain. One the sets do not name makes it a shell command.
const searchWords = new Set(["find", "grep", "rg", "ag", "ack", "locate", "which", "whereis"]);
const readWords = new Set(
  "cat head tail less more wc stat file strings jq awk cut sort uniq tr".split(" "),
);
const listWords = new Set(["ls", "tree", "du"]);
const neutralWords = new Set(["echo", "printf", "true", "false", ":"]);

function commandOp(command: string): Op {
  const words = command
    .split(/&&|\|\||[;|\n]/)
    .map(
      (part) =>
        part
          .trim()
          .replace(/^(\w+=\S*\s+)+/, "")
          .split(/\s+/)[0] ?? "",
    )
    .filter((w) => w && !neutralWords.has(w));
  if (
    words.length === 0 ||
    words.some((w) => !searchWords.has(w) && !readWords.has(w) && !listWords.has(w))
  )
    return "command";
  if (words.some((w) => listWords.has(w))) return "list";
  return words.some((w) => searchWords.has(w)) ? "search" : "read";
}

// The field a call's head shows of its input, the one a click copies.
const shownField: Record<string, string> = {
  Bash: "command",
  Read: "file_path",
  Edit: "file_path",
  MultiEdit: "file_path",
  Write: "file_path",
  NotebookEdit: "notebook_path",
  WebFetch: "url",
  WebSearch: "query",
  Skill: "skill",
  Grep: "pattern",
  Glob: "pattern",
};

function described(name: string, text: string): Call {
  const input = inputOf(text);
  const command = field(input, "command") ?? "";
  const shell = name === "Bash";
  const call: Call = {
    hidden: unseen.has(name),
    name: nameOf(name, input),
    argument: argumentOf(name, text, input),
    hint: `$ ${oneLine(command)}`,
    shell,
    copy: {
      text: field(input, shownField[name] ?? "") ?? (input ? JSON.stringify(input, null, 2) : text),
      what: shell ? "the command" : "the call's input",
    },
    result: "output",
    empty: shell ? "(No output)" : "(No content)",
    permitRow: name !== "Agent",
  };
  switch (name) {
    case "Read":
      call.op = "read";
      call.file = field(input, "file_path");
      call.hint = shortPath(call.file ?? "");
      break;
    case "Grep":
    case "Glob":
      call.op = "search";
      call.hint = quoted(field(input, "pattern") ?? "");
      break;
    case "Bash":
      call.op = commandOp(command);
      call.description = field(input, "description");
      break;
    case "Edit":
    case "MultiEdit": {
      call.result = "edit";
      if (!input) break;
      const all: unknown[] = Array.isArray(input.edits) ? input.edits : [input];
      call.edits = all.map((e) => {
        const edit = e as Input | undefined;
        return { old: field(edit, "old_string"), new: field(edit, "new_string") };
      });
      break;
    }
    case "Write":
      call.result = "write";
      if (input)
        call.written = {
          content: field(input, "content") ?? "",
          path: shortPath(field(input, "file_path") ?? ""),
        };
      break;
  }
  return call;
}

// A call is read once for each name and input it has: a redraw comes thirty
// times a second, and a tool's input may be 64 KiB.
const read = new WeakMap<Item, { name: string; text: string; call: Call }>();

// callOf is all the face draws of a tool call.
export function callOf(item: Item): Call {
  const name = item.name ?? "tool";
  const hit = read.get(item);
  if (hit && hit.name === name && hit.text === item.text) return hit.call;
  const call = described(name, item.text);
  read.set(item, { name, text: item.text, call });
  return call;
}
