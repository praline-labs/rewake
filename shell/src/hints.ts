// The key hints under the bar: what the keys do now, for the focus and the
// moment (docs/v2/shell.md, "The bar"). A hint is an entry of the table below,
// kept or left out by its when; a new one is a line here, not a string in the
// layout.

export type Context = {
  // focus is the program the keys go to; prefix is a Ctrl+\ waiting for
  // its command.
  focus: "main" | "worker";
  name: string;
  prefix: boolean;
  zoomed: boolean;
  workers: number;
};

export type Hint = { keys: string; does: string };

type Entry = {
  // lead is the hint pressed to the left edge, where the keys go; the
  // others are centred in the rest of the line.
  lead?: boolean;
  keys: string;
  does: string | ((c: Context) => string);
  when?: (c: Context) => boolean;
};

const hasWorkers = (c: Context) => c.workers > 0;

const table: Record<"prefix" | "main" | "worker", Entry[]> = {
  // After Ctrl+\: the commands themselves.
  prefix: [
    { keys: "n/p", does: "next/previous" },
    { keys: "←", does: "main", when: (c) => c.focus === "worker" },
    { keys: "→", does: "worker", when: (c) => c.focus === "main" && hasWorkers(c) },
    { keys: "1-9", does: "a worker by place", when: hasWorkers },
    { keys: "0/m", does: "main", when: (c) => c.focus === "worker" },
    { keys: "z", does: (c) => (c.zoomed ? "end the zoom" : "zoom") },
    { keys: "Ctrl+\\", does: (c) => `send it to ${c.name}` },
    { keys: "other", does: "cancel" },
  ],
  main: [
    { lead: true, keys: "keys", does: "go to main" },
    { keys: "Ctrl+\\ →", does: "worker", when: hasWorkers },
    { keys: "Ctrl+\\ n/p", does: "next/previous", when: hasWorkers },
    { keys: "Ctrl+\\ z", does: (c) => (c.zoomed ? "end the zoom" : "zoom"), when: hasWorkers },
    { keys: "click", does: "a tab or a pane focuses it", when: hasWorkers },
    { keys: "Ctrl+\\", does: "the shell's commands", when: (c) => !hasWorkers(c) },
  ],
  worker: [
    { lead: true, keys: "keys", does: (c) => `go to ${c.name}` },
    { keys: "Ctrl+\\ ←", does: "main" },
    { keys: "Ctrl+\\ n/p", does: "next/previous" },
    { keys: "Ctrl+\\ z", does: (c) => (c.zoomed ? "end the zoom" : "zoom") },
    { keys: "Ctrl+\\ twice", does: (c) => `sends Ctrl+\\ to ${c.name}` },
  ],
};

export function hintsFor(c: Context): { lead?: Hint; rest: Hint[] } {
  const entries = (c.prefix ? table.prefix : table[c.focus]).filter((e) => !e.when || e.when(c));
  const hint = (e: Entry) => ({
    keys: e.keys,
    does: typeof e.does === "string" ? e.does : e.does(c),
  });
  const lead = entries.find((e) => e.lead);
  return { lead: lead && hint(lead), rest: entries.filter((e) => !e.lead).map(hint) };
}

const gap = "   ";
const widthOf = (h: Hint) => [...h.keys].length + 1 + [...h.does].length;

// centring is the spaces before the rest so it sits in the middle of what
// the lead leaves; none when it does not fit, so the line is cut at its end.
export function centring(width: number, lead: Hint | undefined, rest: Hint[]): string {
  const used = lead ? 1 + widthOf(lead) : 0;
  const need = rest.reduce((n, h) => n + widthOf(h), 0) + gap.length * Math.max(rest.length - 1, 0);
  const pad = Math.floor((width - used - need) / 2);
  return " ".repeat(Math.max(pad, lead ? gap.length : 1));
}
