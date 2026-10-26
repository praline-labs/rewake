// The panes the shell shows: main's, and a worker's for each open space of
// the room (docs/shell.md, "The prefix and the room").
import { createSignal } from "solid-js";
import type { PaneTerminal } from "./pane.ts";
import type { Space } from "./room.ts";
import { Scanner } from "./scan.ts";
import type { Source } from "./sources.ts";
import { showSpaces } from "./strip.tsx";

// A pane is a program on the screen: main, or a space's worker.
export type Pane = {
  id: string;
  label: string;
  term?: PaneTerminal;
  source?: Source;
  scanner: Scanner;
  title: string;
  space?: Space;
};

export const mainPane: Pane = { id: "main", label: "main", scanner: new Scanner(), title: "" };
export const [workers, setWorkers] = createSignal<Pane[]>([]);
export const [focused, setFocused] = createSignal("main");
export const [chosen, setChosen] = createSignal<string>();
export const [zoomed, setZoomed] = createSignal(false);
// briefs is the id the briefs view is chosen and focused by in the worker's
// place; a space's id is 32 hex digits, so no worker has it.
export const briefs = "briefs";
// before is what was chosen and focused when the briefs were, for the
// prefix's b to go back to.
let before: { chosen?: string; focused: string } = { focused: "main" };

export const allPanes = () => [mainPane, ...workers()];
export const paneOf = (id: string | undefined) => allPanes().find((p) => p.id === id);

// The room's spaces become the worker panes, in the order they opened. A
// space shown here whose main's run reads ended is main's end begun.
export function takeSpaces(
  spaces: Space[],
  ended: string[],
  closing: () => unknown,
  beginClosing: () => void,
) {
  if (closing()) return;
  showSpaces(spaces);
  const was = workers();
  if (was.some((p) => ended.includes(p.id))) return beginClosing();
  const next = spaces.map(
    (space) =>
      was.find((p) => p.id === space.id) ?? {
        id: space.id,
        label: space.name,
        scanner: new Scanner(),
        title: "",
        space,
      },
  );
  for (const pane of was) if (!next.includes(pane)) pane.source?.close();
  if (next.length === was.length && next.every((p, i) => p === was[i])) return;
  setWorkers(next);
  // The briefs are the navigation's, which shows only while a worker is up.
  const keep = (id: string | undefined) => id === briefs && next.length > 0;
  if (!keep(chosen()) && (!paneOf(chosen()) || chosen() === "main")) setChosen(next[0]?.id);
  if (!keep(focused()) && !paneOf(focused())) show("main");
}

// cycle is what the prefix's n and p go through: main, the workers and,
// while the bar shows, the briefs at its end.
export const cycle = () => [
  mainPane.id,
  ...workers().map((p) => p.id),
  ...(workers().length > 0 ? [briefs] : []),
];

export function show(id: string | undefined) {
  if (id === briefs) {
    if (workers().length === 0) return;
    if (focused() !== briefs) before = { chosen: chosen(), focused: focused() };
    setChosen(briefs);
    setFocused(briefs);
    return;
  }
  const pane = paneOf(id);
  if (!pane) return;
  if (pane !== mainPane) setChosen(pane.id);
  setFocused(pane.id);
}

// toggleBriefs is the prefix's b: the briefs, or, while they have the keys,
// back to what was chosen and focused before them, as far as it still is.
export function toggleBriefs() {
  if (focused() !== briefs) return show(briefs);
  const was = before.chosen;
  setChosen(was === briefs || (was !== "main" && paneOf(was)) ? was : workers()[0]?.id);
  setFocused(paneOf(before.focused)?.id ?? "main");
}
