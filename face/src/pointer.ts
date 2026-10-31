// The person's hand over the face (docs/face.md, "Copying"). A press and its
// release with the pointer on the press's cell all the while is a click,
// which copies the part under it; a drag to any other cell selects, and its
// release copies the selection. A press drops the selection there was; the
// pointer with no button pressed lights nothing (the owner's word, October 31,
// 2026: a light that follows the pointer gets in the way).
import type { Point, Selection } from "./select.ts";

// View is where the conversation's rows stand in the face: the first shown,
// how many, in a view of height rows width columns wide, the shown rows at
// its bottom when there are fewer.
export type View = { top: number; shown: number; height: number; width: number };

// rowAt is the conversation's row on a row of the view, none off its rows.
export function rowAt(y: number, view: View): number | undefined {
  const n = y - (view.height - view.shown);
  return n >= 0 && n < view.shown && y < view.height ? view.top + n : undefined;
}

// pointAt is the cell under a point, past the rows shown their first cell or
// their last: a drag above the view takes the first row from its start, one
// below it the last row to its end.
export function pointAt(x: number, y: number, view: View): Point {
  const n = y - (view.height - view.shown);
  if (view.shown === 0 || n < 0) return { row: view.top, col: 0 };
  if (n >= view.shown) return { row: view.top + view.shown - 1, col: view.width };
  return { row: view.top + n, col: Math.min(Math.max(x, 0), Math.max(view.width - 1, 0)) };
}

// A click names the conversation's row it was on.
export type Gesture = { selected: Selection } | { clicked: number } | undefined;

export class Pointer {
  selection: Selection | undefined;
  private pressed:
    | { at: Point; row: number | undefined; x: number; y: number; focused: boolean }
    | undefined;
  private dragging = false;

  get busy() {
    return this.pressed !== undefined;
  }

  // down presses on a point of the view: a drag selects from the cell under
  // it, the nearest row's past the rows shown, while a click is only on a row
  // shown.
  down(view: View, x: number, y: number, focused: boolean) {
    this.selection = undefined;
    this.dragging = false;
    this.pressed = { at: pointAt(x, y, view), row: rowAt(y, view), x, y, focused };
  }

  // drag moves the selection's end; it says whether there is one.
  drag(at: Point, x: number, y: number): boolean {
    const p = this.pressed;
    if (!p || (!this.dragging && x === p.x && y === p.y)) return false;
    this.dragging = true;
    this.selection = { anchor: p.at, focus: at };
    return true;
  }

  // up ends the press: a selection to copy, a click on a row of a focused
  // face, or nothing.
  up(): Gesture {
    const p = this.pressed;
    this.pressed = undefined;
    if (!p) return undefined;
    if (this.dragging) {
      this.dragging = false;
      return this.selection && { selected: this.selection };
    }
    return p.focused && p.row !== undefined ? { clicked: p.row } : undefined;
  }

  clear() {
    this.selection = undefined;
  }
}

// Flash is the part a click copied, lit for a moment with the notice so the
// person sees what went; nothing lights as the pointer moves.
const flashFor = 1000;

export class Flash {
  part: string | undefined;
  private timer: ReturnType<typeof setTimeout> | undefined;

  // over is told when the light goes out by itself.
  constructor(private readonly over: () => void) {}

  on(part: string) {
    this.off();
    this.part = part;
    this.timer = setTimeout(() => {
      this.part = undefined;
      this.over();
    }, flashFor);
  }

  off() {
    clearTimeout(this.timer);
    this.part = undefined;
  }
}
