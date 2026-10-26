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
// briefsOpen is the briefs modal showing: the panes are blurred under it and
// its keys are its own.
export const [briefsOpen, setBriefsOpen] = createSignal(false);

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
  if (!paneOf(chosen()) || chosen() === "main") setChosen(next[0]?.id);
  if (!paneOf(focused())) show("main");
}

export function show(id: string | undefined) {
  const pane = paneOf(id);
  if (!pane) return;
  if (pane !== mainPane) setChosen(pane.id);
  setFocused(pane.id);
}
