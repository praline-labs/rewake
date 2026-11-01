// Runs of reads, searches, listings and commands folded into one line, as
// Claude Code's terminal folds them (docs/face.md, "The look"): "Searched for
// 1 pattern, read 2 files, ran 1 shell command", the numbers bold. Text, a
// prompt, thinking or another tool ends a run, so each sits where it came; a
// notice does not, and shows after it.

import { callOf, type Op } from "./call.ts";
import type { FaceLine, Span } from "./messages.ts";
import type { Item } from "./mirror.gen.ts";
import { tagged } from "./parts.ts";
import { hidden } from "./tools.ts";
import { clean, wrapSpans } from "./wrap.ts";

// opOf is what a call does in a run, or none when it does not fold.
export const opOf = (item: Item): Op | undefined =>
  item.kind === "tool" ? callOf(item).op : undefined;

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
    const { file } = callOf(call);
    if (file !== undefined) files.add(file);
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
// calls drawn under it (render.ts), each with its own mark of a loss. hint
// follows the line: how to open it, or to fold it back.
export function runLines(
  run: { calls: Item[]; open: boolean },
  width: number,
  opened = false,
  hint?: string,
): FaceLine[] {
  const lines = drawnRun(run, width, opened, hint);
  return !opened && run.calls.some((c) => c.status === "gap") ? [...lines, lostLine] : lines;
}

// A click on a run's line opens or closes it (parts.ts).
const parted = (lines: FaceLine[], calls: Item[]) => {
  const first = calls[0];
  return first ? tagged(lines, first, "fold") : lines;
};

function drawnRun(
  run: { calls: Item[]; open: boolean },
  width: number,
  opened: boolean,
  hint?: string,
): FaceLine[] {
  const pad = { text: "  " };
  const hinted = (spans: Span[]): Span[] =>
    hint ? [...spans, { text: ` ${hint}`, tone: "faint" }] : spans;
  if (!run.open)
    return parted(hang(wrapSpans(hinted(said(run.calls, "done")), width - 2), pad, pad), run.calls);
  const last = run.calls[run.calls.length - 1];
  const running = [...run.calls].reverse().find((c) => c.status === "running" && callOf(c).shell);
  const description = running && callOf(running).description;
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
  const out = parted(hang(wrapSpans(hinted(head), width - 2), dot, pad), run.calls);
  if (last && !opened) {
    const hint = wrapSpans([{ text: callOf(last).hint, tone: "faint" }], width - 5)[0] ?? [];
    // A click on it copies what the latest call works on (parts.ts).
    out.push(...tagged([{ spans: [{ text: "  ⎿  ", gutter: true }, ...hint] }], last, "input"));
  }
  return out;
}
