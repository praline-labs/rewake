// The key hints under main's status: what the keys do now, for the focus
// and the moment (docs/shell.md, "The bar"). A hint is an entry of the table
// below, kept or left out by its when; a new one is a line here, not a string
// in the layout.

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
    { keys: "b", does: "briefs", when: hasWorkers },
    { keys: "Ctrl+\\ again", does: (c) => `pass Ctrl+\\ to ${c.name}` },
  ],
  main: [
    { keys: "keys", does: "go to main" },
    { keys: "Ctrl+\\ →", does: "worker", when: hasWorkers },
    { keys: "Ctrl+\\ n/p", does: "next/previous", when: hasWorkers },
    { keys: "Ctrl+\\ z", does: (c) => (c.zoomed ? "end the zoom" : "zoom"), when: hasWorkers },
    { keys: "Ctrl+\\ b", does: "briefs", when: hasWorkers },
    { keys: "Ctrl+\\", does: "the shell's commands", when: (c) => !hasWorkers(c) },
  ],
  worker: [
    { keys: "keys", does: (c) => `go to ${c.name}` },
    // Main has no button on the bar: m reaches it in every layout.
    { keys: "Ctrl+\\ ←/m", does: "main" },
    { keys: "Ctrl+\\ n/p", does: "next/previous" },
    { keys: "Ctrl+\\ z", does: (c) => (c.zoomed ? "end the zoom" : "zoom") },
    { keys: "Ctrl+\\ b", does: "briefs" },
    { keys: "Ctrl+\\ twice", does: (c) => `sends Ctrl+\\ to ${c.name}` },
  ],
};

// separator parts the hints as it parts the strip.
export const separator = " · ";

export function hintsFor(c: Context): Hint[] {
  return (c.prefix ? table.prefix : table[c.focus])
    .filter((e) => !e.when || e.when(c))
    .map((e) => ({ keys: e.keys, does: typeof e.does === "string" ? e.does : e.does(c) }));
}

// fitHints is the hints that fit a width whole, in the table's order; the
// first stays even when it does not, and is cut at the width.
export function fitHints(hints: Hint[], width: number): Hint[] {
  let used = -separator.length;
  const out: Hint[] = [];
  for (const h of hints) {
    used += separator.length + h.keys.length + 1 + h.does.length;
    if (used > width && out.length > 0) break;
    out.push(h);
  }
  return out;
}
