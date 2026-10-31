// Runs of reads, searches, listings and commands folded into one line, as
// Claude Code's terminal folds them (docs/face.md, "The look"): "Searched for
// 1 pattern, read 2 files, ran 1 shell command", the numbers bold. Text, a
// prompt, thinking or another tool ends a run, so each sits where it came; a
// notice does not, and shows after it.

import type { FaceLine, Span } from "./messages.ts";
import type { Item } from "./mirror.gen.ts";
import { partKey } from "./select.ts";
import { field, hidden, inputOf, shortPath } from "./tools.ts";
import { clean, wrapSpans } from "./wrap.ts";

type Op = "search" | "read" | "list" | "command";

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

// opOf is what a call does in a run, or none when it does not fold.
export function opOf(item: Item): Op | undefined {
  if (item.kind !== "tool") return undefined;
  switch (item.name) {
    case "Read":
      return "read";
    case "Grep":
    case "Glob":
      return "search";
    case "Bash":
      return commandOp(field(inputOf(item), "command") ?? "");
    default:
      return undefined;
  }
}

export type Entry = { item: Item } | { calls: Item[]; open: boolean };

// entriesOf is the conversation as the face draws it: each item, or a run of
// folding calls as one entry. A run is open while a call in it runs, or while
// it is the last thing a working turn did.
export function entriesOf(items: Item[], working: boolean): Entry[] {
  const out: Entry[] = [];
  let run: { calls: Item[]; open: boolean } | undefined;
  let held: Item[] = [];
  const close = () => {
    if (run) out.push(run, ...held.map((item) => ({ item })));
    run = undefined;
    held = [];
  };
  for (const item of items) {
    if (hidden(item)) continue;
    if (opOf(item)) {
      run ??= { calls: [], open: false };
      run.calls.push(item);
      if (item.status === "running") run.open = true;
    } else if (run && item.kind === "notice") {
      held.push(item);
    } else {
      close();
      out.push({ item });
    }
  }
  if (run && working) run.open = true;
  close();
  return out;
}

const words: Record<
  Op,
  { done: [string, string]; doing: [string, string]; thing: [string, string] }
> = {
  search: {
    done: ["Searched for", "searched for"],
    doing: ["Searching for", "searching for"],
    thing: ["pattern", "patterns"],
  },
  read: { done: ["Read", "read"], doing: ["Reading", "reading"], thing: ["file", "files"] },
  list: {
    done: ["Listed", "listed"],
    doing: ["Listing", "listing"],
    thing: ["directory", "directories"],
  },
  command: {
    done: ["Ran", "ran"],
    doing: ["Running", "running"],
    thing: ["shell command", "shell commands"],
  },
};

// counts is how many of each a run did; reads count the files, each once.
function counts(calls: Item[]): [Op, number][] {
  const n: Record<Op, number> = { search: 0, read: 0, list: 0, command: 0 };
  const files = new Set<string>();
  for (const call of calls) {
    const op = opOf(call);
    if (!op) continue;
    const path = call.name === "Read" ? field(inputOf(call), "file_path") : undefined;
    if (path !== undefined) files.add(path);
    else n[op]++;
  }
  n.read += files.size;
  return (["search", "read", "list", "command"] as const)
    .filter((op) => n[op] > 0)
    .map((op) => [op, n[op]]);
}

const failed = (call: Item) => call.status === "error" || call.status === "refused";

// said is what a run did, counted, and how many of its calls failed, in red
// as the terminal program marks a run with a failed call.
function said(calls: Item[], tense: "done" | "doing"): Span[] {
  const spans: Span[] = [];
  counts(calls).forEach(([op, n], i) => {
    const w = words[op];
    if (i > 0) spans.push({ text: ", ", tone: "faint" });
    spans.push({ text: `${w[tense][i === 0 ? 0 : 1]} `, tone: "faint" });
    spans.push({ text: String(n), tone: "faint", bold: true });
    spans.push({ text: ` ${w.thing[n === 1 ? 0 : 1]}`, tone: "faint" });
  });
  const n = calls.filter(failed).length;
  if (n > 0)
    spans.push(
      { text: ", ", tone: "faint" },
      { text: String(n), tone: "error", bold: true },
      { text: " failed", tone: "error" },
    );
  return spans;
}

// hintOf is what a call works on: a path, a pattern or a command.
function hintOf(call: Item): string {
  const input = inputOf(call);
  switch (call.name) {
    case "Read":
      return shortPath(field(input, "file_path") ?? "");
    case "Grep":
    case "Glob":
      return JSON.stringify(field(input, "pattern") ?? "");
    default:
      return `$ ${clean(field(input, "command") ?? "")
        .replace(/\s+/g, " ")
        .trim()}`;
  }
}

// lostLine marks an item, or a call within a run, that a gap lost on the way.
export const lostLine: FaceLine = {
  spans: [{ text: "  [lost on the way — the native face has it]", tone: "gap" }],
};

const hang = (rows: Span[][], first: Span, rest: Span): FaceLine[] =>
  rows.map((row, i) => ({
    spans: [i === 0 ? { ...first, gutter: true } : { ...rest, gutter: true }, ...row],
  }));

// runLines is a run's rows: a finished run one faint line, an open one its
// dot, the running command's description or what it is doing, and what the
// latest call works on. A run the person opened shows its line alone, its
// calls drawn under it (render.ts), each with its own mark of a loss.
export function runLines(
  run: { calls: Item[]; open: boolean },
  width: number,
  opened = false,
): FaceLine[] {
  const lines = drawnRun(run, width, opened);
  return !opened && run.calls.some((c) => c.status === "gap") ? [...lines, lostLine] : lines;
}

// A click on a run's line opens or closes it (unfold.ts).
const parted = (lines: FaceLine[], calls: Item[]) => {
  const first = calls[0];
  return first ? lines.map((line) => ({ ...line, part: partKey("fold", first) })) : lines;
};

function drawnRun(
  run: { calls: Item[]; open: boolean },
  width: number,
  opened: boolean,
): FaceLine[] {
  const pad = { text: "  " };
  if (!run.open)
    return parted(hang(wrapSpans(said(run.calls, "done"), width - 2), pad, pad), run.calls);
  const last = run.calls[run.calls.length - 1];
  const running = [...run.calls].reverse().find((c) => c.status === "running" && c.name === "Bash");
  const description = running && field(inputOf(running), "description");
  const head: Span[] = description
    ? [{ text: `${clean(description).replace(/\s+/g, " ").trim()}…` }]
    : [
        ...said(run.calls, "doing").map((s) => ({
          ...s,
          tone: s.tone === "error" ? s.tone : undefined,
        })),
        { text: "…" },
      ];
  // The dot is red once a call of the run failed, as the terminal program's.
  const dot: Span = { text: "● ", tone: run.calls.some(failed) ? "error" : "faint", blink: true };
  const out = parted(hang(wrapSpans(head, width - 2), dot, pad), run.calls);
  if (last && !opened) {
    const hint = wrapSpans([{ text: hintOf(last), tone: "faint" }], width - 5)[0] ?? [];
    // A click on it copies what the latest call works on (parts.ts).
    out.push({ spans: [{ text: "  ⎿  ", gutter: true }, ...hint], part: partKey("input", last) });
  }
  return out;
}
