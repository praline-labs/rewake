// Where the face's view stands (docs/face.md, "The face"): at the bottom it
// follows new rows; scrolled up it holds the row the person put at its top
// while rows arrive below, until they scroll back to the bottom.

export class Scroll {
  // top is the first row shown, counted from the model's first; undefined
  // follows the bottom.
  top: number | undefined;

  // by moves the view up by rows (down when negative) over total rows in a
  // view height rows tall; reaching the bottom follows again.
  by(rows: number, total: number, height: number) {
    const bottom = Math.max(total - height, 0);
    const next = Math.min(Math.max((this.top ?? bottom) - rows, 0), bottom);
    this.top = next >= bottom ? undefined : next;
  }

  follow() {
    this.top = undefined;
  }

  // view is the rows shown out of all of them: the tail while following, the
  // held ones otherwise, the bottom once fewer rows than that are left.
  view<T>(all: T[], height: number): T[] {
    const bottom = Math.max(all.length - height, 0);
    if (this.top !== undefined && this.top >= bottom) this.top = undefined;
    const from = this.top ?? bottom;
    return all.slice(from, from + height);
  }
}
