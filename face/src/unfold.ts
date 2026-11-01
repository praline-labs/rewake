// What of the conversation is shown whole (docs/face.md, "Unfolding"), as
// the terminal programs' ctrl+o shows it: the person's key opens or closes
// everything at once, and a click on a run's line or on the row counting the
// lines cut opens or closes that one (parts.ts).

import { type Fold, keyOf } from "./parts.ts";

// Opened is what the drawing asks: whether everything is open, and whether
// a fold is.
export type Opened = { all: boolean; open(fold: Fold): boolean };

// shut is the drawing with everything folded.
export const shut: Opened = { all: false, open: () => false };

export class Unfold implements Opened {
  all = false;
  // turned are the folds a click turned the other way from all.
  private turned = new Set<string>();

  open(fold: Fold): boolean {
    return this.all !== this.turned.has(keyOf(fold));
  }

  toggle(fold: Fold) {
    const key = keyOf(fold);
    if (!this.turned.delete(key)) this.turned.add(key);
  }

  // every opens or closes everything, forgetting what the clicks turned.
  every(all: boolean) {
    this.all = all;
    this.turned.clear();
  }

  // forget drops the clicks' folds, whose items a new snapshot replaces.
  forget() {
    this.turned.clear();
  }
}
