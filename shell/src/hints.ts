// The key hints under the bar: what the keys do now, for the focus and the
// moment (docs/shell.md, "The bar"). A hint is an entry of the table below,
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
    { keys: "Ctrl+\\ again", does: (c) => `pass Ctrl+\\ to ${c.name}` },
  ],
  main: [
    { keys: "keys", does: "go to main" },
    { keys: "Ctrl+\\ →", does: "worker", when: hasWorkers },
    { keys: "Ctrl+\\ n/p", does: "next/previous", when: hasWorkers },
    { keys: "Ctrl+\\ z", does: (c) => (c.zoomed ? "end the zoom" : "zoom"), when: hasWorkers },
    { keys: "Ctrl+\\", does: "the shell's commands", when: (c) => !hasWorkers(c) },
  ],
  worker: [
    { keys: "keys", does: (c) => `go to ${c.name}` },
    { keys: "Ctrl+\\ ←", does: "main" },
    { keys: "Ctrl+\\ n/p", does: "next/previous" },
    { keys: "Ctrl+\\ z", does: (c) => (c.zoomed ? "end the zoom" : "zoom") },
    { keys: "Ctrl+\\ twice", does: (c) => `sends Ctrl+\\ to ${c.name}` },
  ],
};

export function hintsFor(c: Context): Hint[] {
  return (c.prefix ? table.prefix : table[c.focus])
    .filter((e) => !e.when || e.when(c))
    .map((e) => ({ keys: e.keys, does: typeof e.does === "string" ? e.does : e.does(c) }));
}
