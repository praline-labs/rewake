// What of the conversation is shown whole (docs/face.md, "Unfolding"), as
// the terminal programs' ctrl+o shows it: the person's key opens or closes
// everything at once, and a click on a run's line or on the row counting the
// lines cut opens or closes that one. A part is named by the key of the row
// a click toggles it by (select.ts): "fold <first call>" for a run, "more
// <item>" for a cut output or thinking block.

import type { FaceLine } from "./messages.ts";

// Opened is what the drawing asks: whether everything is open, and whether
// the part a key names is.
export type Opened = { all: boolean; open(key: string): boolean };

// shut is the drawing with everything folded.
export const shut: Opened = { all: false, open: () => false };

// toggles says whether a click on a row of this key opens or closes rather
// than copies.
export const toggles = (key: string) => key.startsWith("fold ") || key.startsWith("more ");

export class Unfold implements Opened {
  all = false;
  // turned are the parts a click turned the other way from all.
  private turned = new Set<string>();

  open(key: string): boolean {
    return this.all !== this.turned.has(key);
  }

  toggle(key: string) {
    if (!this.turned.delete(key)) this.turned.add(key);
  }

  // every opens or closes everything, forgetting what the clicks turned.
  every(all: boolean) {
    this.all = all;
    this.turned.clear();
  }

  // forget drops the clicks' parts, whose items a new snapshot replaces.
  forget() {
    this.turned.clear();
  }
}

// Anchor is a row to keep where it stood in the view once the conversation is
// laid out again: the key of its part, its place among that part's rows and
// its row in the view.
export type Anchor = { key: string; k: number; y: number };

// anchorAt is the anchor of a row of the view whose first row is top, none
// for a row of no part.
export function anchorAt(rows: FaceLine[], row: number, top: number): Anchor | undefined {
  const key = rows[row]?.part;
  if (key === undefined) return undefined;
  return { key, k: row - rows.findIndex((r) => r.part === key), y: row - top };
}

// topFor is the first row of a view that shows the anchor's row where it
// stood, none when its part is gone; a part grown shorter keeps its last row.
export function topFor(rows: FaceLine[], { key, k, y }: Anchor): number | undefined {
  const first = rows.findIndex((r) => r.part === key);
  if (first < 0) return undefined;
  const last = rows.findLastIndex((r) => r.part === key);
  return Math.max(first + Math.min(k, last - first) - y, 0);
}
