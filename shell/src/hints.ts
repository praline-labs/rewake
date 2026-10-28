// The key hints under main's status: what the keys do now, for the focus
// and the moment (docs/shell.md, "The bar"). A hint is an entry of the table
// below, kept or left out by its when; a new one is a line here, not a string
// in the layout.

export type Context = {
  // focus is the program the keys go to, or the briefs; prefix is a Ctrl+\
  // waiting for its command; brief is one brief open in the briefs.
  focus: "main" | "worker" | "briefs";
  name: string;
  brief?: boolean;
  prefix: boolean;
  zoomed: boolean;
  workers: number;
  // face is the focused pane's face, where its session serves a mirror.
  face?: "rewake" | "native";
};

export type Hint = { keys: string; does: string };

type Entry = {
  keys: string;
  does: string | ((c: Context) => string);
  when?: (c: Context) => boolean;
};

const hasWorkers = (c: Context) => c.workers > 0;
const notMain = (c: Context) => c.focus !== "main";
const hasFace = (c: Context) => c.face !== undefined;
const otherFace = (c: Context) => (c.face === "rewake" ? "native face" : "rewake face");

const table: Record<Context["focus"] | "prefix", Entry[]> = {
  // After Ctrl+\: the commands themselves.
  prefix: [
    { keys: "n/p", does: "next/previous" },
    { keys: "←", does: "main", when: notMain },
    { keys: "→", does: "worker", when: (c) => c.focus === "main" && hasWorkers(c) },
    { keys: "1-9", does: "a worker by place", when: hasWorkers },
    { keys: "0/m", does: "main", when: notMain },
    { keys: "z", does: (c) => (c.zoomed ? "end the zoom" : "zoom") },
    { keys: "b", does: (c) => (c.focus === "briefs" ? "back" : "briefs"), when: hasWorkers },
    { keys: "v", does: otherFace, when: hasFace },
    {
      keys: "Ctrl+\\ again",
      does: (c) => `pass Ctrl+\\ to ${c.name}`,
      when: (c) => c.focus !== "briefs",
    },
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
    {
      keys: "keys",
      does: (c) => (c.face === "rewake" ? `a prompt for ${c.name}` : `go to ${c.name}`),
    },
    { keys: "Ctrl+\\ v", does: otherFace, when: hasFace },
    // Main has no button on the bar: m reaches it in every layout.
    { keys: "Ctrl+\\ ←/m", does: "main" },
    { keys: "Ctrl+\\ n/p", does: "next/previous" },
    { keys: "Ctrl+\\ z", does: (c) => (c.zoomed ? "end the zoom" : "zoom") },
    { keys: "Ctrl+\\ b", does: "briefs" },
    { keys: "Ctrl+\\ twice", does: (c) => `sends Ctrl+\\ to ${c.name}` },
  ],
  // The briefs read only: their own keys, then the way out.
  briefs: [
    { keys: "↑↓ Enter", does: "open a brief", when: (c) => !c.brief },
    { keys: "↑↓ PgUp PgDn", does: "scroll", when: (c) => c.brief === true },
    // Esc still goes back, but is never needed: it is the harnesses'
    // interrupt, and a slip of the focus would send it to a worker.
    { keys: "←→", does: "another brief", when: (c) => c.brief === true },
    { keys: "Backspace", does: "the list", when: (c) => c.brief === true },
    { keys: "Ctrl+\\ ←/m", does: "main" },
    { keys: "Ctrl+\\ n/p", does: "next/previous", when: hasWorkers },
    { keys: "Ctrl+\\ z", does: (c) => (c.zoomed ? "end the zoom" : "zoom") },
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
